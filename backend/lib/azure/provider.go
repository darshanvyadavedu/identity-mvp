package azure

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"user-authentication/config"
	"user-authentication/lib/provider"
)

// Provider implements lib/provider.IdentityProvider using Azure Face API + Document Intelligence.
type Provider struct {
	face *faceClient
	doc  *documentClient
}

// New returns an AzureProvider initialised from config.
func New() provider.IdentityProvider {
	cfg := config.Get()
	return &Provider{
		face: &faceClient{endpoint: cfg.AzureFaceEndpoint, key: cfg.AzureFaceKey},
		doc:  &documentClient{endpoint: cfg.AzureDocEndpoint, key: cfg.AzureDocKey},
	}
}

func (p *Provider) EnsureResources(ctx context.Context) error {
	cfg := config.Get()
	body, _ := json.Marshal(map[string]string{
		"name":             cfg.AzureFaceListID,
		"recognitionModel": "recognition_04",
	})
	_, err := p.face.do(ctx, http.MethodPut,
		"/face/"+faceV10APIVersion+"/facelists/"+cfg.AzureFaceListID, "application/json", body)
	if err != nil && strings.Contains(err.Error(), "409") {
		err = nil // already exists
	}
	if err != nil {
		log.Printf("[azure] face list %q: %v", cfg.AzureFaceListID, err)
	} else {
		log.Printf("[azure] face list %q ready", cfg.AzureFaceListID)
	}
	return nil // non-fatal
}

func (p *Provider) CreateLivenessSession(ctx context.Context, userID string) (*provider.CreateSessionResult, error) {
	return p.face.createLivenessSession(ctx, userID)
}

func (p *Provider) GetLivenessResult(ctx context.Context, providerSessionID string) (*provider.LivenessResult, error) {
	return p.face.getLivenessResult(ctx, providerSessionID)
}

func (p *Provider) CompareFaces(ctx context.Context, refBytes, docBytes []byte) (*provider.CompareFacesResult, error) {
	return p.face.compareFaces(ctx, refBytes, docBytes)
}

func (p *Provider) SearchFacesByImage(ctx context.Context, imgBytes []byte, collectionID string) (*provider.SearchFacesResult, error) {
	return p.face.searchFacesByImage(ctx, imgBytes, collectionID)
}

func (p *Provider) IndexFace(ctx context.Context, imgBytes []byte, collectionID, userID string) (string, error) {
	return p.face.indexFace(ctx, imgBytes, collectionID, userID)
}

func (p *Provider) DeleteFace(ctx context.Context, collectionID, faceID string) error {
	return p.face.deleteFace(ctx, collectionID, faceID)
}

func (p *Provider) AnalyzeID(ctx context.Context, imgBytes []byte) (*provider.DocumentData, []byte, error) {
	return p.doc.analyzeID(ctx, imgBytes)
}
