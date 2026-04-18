package clients

import (
	"context"

	"user-authentication/app/models"
)

// DocumentClientInterface is a provider-agnostic interface for identity document OCR.
// Implementations live in lib/aws (Textract) and lib/azure (Document Intelligence).
type DocumentClientInterface interface {
	// AnalyzeID extracts structured identity fields from an image of an ID document.
	// Returns the parsed fields, the raw provider response bytes (for storage), and any error.
	AnalyzeID(ctx context.Context, imgBytes []byte) (*models.DocumentData, []byte, error)
}
