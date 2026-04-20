package easyocr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"

	"user-authentication/lib/provider"
)

type documentClient struct {
	serviceURL string
}

type analyzeResponse struct {
	FirstName      string   `json:"firstName"`
	LastName       string   `json:"lastName"`
	DOB            string   `json:"dob"`
	IDNumber       string   `json:"idNumber"`
	Expiry         string   `json:"expiry"`
	IssuingCountry string   `json:"issuingCountry"`
	Address        string   `json:"address"`
	DocumentType   string   `json:"documentType"`
	RawText        []string `json:"rawText"`
	RawOCR         []any    `json:"rawOCR"`
}

func (c *documentClient) analyzeID(ctx context.Context, imgBytes []byte) (*provider.DocumentData, []byte, error) {
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)

	part, err := w.CreateFormFile("file", "document.jpg")
	if err != nil {
		return nil, nil, fmt.Errorf("easyocr: create form file: %w", err)
	}
	if _, err = part.Write(imgBytes); err != nil {
		return nil, nil, fmt.Errorf("easyocr: write image bytes: %w", err)
	}
	w.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.serviceURL+"/analyze", body)
	if err != nil {
		return nil, nil, fmt.Errorf("easyocr: build request: %w", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("easyocr: request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("easyocr: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return &provider.DocumentData{}, raw, fmt.Errorf("easyocr: service returned %d: %s", resp.StatusCode, raw)
	}

	var result analyzeResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		return &provider.DocumentData{}, raw, fmt.Errorf("easyocr: decode response: %w", err)
	}

	doc := &provider.DocumentData{
		FirstName:      result.FirstName,
		LastName:       result.LastName,
		DOB:            result.DOB,
		IDNumber:       result.IDNumber,
		Expiry:         result.Expiry,
		IssuingCountry: result.IssuingCountry,
		Address:        result.Address,
		DocumentType:   result.DocumentType,
	}
	return doc, raw, nil
}
