package handlers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"user-authentication/db"
	"user-authentication/models"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rekognition"
	rektypes "github.com/aws/aws-sdk-go-v2/service/rekognition/types"
	"github.com/julienschmidt/httprouter"
)

const faceMatchThreshold = float32(80.0)

// POST /api/documents
// Header:   X-User-ID: <uuid>
// Form:     sessionId (string) + file (image/jpeg or image/png)
//
// 1. Verify the session is in liveness_passed state.
// 2. OCR the document with Rekognition DetectText → document_scan_results.
// 3. Compare liveness face vs document face → face_match_results.
// 4. Update verification_sessions.decision_status.
func UploadDocument(rekClient *rekognition.Client) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		userID := r.Header.Get("X-User-ID")
		if userID == "" {
			http.Error(w, "X-User-ID header is required", http.StatusBadRequest)
			return
		}

		if err := r.ParseMultipartForm(10 << 20); err != nil {
			http.Error(w, "parse form: "+err.Error(), http.StatusBadRequest)
			return
		}

		internalSessionID := strings.TrimSpace(r.FormValue("sessionId"))
		if internalSessionID == "" {
			http.Error(w, "sessionId is required", http.StatusBadRequest)
			return
		}

		// 1. Load session and verify state.
		var session models.VerificationSession
		if err := db.DB.Where("session_id = ? AND user_id = ?", internalSessionID, userID).
			First(&session).Error; err != nil {
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}
		if session.Status != "liveness_passed" {
			http.Error(w, fmt.Sprintf("liveness check must pass first (current status: %s)", session.Status), http.StatusBadRequest)
			return
		}

		// 2. Load liveness reference image from liveness_results.
		var livenessCheck models.BiometricCheck
		if err := db.DB.Where("session_id = ? AND check_type = ?", internalSessionID, "liveness").
			First(&livenessCheck).Error; err != nil {
			http.Error(w, "liveness check record not found", http.StatusInternalServerError)
			return
		}
		var livenessResult models.LivenessResult
		if err := db.DB.Where("check_id = ?", livenessCheck.CheckID).First(&livenessResult).Error; err != nil {
			http.Error(w, "liveness result not found", http.StatusInternalServerError)
			return
		}
		if livenessResult.ReferenceImage == "" {
			http.Error(w, "no liveness reference image on record", http.StatusBadRequest)
			return
		}
		refBytes, err := dataURLToBytes(livenessResult.ReferenceImage)
		if err != nil {
			http.Error(w, "decode reference image: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// 3. Read uploaded document file.
		file, _, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "file is required: "+err.Error(), http.StatusBadRequest)
			return
		}
		defer file.Close()
		docBytes, err := io.ReadAll(file)
		if err != nil {
			http.Error(w, "read file: "+err.Error(), http.StatusInternalServerError)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		now := time.Now()

		// 4. OCR — Rekognition DetectText.
		var docAttempts int64
		db.DB.Model(&models.BiometricCheck{}).
			Where("session_id = ? AND check_type = ?", internalSessionID, "doc_scan").
			Count(&docAttempts)
		docCheck := models.BiometricCheck{
			SessionID:     internalSessionID,
			UserID:        userID,
			CheckType:     "doc_scan",
			Status:        "pending",
			AttemptNumber: int(docAttempts) + 1,
			AttemptedAt:   &now,
		}
		db.DB.Create(&docCheck)

		docData, rawDocJSON, err := analyzeDocument(ctx, rekClient, docBytes)
		docStatus := "succeeded"
		if err != nil {
			docStatus = "failed"
		}
		db.DB.Model(&docCheck).Update("status", docStatus)

		extractedJSON, _ := json.Marshal(docData)
		docScanResult := models.DocumentScanResult{
			CheckID:         docCheck.CheckID,
			IssuingCountry:  docData.IssuingCountry,
			IDNumberHMAC:    computeHMAC(docData.IDNumber, os.Getenv("HMAC_SECRET")),
			ExtractedFields: extractedJSON,
			RawResponse:     rawDocJSON,
		}
		db.DB.Create(&docScanResult)

		// 5. Face match — Rekognition CompareFaces.
		var fmAttempts int64
		db.DB.Model(&models.BiometricCheck{}).
			Where("session_id = ? AND check_type = ?", internalSessionID, "face_match").
			Count(&fmAttempts)
		fmCheck := models.BiometricCheck{
			SessionID:     internalSessionID,
			UserID:        userID,
			CheckType:     "face_match",
			Status:        "pending",
			AttemptNumber: int(fmAttempts) + 1,
			AttemptedAt:   &now,
		}
		db.DB.Create(&fmCheck)

		similarity, rawFMJSON, fmErr := compareFaces(ctx, rekClient, refBytes, docBytes)
		passed := fmErr == nil && similarity >= float64(faceMatchThreshold)
		fmStatus := "succeeded"
		if fmErr != nil {
			fmStatus = "failed"
		}
		db.DB.Model(&fmCheck).Update("status", fmStatus)

		rawFmJSON, _ := json.Marshal(map[string]any{"similarity": similarity, "passed": passed})
		_ = rawFmJSON
		faceMatchResult := models.FaceMatchResult{
			CheckID:     fmCheck.CheckID,
			Confidence:  similarity / 100.0,
			Threshold:   float64(faceMatchThreshold) / 100.0,
			Passed:      passed,
			SourceA:     "liveness_frame",
			SourceB:     "id_document",
			RawResponse: rawFMJSON,
		}
		db.DB.Create(&faceMatchResult)

		// 6. Update session decision.
		decisionStatus := "verified"
		if !passed {
			decisionStatus = "failed"
		}
		db.DB.Model(&session).Updates(map[string]any{
			"status":          "completed",
			"decision_status": decisionStatus,
		})

		// 7. Audit log.
		writeAudit(userID, "document_verified", internalSessionID, map[string]any{
			"faceMatchPassed": passed,
			"similarity":      similarity,
			"decisionStatus":  decisionStatus,
		})

		WriteJSON(w, http.StatusOK, map[string]any{
			"sessionId":      internalSessionID,
			"decisionStatus": decisionStatus,
			"document":       docData,
			"faceMatch": map[string]any{
				"similarity": similarity,
				"passed":     passed,
				"threshold":  faceMatchThreshold,
			},
			"issuingCountry": docData.IssuingCountry,
		})
	}
}

