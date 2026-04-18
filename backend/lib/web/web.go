package web

import (
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"

	"github.com/julienschmidt/httprouter"
)

const API_V1 = "1.0.0"

// ── Request ───────────────────────────────────────────────────────────────────

// Request wraps *http.Request and httprouter.Params.
type Request struct {
	raw    *http.Request
	params httprouter.Params
	ctx    context.Context
}

func newRequest(r *http.Request, ps httprouter.Params) *Request {
	return &Request{raw: r, params: ps, ctx: r.Context()}
}

func (r *Request) Context() context.Context { return r.ctx }

func (r *Request) Header(key string) string { return r.raw.Header.Get(key) }

func (r *Request) Param(name string) string { return r.params.ByName(name) }

func (r *Request) FormValue(key string) string { return r.raw.FormValue(key) }

func (r *Request) ParseMultipartForm(maxMemory int64) error {
	return r.raw.ParseMultipartForm(maxMemory)
}

func (r *Request) FormFile(key string) (multipart.File, *multipart.FileHeader, error) {
	return r.raw.FormFile(key)
}

func (r *Request) DecodeBody(v any) error {
	defer r.raw.Body.Close()
	return json.NewDecoder(r.raw.Body).Decode(v)
}

func (r *Request) SetContextValue(key, value any) {
	r.ctx = context.WithValue(r.ctx, key, value)
}

func (r *Request) GetContextValue(key any) any {
	return r.ctx.Value(key)
}

// ── Response ──────────────────────────────────────────────────────────────────

// Response carries the HTTP status code and the body to write.
type Response struct {
	statusCode  int
	body        any
	rawBody     []byte
	contentType string // non-empty triggers raw (non-JSON) write
}

type successBody struct {
	Success    bool   `json:"success"`
	Data       any    `json:"data"`
	APIVersion string `json:"api_version"`
}

type errorDetail struct {
	Code        string `json:"code"`
	Description string `json:"description"`
}

type errorBody struct {
	Success bool        `json:"success"`
	Error   errorDetail `json:"error"`
}

// NewResponse constructs a successful JSON response.
func NewResponse(data any, success bool, statusCode int, apiVersion string) Response {
	return Response{
		statusCode: statusCode,
		body: successBody{
			Success:    success,
			Data:       data,
			APIVersion: apiVersion,
		},
	}
}

// NewRawResponse constructs a response that writes raw bytes with the given content type.
func NewRawResponse(data []byte, contentType string, statusCode int) Response {
	return Response{
		statusCode:  statusCode,
		rawBody:     data,
		contentType: contentType,
	}
}

func errResponse(code, desc string, statusCode int) Response {
	return Response{
		statusCode: statusCode,
		body: errorBody{
			Success: false,
			Error:   errorDetail{Code: code, Description: desc},
		},
	}
}

func ErrBadRequest(desc string) Response {
	return errResponse("bad_request", desc, http.StatusBadRequest)
}

func ErrNotFound(desc string) Response {
	return errResponse("not_found", desc, http.StatusNotFound)
}

func ErrInternalServerError(desc string) Response {
	return errResponse("internal_server_error", desc, http.StatusInternalServerError)
}

func ErrConflict(desc string) Response {
	return errResponse("conflict", desc, http.StatusConflict)
}

func ErrWithStatus(desc string, statusCode int) Response {
	return errResponse("error", desc, statusCode)
}

// write serialises the response to the http.ResponseWriter.
func (resp Response) write(w http.ResponseWriter) {
	if resp.contentType != "" && resp.rawBody != nil {
		w.Header().Set("Content-Type", resp.contentType)
		w.WriteHeader(resp.statusCode)
		_, _ = w.Write(resp.rawBody)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(resp.statusCode)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(resp.body)
}

// ── Endpoint & Middleware ─────────────────────────────────────────────────────

// Endpoint is a handler function that takes a *Request and returns a Response.
type Endpoint func(*Request) Response

// Middleware wraps an Endpoint.
type Middleware func(Endpoint) Endpoint

// Serve adapts an Endpoint (with middlewares applied left-to-right) to an
// httprouter.Handle.
func Serve(middlewares []Middleware, handler Endpoint) httprouter.Handle {
	// Apply middlewares right-to-left so the first middleware is outermost.
	h := handler
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		req := newRequest(r, ps)
		resp := h(req)
		resp.write(w)
	}
}
