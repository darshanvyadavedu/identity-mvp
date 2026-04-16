package azure

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DocIntelClient wraps Azure Document Intelligence calls.
type DocIntelClient struct {
	Endpoint string
	Key      string
}

const docIntelAPIVersion = "2024-02-29-preview"

// DocumentFields holds the extracted identity fields from prebuilt-idDocument.
type DocumentFields struct {
	FirstName      string
	LastName       string
	DOB            string
	DocumentNumber string
	Expiry         string
	Country        string
	Address        string
	DocumentType   string
}

// AnalyzeID sends the image to Document Intelligence prebuilt-idDocument model
// and polls until the result is ready (up to 30 seconds).
func (c *DocIntelClient) AnalyzeID(ctx context.Context, imgBytes []byte) (*DocumentFields, []byte, error) {
	// 1. Submit analysis job.
	url := fmt.Sprintf("%s/documentintelligence/documentModels/prebuilt-idDocument:analyze?api-version=%s",
		c.Endpoint, docIntelAPIVersion)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(imgBytes))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Ocp-Apim-Subscription-Key", c.Key)
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		return nil, nil, fmt.Errorf("document intelligence submit: status %d", resp.StatusCode)
	}

	// 2. Poll Operation-Location until succeeded.
	operationURL := resp.Header.Get("Operation-Location")
	if operationURL == "" {
		return nil, nil, fmt.Errorf("document intelligence: missing Operation-Location header")
	}

	var rawResult []byte
	for i := 0; i < 15; i++ {
		time.Sleep(2 * time.Second)

		pollReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, operationURL, nil)
		pollReq.Header.Set("Ocp-Apim-Subscription-Key", c.Key)

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
		// "running" or "notStarted" — keep polling
	}

	return nil, nil, fmt.Errorf("document intelligence: timed out waiting for result")
}

// parseDocResult extracts identity fields from the Document Intelligence response JSON.
func parseDocResult(raw []byte) (*DocumentFields, []byte, error) {
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

	fields := &DocumentFields{}
	if len(result.AnalyzeResult.Documents) == 0 {
		return fields, raw, nil
	}

	doc := result.AnalyzeResult.Documents[0]
	fields.DocumentType = doc.DocType

	get := func(key string) string {
		if f, ok := doc.Fields[key]; ok {
			if f.ValueString != "" {
				return f.ValueString
			}
			return f.Content
		}
		return ""
	}

	fields.FirstName      = get("FirstName")
	fields.LastName       = get("LastName")
	fields.DOB            = get("DateOfBirth")
	fields.DocumentNumber = get("DocumentNumber")
	fields.Expiry         = get("DateOfExpiration")
	fields.Country        = get("CountryRegion")
	fields.Address        = get("Address")

	return fields, raw, nil
}
