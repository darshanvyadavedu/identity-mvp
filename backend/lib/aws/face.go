package aws

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log"
	"strings"

	"user-authentication/lib/provider"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/rekognition"
	rektypes "github.com/aws/aws-sdk-go-v2/service/rekognition/types"
)

const (
	compareFaceThreshold = float32(80.0)
	searchFaceThreshold  = float32(95.0)
)

type faceClient struct {
	client *rekognition.Client
}

func newFaceClient(region string) (*faceClient, error) {
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(region))
	if err != nil {
		return nil, err
	}
	return &faceClient{client: rekognition.NewFromConfig(cfg)}, nil
}

func (c *faceClient) ensureCollection(ctx context.Context, collectionID string) error {
	_, err := c.client.CreateCollection(ctx, &rekognition.CreateCollectionInput{
		CollectionId: aws.String(collectionID),
	})
	return err
}

func (c *faceClient) createLivenessSession(ctx context.Context) (*provider.CreateSessionResult, error) {
	out, err := c.client.CreateFaceLivenessSession(ctx, &rekognition.CreateFaceLivenessSessionInput{})
	if err != nil {
		return nil, err
	}
	return &provider.CreateSessionResult{
		ProviderSessionID: aws.ToString(out.SessionId),
	}, nil
}

func (c *faceClient) getLivenessResult(ctx context.Context, providerSessionID string) (*provider.LivenessResult, error) {
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

	return &provider.LivenessResult{
		Complete:       true,
		ProviderStatus: awsStatus,
		Verdict:        verdict,
		Confidence:     confidence,
		ReferenceImage: refDataURL,
		ReferenceBytes: refBytes,
		RawResponse:    rawJSON,
	}, nil
}

func (c *faceClient) compareFaces(ctx context.Context, refBytes, docBytes []byte) (*provider.CompareFacesResult, error) {
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
		return &provider.CompareFacesResult{RawResponse: raw}, nil
	}
	similarity := float64(aws.ToFloat32(out.FaceMatches[0].Similarity))
	return &provider.CompareFacesResult{
		Similarity:  similarity,
		Passed:      similarity >= float64(compareFaceThreshold),
		RawResponse: raw,
	}, nil
}

func (c *faceClient) searchFacesByImage(ctx context.Context, imgBytes []byte, collectionID string) (*provider.SearchFacesResult, error) {
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
		return &provider.SearchFacesResult{Found: false}, nil
	}
	match := out.FaceMatches[0]
	return &provider.SearchFacesResult{
		Found:         true,
		FaceID:        aws.ToString(match.Face.FaceId),
		MatchedUserID: aws.ToString(match.Face.ExternalImageId),
	}, nil
}

func (c *faceClient) deleteFace(ctx context.Context, collectionID, faceID string) error {
	_, err := c.client.DeleteFaces(ctx, &rekognition.DeleteFacesInput{
		CollectionId: aws.String(collectionID),
		FaceIds:      []string{faceID},
	})
	return err
}

func (c *faceClient) indexFace(ctx context.Context, imgBytes []byte, collectionID, userID string) (string, error) {
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

// Silence unused import warning for log if not used elsewhere.
var _ = log.Printf
