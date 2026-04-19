package aws

import (
	"context"
	"log"

	"user-authentication/config"
	"user-authentication/lib/provider"
)

// Provider implements lib/provider.IdentityProvider using AWS Rekognition + Textract.
type Provider struct {
	face *faceClient
	doc  *documentClient
}

// New returns an AWSProvider initialised from config.
func New() (provider.IdentityProvider, error) {
	cfg := config.Get()
	f, err := newFaceClient(cfg.AWSRegion)
	if err != nil {
		return nil, err
	}
	d, err := newDocumentClient(cfg.AWSRegion)
	if err != nil {
		return nil, err
	}
	return &Provider{face: f, doc: d}, nil
}

func (p *Provider) EnsureResources(ctx context.Context) error {
	collectionID := config.Get().RekognitionCollectionID
	err := p.face.ensureCollection(ctx, collectionID)
	if err != nil {
		log.Printf("[aws] rekognition collection %q: %v (ignored if already exists)", collectionID, err)
	} else {
		log.Printf("[aws] rekognition collection %q ready", collectionID)
	}
	return nil // non-fatal; collection may already exist
}

func (p *Provider) CreateLivenessSession(ctx context.Context, userID string) (*provider.CreateSessionResult, error) {
	return p.face.createLivenessSession(ctx)
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
