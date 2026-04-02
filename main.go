package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"regexp"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"

	"github.com/aws/aws-sdk-go-v2/service/rekognition"
	rektypes "github.com/aws/aws-sdk-go-v2/service/rekognition/types"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/julienschmidt/httprouter"
)

// ── Models ────────────────────────────────────────────────────────────────────

type DocumentData struct {
	FirstName string `json:"firstName,omitempty"`
	LastName  string `json:"lastName,omitempty"`
	DOB       string `json:"dob,omitempty"`
	IDNumber  string `json:"idNumber,omitempty"`
	Expiry    string `json:"expiry,omitempty"`
	Address   string `json:"address,omitempty"`
}

type FaceMatch struct {
	Similarity float64 `json:"similarity"`
	Passed     bool    `json:"passed"`
}

type User struct {
	ID                 string        `json:"userId"`
	Username           string        `json:"username"`
	SessionID          string        `json:"sessionId"`
	VerificationStatus string        `json:"verificationStatus"` // pending | liveness_passed | verified | failed
	LivenessConfidence float64       `json:"livenessConfidence,omitempty"`
	ReferenceImage     string        `json:"referenceImage,omitempty"` // data:image/jpeg;base64,...
	Document           *DocumentData `json:"document,omitempty"`
	FaceMatch          *FaceMatch    `json:"faceMatch,omitempty"`
	CreatedAt          time.Time     `json:"createdAt"`
	UpdatedAt          time.Time     `json:"updatedAt"`
}

// ── In-memory store ───────────────────────────────────────────────────────────

var (
	users         = map[string]*User{}
	sessionToUser = map[string]string{}
	storeMu       sync.RWMutex
)

// ── Config ────────────────────────────────────────────────────────────────────

