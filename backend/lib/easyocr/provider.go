// Package easyocr provides a DocumentProvider that uses a local EasyOCR
// Python microservice for document text extraction (AnalyzeID).
//
// Configure with DOC_PROVIDER=easyocr and EASYOCR_SERVICE_URL (default: http://localhost:8090).
// Pair with PROVIDER=aws or PROVIDER=azure for liveness and face-matching.
package easyocr

import (
	"context"

	"user-authentication/config"
	"user-authentication/lib/provider"
)

// Provider implements lib/provider.DocumentProvider using an EasyOCR microservice.
type Provider struct {
	doc *documentClient
}

// New returns a Provider configured from environment variables.
func New() provider.DocumentProvider {
	cfg := config.Get()
	return &Provider{
		doc: &documentClient{serviceURL: cfg.EasyOCRServiceURL},
	}
}

func (p *Provider) AnalyzeID(ctx context.Context, imgBytes []byte) (*provider.DocumentData, []byte, error) {
	return p.doc.analyzeID(ctx, imgBytes)
}
