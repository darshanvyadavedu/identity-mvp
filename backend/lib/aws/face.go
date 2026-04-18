package aws

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log"
	"strings"
	"sync"

	"user-authentication/app/clients"
	"user-authentication/config"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rekognition"
	rektypes "github.com/aws/aws-sdk-go-v2/service/rekognition/types"
)

const (
	compareFaceThreshold = float32(80.0)
	searchFaceThreshold  = float32(95.0)
)

var (
	faceOnce     sync.Once
	faceInstance *FaceClient
)

// FaceClient implements clients.FaceClientInterface using AWS Rekognition.
type FaceClient struct {
	client *rekognition.Client
}

// FaceClientOption configures a FaceClient.
type FaceClientOption func(*FaceClient)

// NewFaceClient returns the singleton FaceClient, initialising it once from config.
func NewFaceClient(opts ...FaceClientOption) clients.FaceClientInterface {
	faceOnce.Do(func() {
		cfg, err := awsconfig.LoadDefaultConfig(context.Background(),
			awsconfig.WithRegion(config.Get().AWSRegion))
		if err != nil {
			log.Fatalf("aws: load config for rekognition: %v", err)
		}
		faceInstance = &FaceClient{client: rekognition.NewFromConfig(cfg)}
	})
	for _, opt := range opts {
		opt(faceInstance)
	}
	return faceInstance
}

// EnsureCollection creates the Rekognition face collection if it doesn't exist.
// Call once at startup; safe to call even if the collection already exists.
func EnsureCollection(ctx context.Context) error {
	c := NewFaceClient().(*FaceClient)
	collectionID := config.Get().RekognitionCollectionID
	_, err := c.client.CreateCollection(ctx, &rekognition.CreateCollectionInput{
		CollectionId: aws.String(collectionID),
	})
	return err
}

func (c *FaceClient) CreateLivenessSession(ctx context.Context, _ string) (*clients.CreateSessionResult, error) {
	out, err := c.client.CreateFaceLivenessSession(ctx, &rekognition.CreateFaceLivenessSessionInput{})
	if err != nil {
		return nil, err
	}
	return &clients.CreateSessionResult{
		ProviderSessionID: aws.ToString(out.SessionId),
	}, nil
}

func (c *FaceClient) GetLivenessResult(ctx context.Context, providerSessionID string) (*clients.LivenessResult, error) {
	out, err := c.client.GetFaceLivenessSessionResults(ctx, &rekognition.GetFaceLivenessSessionResultsInput{
		SessionId: aws.String(providerSessionID),
	})
	if err != nil {
		return nil, err
	}

	awsStatus := strings.ToLower(string(out.Status))
	confidence := float64(aws.ToFloat32(out.Confidence))
	rawJSON, _ := json.Marshal(out)

	verdict := awsStatus
	switch awsStatus {
	case "succeeded":
		verdict = "live"
	case "failed":
		verdict = "failed"
	}

	var refDataURL string
	var refBytes []byte
	if out.ReferenceImage != nil && len(out.ReferenceImage.Bytes) > 0 {
		refBytes = out.ReferenceImage.Bytes
		refDataURL = "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(refBytes)
	}

	return &clients.LivenessResult{
		Complete:       true,
		ProviderStatus: awsStatus,
		Verdict:        verdict,
		Confidence:     confidence,
		ReferenceImage: refDataURL,
		ReferenceBytes: refBytes,
		RawResponse:    rawJSON,
	}, nil
}

func (c *FaceClient) CompareFaces(ctx context.Context, refBytes, docBytes []byte) (*clients.CompareFacesResult, error) {
	out, err := c.client.CompareFaces(ctx, &rekognition.CompareFacesInput{
		SourceImage:         &rektypes.Image{Bytes: refBytes},
		TargetImage:         &rektypes.Image{Bytes: docBytes},
		SimilarityThreshold: aws.Float32(compareFaceThreshold),
	})
	if err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(out)
	if len(out.FaceMatches) == 0 {
		return &clients.CompareFacesResult{RawResponse: raw}, nil
	}
	similarity := float64(aws.ToFloat32(out.FaceMatches[0].Similarity))
	return &clients.CompareFacesResult{
		Similarity:  similarity,
		Passed:      similarity >= float64(compareFaceThreshold),
		RawResponse: raw,
	}, nil
}

func (c *FaceClient) SearchFacesByImage(ctx context.Context, imgBytes []byte, collectionID string) (*clients.SearchFacesResult, error) {
	out, err := c.client.SearchFacesByImage(ctx, &rekognition.SearchFacesByImageInput{
		CollectionId:       aws.String(collectionID),
		Image:              &rektypes.Image{Bytes: imgBytes},
		FaceMatchThreshold: aws.Float32(searchFaceThreshold),
		MaxFaces:           aws.Int32(1),
	})
	if err != nil {
		return nil, err
	}
	if len(out.FaceMatches) == 0 {
		return &clients.SearchFacesResult{Found: false}, nil
	}
	match := out.FaceMatches[0]
	return &clients.SearchFacesResult{
		Found:         true,
		FaceID:        aws.ToString(match.Face.FaceId),
		MatchedUserID: aws.ToString(match.Face.ExternalImageId),
	}, nil
}

func (c *FaceClient) DeleteFace(ctx context.Context, collectionID, faceID string) error {
	_, err := c.client.DeleteFaces(ctx, &rekognition.DeleteFacesInput{
		CollectionId: aws.String(collectionID),
		FaceIds:      []string{faceID},
	})
	return err
}

func (c *FaceClient) IndexFace(ctx context.Context, imgBytes []byte, collectionID, userID string) (string, error) {
	out, err := c.client.IndexFaces(ctx, &rekognition.IndexFacesInput{
		CollectionId:        aws.String(collectionID),
		Image:               &rektypes.Image{Bytes: imgBytes},
		ExternalImageId:     aws.String(userID),
		MaxFaces:            aws.Int32(1),
		DetectionAttributes: []rektypes.Attribute{},
	})
	if err != nil {
		return "", err
	}
	if len(out.FaceRecords) == 0 {
		return "", nil
	}
	return aws.ToString(out.FaceRecords[0].Face.FaceId), nil
}
