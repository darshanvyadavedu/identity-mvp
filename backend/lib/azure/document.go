package azure

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"user-authentication/app/clients"
	"user-authentication/app/models"
	"user-authentication/config"
)

const docIntelAPIVersion = "2024-02-29-preview"

// DocumentClient implements clients.DocumentClientInterface using Azure Document Intelligence.
type DocumentClient struct {
	endpoint string
	key      string
}

// DocumentClientOption configures a DocumentClient.
type DocumentClientOption func(*DocumentClient)

// NewDocumentClient creates an Azure DocumentClient reading credentials from config.
func NewDocumentClient(opts ...DocumentClientOption) clients.DocumentClientInterface {
	cfg := config.Get()
	c := &DocumentClient{endpoint: cfg.AzureDocEndpoint, key: cfg.AzureDocKey}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *DocumentClient) AnalyzeID(ctx context.Context, imgBytes []byte) (*models.DocumentData, []byte, error) {
	// 1. Submit analysis job.
	url := fmt.Sprintf("%s/documentintelligence/documentModels/prebuilt-idDocument:analyze?api-version=%s",
		strings.TrimRight(c.endpoint, "/"), docIntelAPIVersion)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(imgBytes))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Ocp-Apim-Subscription-Key", c.key)
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		return nil, nil, fmt.Errorf("document intelligence submit: status %d", resp.StatusCode)
	}

	operationURL := resp.Header.Get("Operation-Location")
	if operationURL == "" {
		return nil, nil, fmt.Errorf("document intelligence: missing Operation-Location header")
	}

	// 2. Poll until succeeded (up to 30 seconds).
	var rawResult []byte
	for i := 0; i < 15; i++ {
		time.Sleep(2 * time.Second)

		pollReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, operationURL, nil)
		pollReq.Header.Set("Ocp-Apim-Subscription-Key", c.key)

		pollResp, err := http.DefaultClient.Do(pollReq)
		if err != nil {
			return nil, nil, err
		}
		rawResult, _ = io.ReadAll(pollResp.Body)
		pollResp.Body.Close()

		var status struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal(rawResult, &status); err != nil {
			return nil, nil, err
		}

		switch status.Status {
		case "succeeded":
			return parseDocResult(rawResult)
		case "failed":
			return nil, rawResult, fmt.Errorf("document intelligence analysis failed")
		}
		// "running" or "notStarted" — keep polling.
	}

	return nil, nil, fmt.Errorf("document intelligence: timed out waiting for result")
}

func parseDocResult(raw []byte) (*models.DocumentData, []byte, error) {
	var result struct {
		AnalyzeResult struct {
			Documents []struct {
				DocType string `json:"docType"`
				Fields  map[string]struct {
					ValueString string `json:"valueString"`
					Content     string `json:"content"`
				} `json:"fields"`
			} `json:"documents"`
		} `json:"analyzeResult"`
	}

	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, raw, err
	}

	doc := &models.DocumentData{}
	if len(result.AnalyzeResult.Documents) == 0 {
		return doc, raw, nil
	}

	d := result.AnalyzeResult.Documents[0]
	doc.DocumentType = d.DocType

	get := func(key string) string {
		if f, ok := d.Fields[key]; ok {
			if f.ValueString != "" {
				return f.ValueString
			}
			return f.Content
		}
		return ""
	}

	doc.FirstName = get("FirstName")
	doc.LastName = get("LastName")
	doc.DOB = get("DateOfBirth")
	doc.IDNumber = get("DocumentNumber")
	doc.Expiry = get("DateOfExpiration")
	doc.IssuingCountry = get("CountryRegion")
	doc.Address = get("Address")

	return doc, raw, nil
}