func main() {
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		log.Printf("warning: could not load .env: %v", err)
	}

	port := 8080
	if v := os.Getenv("PORT"); v != "" {
		fmt.Sscanf(v, "%d", &port)
	}
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-1"
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(), awsconfig.WithRegion(region))
	if err != nil {
		log.Fatalf("aws config: %v", err)
	}
	rekClient := rekognition.NewFromConfig(awsCfg)

	router := httprouter.New()

	// ── Health ────────────────────────────────────────────────────────────────
	router.GET("/", func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintln(w, "Endpoints:\n  POST /api/sessions\n  GET  /api/sessions/:sessionId/result\n  POST /api/documents\n  GET  /api/users\n  GET  /api/users/:userId")
	})

	// ── POST /api/sessions ────────────────────────────────────────────────────
	// Body: { "username": "alice" }
	// Creates an AWS Rekognition liveness session + user record.
	router.POST("/api/sessions", func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		var body struct {
			Username string `json:"username"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Username) == "" {
			http.Error(w, `body must be JSON with a non-empty "username"`, http.StatusBadRequest)
			return
		}
		username := strings.TrimSpace(body.Username)

		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()

		out, err := rekClient.CreateFaceLivenessSession(ctx, &rekognition.CreateFaceLivenessSessionInput{})
		if err != nil {
			http.Error(w, fmt.Sprintf("create session: %v", err), http.StatusBadGateway)
			return
		}

		user := &User{
			ID:                 uuid.NewString(),
			Username:           username,
			SessionID:          aws.ToString(out.SessionId),
			VerificationStatus: "pending",
			CreatedAt:          time.Now(),
			UpdatedAt:          time.Now(),
		}

		storeMu.Lock()
		users[user.ID] = user
		sessionToUser[user.SessionID] = user.ID
		storeMu.Unlock()

		log.Printf("session created  sessionId=%s userId=%s username=%s", user.SessionID, user.ID, user.Username)
		writeJSON(w, http.StatusCreated, map[string]string{
			"sessionId": user.SessionID,
			"userId":    user.ID,
			"username":  user.Username,
		})
	})

	// ── GET /api/sessions/:sessionId/result ───────────────────────────────────
	// Fetches liveness result from AWS, updates user record.
	router.GET("/api/sessions/:sessionId/result", func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		sessionID := ps.ByName("sessionId")
		if sessionID == "" || strings.Contains(sessionID, "/") {
			http.Error(w, "invalid sessionId", http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()

		out, err := rekClient.GetFaceLivenessSessionResults(ctx, &rekognition.GetFaceLivenessSessionResultsInput{
			SessionId: aws.String(sessionID),
		})
		if err != nil {
			http.Error(w, fmt.Sprintf("get liveness results: %v", err), http.StatusBadGateway)
			return
		}

		var refImage string
		if out.ReferenceImage != nil && len(out.ReferenceImage.Bytes) > 0 {
			refImage = "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(out.ReferenceImage.Bytes)
		}

		livenessStatus := strings.ToLower(string(out.Status))
		confidence := float64(aws.ToFloat32(out.Confidence))

		verificationStatus := "pending"
		if livenessStatus == "succeeded" {
			verificationStatus = "liveness_passed"
		} else if livenessStatus == "failed" {
			verificationStatus = "failed"
		}

		storeMu.Lock()
		if uid, ok := sessionToUser[sessionID]; ok {
			if u, ok := users[uid]; ok {
				u.VerificationStatus = verificationStatus
				u.LivenessConfidence = confidence
				u.ReferenceImage = refImage
				u.UpdatedAt = time.Now()
			}
		}
		storeMu.Unlock()

		storeMu.RLock()
		u := users[sessionToUser[sessionID]]
		storeMu.RUnlock()

		log.Printf("liveness result  sessionId=%s status=%s confidence=%.1f", sessionID, livenessStatus, confidence)
		writeJSON(w, http.StatusOK, map[string]any{
			"sessionId":          sessionID,
			"livenessStatus":     livenessStatus,
			"livenessConfidence": confidence,
			"referenceImage":     refImage,
			"user":               u,
		})
	})

	// ── POST /api/documents ───────────────────────────────────────────────────
	// Multipart: userId (string) + file (image of ID document).
	// 1. Textract AnalyzeID  — extract name, DOB, ID number, expiry.
	// 2. Rekognition CompareFaces — match document face vs liveness reference.
	// 3. Updates user record; returns full result.
	router.POST("/api/documents", func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			http.Error(w, "parse form: "+err.Error(), http.StatusBadRequest)
			return
		}

		userID := strings.TrimSpace(r.FormValue("userId"))
		if userID == "" {
			http.Error(w, "userId is required", http.StatusBadRequest)
			return
		}

		storeMu.RLock()
		user, ok := users[userID]
		storeMu.RUnlock()
		if !ok {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		if user.VerificationStatus != "liveness_passed" {
			http.Error(w, "liveness check must be completed before document upload", http.StatusBadRequest)
			return
		}
		if user.ReferenceImage == "" {
			http.Error(w, "no liveness reference image on record", http.StatusBadRequest)
			return
		}

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

		// Step 1 — OCR the document using Rekognition DetectText.
		docData, err := analyzeDocument(ctx, rekClient, docBytes)
		if err != nil {
			http.Error(w, "detect text: "+err.Error(), http.StatusBadGateway)
			return
		}

		// Step 2 — Compare liveness face vs document face.
		// Decode reference image from stored data URL.
		refBytes, err := dataURLToBytes(user.ReferenceImage)
		if err != nil {
			http.Error(w, "decode reference image: "+err.Error(), http.StatusInternalServerError)
			return
		}

		faceMatch, err := compareFaces(ctx, rekClient, refBytes, docBytes)
		if err != nil {
			http.Error(w, "compare faces: "+err.Error(), http.StatusBadGateway)
			return
		}

		// Step 3 — Update user record.
		verificationStatus := "verified"
		if !faceMatch.Passed {
			verificationStatus = "failed"
		}

		storeMu.Lock()
		user.Document = docData
		user.FaceMatch = faceMatch
		user.VerificationStatus = verificationStatus
		user.UpdatedAt = time.Now()
		storeMu.Unlock()

		log.Printf("document processed  userId=%s similarity=%.1f passed=%v status=%s",
			userID, faceMatch.Similarity, faceMatch.Passed, verificationStatus)

		writeJSON(w, http.StatusOK, map[string]any{
			"userId":             userID,
			"verificationStatus": verificationStatus,
			"document":           docData,
			"faceMatch":          faceMatch,
			"user":               user,
		})
	})

	// ── GET /api/users ────────────────────────────────────────────────────────
	router.GET("/api/users", func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		storeMu.RLock()
		list := make([]*User, 0, len(users))
		for _, u := range users {
			list = append(list, u)
		}
		storeMu.RUnlock()
		writeJSON(w, http.StatusOK, list)
	})

	// ── GET /api/users/:userId ────────────────────────────────────────────────
	router.GET("/api/users/:userId", func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		storeMu.RLock()
		u, ok := users[ps.ByName("userId")]
		storeMu.RUnlock()
		if !ok {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, u)
	})

	log.Printf("server listening on http://localhost:%d", port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), withCORS(router)))
}

// ── AWS helpers ───────────────────────────────────────────────────────────────

// analyzeDocument uses Rekognition DetectText to read all text from the ID image,
// then extracts common fields via regex. No Textract subscription required.
func analyzeDocument(ctx context.Context, client *rekognition.Client, imgBytes []byte) (*DocumentData, error) {
	out, err := client.DetectText(ctx, &rekognition.DetectTextInput{
		Image: &rektypes.Image{Bytes: imgBytes},
	})
	if err != nil {
		return nil, err
	}

	// Collect LINE-level detections (already merged words → cleaner than WORD blocks).
	var lines []string
	for _, d := range out.TextDetections {
		if d.Type == rektypes.TextTypesLine {
			lines = append(lines, aws.ToString(d.DetectedText))
		}
	}

	return parseIDFields(lines), nil
}

var (
	// Dates: MM/DD/YYYY, DD/MM/YYYY, YYYY-MM-DD, DD MON YYYY, etc.
	reDate = regexp.MustCompile(`\b(\d{1,2}[/.\-]\d{1,2}[/.\-]\d{2,4}|\d{4}[/.\-]\d{2}[/.\-]\d{2}|\d{1,2}\s+(?:JAN|FEB|MAR|APR|MAY|JUN|JUL|AUG|SEP|OCT|NOV|DEC)\s+\d{4})\b`)
	// Document numbers: letter(s) followed by digits, or all-digit long numbers.
	reDocNum = regexp.MustCompile(`\b([A-Z]{1,2}\d{6,9}|[A-Z]\d{7}|\d{9})\b`)
	// MRZ line (passport / TD3): two lines of 44 chars each with < separators.
	reMRZ = regexp.MustCompile(`^[A-Z0-9<]{20,44}$`)
)

func parseIDFields(lines []string) *DocumentData {
	doc := &DocumentData{}

	var dates []string
	var mrzLines []string

	for _, line := range lines {
		upper := strings.ToUpper(strings.TrimSpace(line))

		// Collect dates.
		for _, m := range reDate.FindAllString(upper, -1) {
			dates = append(dates, m)
		}

		// Document number.
		if doc.IDNumber == "" {
			if m := reDocNum.FindString(upper); m != "" {
				doc.IDNumber = m
			}
		}

		// MRZ lines.
		if reMRZ.MatchString(upper) {
			mrzLines = append(mrzLines, upper)
		}
	}

	// Assign dates: first = DOB, last = expiry (most IDs list DOB before expiry).
	if len(dates) > 0 {
		doc.DOB = dates[0]
	}
	if len(dates) > 1 {
		doc.Expiry = dates[len(dates)-1]
	}

	// Parse MRZ if present (TD3 passport / TD1 ID card).
	if len(mrzLines) >= 2 {
		parseMRZ(mrzLines, doc)
	}

	return doc
}

// parseMRZ extracts name, DOB, expiry and document number from MRZ lines.
func parseMRZ(lines []string, doc *DocumentData) {
	// TD3 (passport): line 1 = 44 chars, positions 5–43 = name field.
	if len(lines[0]) >= 44 {
		namePart := lines[0][5:44]
		parts := strings.SplitN(namePart, "<<", 2)
		if len(parts) == 2 {
			doc.LastName = strings.ReplaceAll(parts[0], "<", " ")
			doc.FirstName = strings.ReplaceAll(parts[1], "<", " ")
		}
	}
	// TD3 line 2: positions 0–8 = doc number, 13–18 = DOB, 21–26 = expiry.
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
	return s[4:6] + "/" + s[2:4] + "/" + s[0:2] // DD/MM/YY
}

// compareFaces calls Rekognition CompareFaces.
// sourceBytes = liveness reference image, targetBytes = document photo.
const faceMatchThreshold = float32(80.0)

func compareFaces(ctx context.Context, client *rekognition.Client, sourceBytes, targetBytes []byte) (*FaceMatch, error) {
	out, err := client.CompareFaces(ctx, &rekognition.CompareFacesInput{
		SourceImage:         &rektypes.Image{Bytes: sourceBytes},
		TargetImage:         &rektypes.Image{Bytes: targetBytes},
		SimilarityThreshold: aws.Float32(faceMatchThreshold),
	})
	if err != nil {
		return nil, err
	}

	if len(out.FaceMatches) == 0 {
		return &FaceMatch{Similarity: 0, Passed: false}, nil
	}

	similarity := float64(aws.ToFloat32(out.FaceMatches[0].Similarity))
	return &FaceMatch{
		Similarity: similarity,
		Passed:     similarity >= float64(faceMatchThreshold),
	}, nil
}

// dataURLToBytes strips the "data:image/...;base64," prefix and decodes.
func dataURLToBytes(dataURL string) ([]byte, error) {
	idx := strings.Index(dataURL, ",")
	if idx == -1 {
		return nil, fmt.Errorf("invalid data URL")
	}
	return base64.StdEncoding.DecodeString(dataURL[idx+1:])
}

// ── HTTP helpers ──────────────────────────────────────────────────────────────

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}
