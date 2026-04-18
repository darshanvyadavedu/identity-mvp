package clients

import "context"

// CreateSessionResult is returned when a liveness session is created.
type CreateSessionResult struct {
	ProviderSessionID string
	AuthToken         string // Azure only; empty for AWS
}

// LivenessResult is returned after polling a liveness session.
type LivenessResult struct {
	Complete       bool    // false if the session is still in progress
	ProviderStatus string  // raw status string from the provider
	Verdict        string  // "live" | "failed" (only valid when Complete=true)
	Confidence     float64 // 0–100 scale
	ReferenceImage string  // data URL for DB storage (may be empty)
	ReferenceBytes []byte  // raw JPEG bytes for face comparison
	RawResponse    []byte
}

// CompareFacesResult holds the outcome of a face comparison.
type CompareFacesResult struct {
	Similarity  float64
	Passed      bool
	RawResponse []byte
}

// SearchFacesResult holds the outcome of a biometric duplicate search.
type SearchFacesResult struct {
	Found         bool
	FaceID        string
	MatchedUserID string
}

// FaceClientInterface is a provider-agnostic interface for liveness and face operations.
// Implementations live in lib/aws and lib/azure.
type FaceClientInterface interface {
	// CreateLivenessSession starts a new liveness check session with the provider.
	CreateLivenessSession(ctx context.Context, userID string) (*CreateSessionResult, error)

	// GetLivenessResult polls the provider for the result of a session.
	// Returns Complete=false if the session is still in progress.
	GetLivenessResult(ctx context.Context, providerSessionID string) (*LivenessResult, error)

	// CompareFaces compares a selfie (ref) against a document face image.
	CompareFaces(ctx context.Context, refBytes, docBytes []byte) (*CompareFacesResult, error)

	// SearchFacesByImage searches a collection/list for a biometric duplicate.
	SearchFacesByImage(ctx context.Context, imgBytes []byte, collectionID string) (*SearchFacesResult, error)

	// DeleteFace removes a face from the collection/list.
	DeleteFace(ctx context.Context, collectionID, faceID string) error

	// IndexFace enrolls a face image in the collection/list and returns the provider face ID.
	IndexFace(ctx context.Context, imgBytes []byte, collectionID, userID string) (string, error)
}