// ── AWS helpers ───────────────────────────────────────────────────────────────

type DocumentData struct {
	FirstName      string `json:"firstName,omitempty"`
	LastName       string `json:"lastName,omitempty"`
	DOB            string `json:"dob,omitempty"`
	IDNumber       string `json:"idNumber,omitempty"`
	Expiry         string `json:"expiry,omitempty"`
	IssuingCountry string `json:"issuingCountry,omitempty"`
}

// computeHMAC returns HMAC-SHA256 of value keyed by secret, hex-encoded.
func computeHMAC(value, secret string) string {
	if value == "" || secret == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil))
}

var (
	reDate   = regexp.MustCompile(`\b(\d{1,2}[/.\-]\d{1,2}[/.\-]\d{2,4}|\d{4}[/.\-]\d{2}[/.\-]\d{2})\b`)
	reDocNum = regexp.MustCompile(`\b([A-Z]{1,2}\d{6,9}|\d{9})\b`)
	reMRZ    = regexp.MustCompile(`^[A-Z0-9<]{20,44}$`)
)

func analyzeDocument(ctx context.Context, client *rekognition.Client, imgBytes []byte) (*DocumentData, []byte, error) {
	out, err := client.DetectText(ctx, &rekognition.DetectTextInput{
		Image: &rektypes.Image{Bytes: imgBytes},
	})
	if err != nil {
		return nil, nil, err
	}

	raw, _ := json.Marshal(out)

	var lines []string
	for _, d := range out.TextDetections {
		if d.Type == rektypes.TextTypesLine {
			lines = append(lines, aws.ToString(d.DetectedText))
		}
	}

	return parseIDFields(lines), raw, nil
}

func parseIDFields(lines []string) *DocumentData {
	doc := &DocumentData{}
	var dates, mrzLines []string

	for _, line := range lines {
		upper := strings.ToUpper(strings.TrimSpace(line))
		for _, m := range reDate.FindAllString(upper, -1) {
			dates = append(dates, m)
		}
		if doc.IDNumber == "" {
			if m := reDocNum.FindString(upper); m != "" {
				doc.IDNumber = m
			}
		}
		if reMRZ.MatchString(upper) {
			mrzLines = append(mrzLines, upper)
		}
	}

	if len(dates) > 0 {
		doc.DOB = dates[0]
	}
	if len(dates) > 1 {
		doc.Expiry = dates[len(dates)-1]
	}
	if len(mrzLines) >= 2 {
		parseMRZ(mrzLines, doc)
	}
	return doc
}

func parseMRZ(lines []string, doc *DocumentData) {
	if len(lines[0]) >= 5 {
		raw := strings.TrimRight(lines[0][2:5], "<")
		if raw != "" {
			doc.IssuingCountry = raw
		}
	}
	if len(lines[0]) >= 44 {
		namePart := lines[0][5:44]
		parts := strings.SplitN(namePart, "<<", 2)
		if len(parts) == 2 {
			doc.LastName = strings.TrimSpace(strings.ReplaceAll(parts[0], "<", " "))
			doc.FirstName = strings.TrimSpace(strings.ReplaceAll(parts[1], "<", " "))
		}
	}
	if len(lines) >= 2 && len(lines[1]) >= 27 {
		l2 := lines[1]
		if doc.IDNumber == "" {
			doc.IDNumber = strings.TrimRight(l2[0:9], "<")
		}
		doc.DOB = formatMRZDate(l2[13:19])
		doc.Expiry = formatMRZDate(l2[21:27])
	}
}

func formatMRZDate(s string) string {
	if len(s) != 6 {
		return s
	}
	return s[4:6] + "/" + s[2:4] + "/" + s[0:2]
}

func compareFaces(ctx context.Context, client *rekognition.Client, srcBytes, tgtBytes []byte) (float64, []byte, error) {
	out, err := client.CompareFaces(ctx, &rekognition.CompareFacesInput{
		SourceImage:         &rektypes.Image{Bytes: srcBytes},
		TargetImage:         &rektypes.Image{Bytes: tgtBytes},
		SimilarityThreshold: aws.Float32(faceMatchThreshold),
	})
	if err != nil {
		return 0, nil, err
	}

	raw, _ := json.Marshal(out)

	if len(out.FaceMatches) == 0 {
		return 0, raw, nil
	}
	return float64(aws.ToFloat32(out.FaceMatches[0].Similarity)), raw, nil
}

func dataURLToBytes(dataURL string) ([]byte, error) {
	idx := strings.Index(dataURL, ",")
	if idx == -1 {
		return nil, fmt.Errorf("invalid data URL")
	}
	return base64.StdEncoding.DecodeString(dataURL[idx+1:])
}

// WriteJSON is shared across handlers.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}
