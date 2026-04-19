package services

import "net/http"

// ServiceErrorInterface is returned by all service methods instead of plain error.
type ServiceErrorInterface interface {
	HttpStatus() int
	Description() string
	Code() string
}

type serviceError struct {
	code        string
	description string
	httpStatus  int
}

func (e *serviceError) HttpStatus() int     { return e.httpStatus }
func (e *serviceError) Description() string { return e.description }
func (e *serviceError) Code() string        { return e.code }

func newServiceError(code, description string, httpStatus int) ServiceErrorInterface {
	return &serviceError{code: code, description: description, httpStatus: httpStatus}
}

// ── Error factories ───────────────────────────────────────────────────────────

var (
	ErrBadRequest = func(desc string) ServiceErrorInterface {
		return newServiceError("bad_request", desc, http.StatusBadRequest)
	}

	ErrNotFound = func(desc string) ServiceErrorInterface {
		return newServiceError("not_found", desc, http.StatusNotFound)
	}

	ErrInternalServer = func(desc string) ServiceErrorInterface {
		return newServiceError("internal_server_error", desc, http.StatusInternalServerError)
	}

	ErrConflict = func(desc string) ServiceErrorInterface {
		return newServiceError("conflict", desc, http.StatusConflict)
	}

	ErrBadGateway = func(desc string) ServiceErrorInterface {
		return newServiceError("bad_gateway", desc, http.StatusBadGateway)
	}
)
