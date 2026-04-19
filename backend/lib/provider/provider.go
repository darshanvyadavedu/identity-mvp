package provider

import "context"

// IdentityProvider is the single interface every identity provider must implement.
// To add a new provider (Google, Onfido, etc.) implement this interface in lib/<name>/.
type IdentityProvider interface {
	// CreateLivenessSession starts a liveness session and returns the tokens
	// the frontend SDK needs to run the check.
	CreateLivenessSession(ctx context.Context, userID string) (*CreateSessionResult, error)

	// GetLivenessResult polls the provider for the outcome of a session.
	// Returns Complete=false while the session is still in progress.
	GetLivenessResult(ctx context.Context, providerSessionID string) (*LivenessResult, error)

	// CompareFaces compares a selfie (ref) against a document face image.
	CompareFaces(ctx context.Context, refBytes, docBytes []byte) (*CompareFacesResult, error)

	// SearchFacesByImage searches a collection/list for a biometric duplicate.
	SearchFacesByImage(ctx context.Context, imgBytes []byte, collectionID string) (*SearchFacesResult, error)

	// IndexFace enrolls a face in the collection/list and returns the provider face ID.
	IndexFace(ctx context.Context, imgBytes []byte, collectionID, userID string) (string, error)

	// DeleteFace removes a face from the collection/list.
	DeleteFace(ctx context.Context, collectionID, faceID string) error

	// AnalyzeID extracts structured identity fields from an ID document image.
	AnalyzeID(ctx context.Context, imgBytes []byte) (*DocumentData, []byte, error)

	// EnsureResources creates provider-specific resources (face collections,
	// face lists, etc.) on first use. Implementations must be idempotent.
	EnsureResources(ctx context.Context) error
}

// ── Result types ──────────────────────────────────────────────────────────────

type CreateSessionResult struct {
	ProviderSessionID string
	AuthToken         string // Azure only; empty for AWS
}

type LivenessResult struct {
	Complete       bool    // false = session still in progress
	ProviderStatus string  // raw status string from the provider
	Verdict        string  // "live" | "failed"
	Confidence     float64 // 0–100
	ReferenceImage string  // data URL for DB storage
	ReferenceBytes []byte  // raw JPEG for face comparison
	RawResponse    []byte
}

type CompareFacesResult struct {
	Similarity  float64
	Passed      bool
	RawResponse []byte
}

type SearchFacesResult struct {
	Found         bool
	FaceID        string
	MatchedUserID string
}

// DocumentData holds structured fields extracted from an identity document.
type DocumentData struct {
	FirstName      string `json:"firstName,omitempty"`
	LastName       string `json:"lastName,omitempty"`
	DOB            string `json:"dob,omitempty"`
	IDNumber       string `json:"idNumber,omitempty"`
	Expiry         string `json:"expiry,omitempty"`
	IssuingCountry string `json:"issuingCountry,omitempty"`
	Address        string `json:"address,omitempty"`
	DocumentType   string `json:"documentType,omitempty"`
}
