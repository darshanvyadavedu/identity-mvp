package aws

import (
	"context"
	"encoding/json"
	"log"
	"strings"

	"user-authentication/lib/provider"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/textract"
	txttypes "github.com/aws/aws-sdk-go-v2/service/textract/types"
)

const minConfidence = float64(60)

type documentClient struct {
	client *textract.Client
}

func newDocumentClient(region string) (*documentClient, error) {
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(region))
	if err != nil {
		return nil, err
	}
	return &documentClient{client: textract.NewFromConfig(cfg)}, nil
}

func (c *documentClient) analyzeID(ctx context.Context, imgBytes []byte) (*provider.DocumentData, []byte, error) {
	out, err := c.client.AnalyzeID(ctx, &textract.AnalyzeIDInput{
		DocumentPages: []txttypes.Document{
			{Bytes: imgBytes},
		},
	})
	if err != nil {
		return &provider.DocumentData{}, nil, err
	}

	raw, _ := json.Marshal(out)
	doc := &provider.DocumentData{}

	if len(out.IdentityDocuments) == 0 {
		log.Printf("[textract] 0 identity documents returned")
		return doc, raw, nil
	}

	fields := out.IdentityDocuments[0].IdentityDocumentFields
	log.Printf("[textract] %d field(s) returned", len(fields))

	for _, field := range fields {
		if field.Type == nil || field.ValueDetection == nil {
			continue
		}
		key := aws.ToString(field.Type.Text)
		val := strings.TrimSpace(aws.ToString(field.ValueDetection.Text))
		conf := float64(aws.ToFloat32(field.ValueDetection.Confidence))

		log.Printf("[textract]   %-25s = %-30q (%.1f%%)", key, val, conf)

		if val == "" || conf < minConfidence {
			continue
		}
		switch key {
		case "FIRST_NAME":
			doc.FirstName = val
		case "LAST_NAME":
			doc.LastName = val
		case "DATE_OF_BIRTH":
			doc.DOB = val
		case "DOCUMENT_NUMBER":
			doc.IDNumber = val
		case "DATE_OF_EXPIRY", "EXPIRATION_DATE":
			doc.Expiry = val
		case "COUNTY", "COUNTRY", "PLACE_OF_BIRTH":
			if doc.IssuingCountry == "" {
				doc.IssuingCountry = val
			}
		case "ADDRESS":
			doc.Address = val
		case "ID_TYPE":
			doc.DocumentType = val
		}
	}

	log.Printf("[textract] extracted: %+v", doc)
	return doc, raw, nil
}
