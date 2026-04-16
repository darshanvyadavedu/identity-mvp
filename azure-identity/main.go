package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"azure-identity/azure"
	"azure-identity/handlers"
	"azure-identity/store"

	"github.com/julienschmidt/httprouter"
)

func main() {
	faceEndpoint  := mustEnv("AZURE_FACE_ENDPOINT")
	faceKey       := mustEnv("AZURE_FACE_KEY")
	docEndpoint   := mustEnv("AZURE_DOCUMENT_ENDPOINT")
	docKey        := mustEnv("AZURE_DOCUMENT_KEY")
	faceListID    := getenv("AZURE_FACE_LIST_ID", "identity-verification")
	port          := getenv("PORT", "8081")

	face   := &azure.FaceClient{Endpoint: faceEndpoint, Key: faceKey}
	docInt := &azure.DocIntelClient{Endpoint: docEndpoint, Key: docKey}
	st     := store.New()

	// Ensure Azure FaceList exists (idempotent).
	if err := face.EnsureFaceList(context.Background(), faceListID); err != nil {
		log.Printf("face list %q: %v (may already exist)", faceListID, err)
	} else {
		log.Printf("face list %q ready", faceListID)
	}

	router := httprouter.New()

	router.GET("/", func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintln(w, "Azure Identity Verification\n\n"+
			"  POST /api/sessions                        — create liveness session\n"+
			"  GET  /api/sessions/:sessionId/result      — fetch liveness result\n"+
			"  POST /api/documents                       — upload ID (Doc Intelligence + Face verify)\n"+
			"  POST /api/sessions/:sessionId/consent     — store consent & verified data\n",
		)
	})

	router.POST("/api/sessions", handlers.CreateSession(face, st))
	router.GET("/api/sessions/:sessionId/result", handlers.GetLivenessResult(face, st))
	router.POST("/api/documents", handlers.UploadDocument(face, docInt, st))
	router.POST("/api/sessions/:sessionId/consent", handlers.StoreConsent(face, faceListID, st))

	log.Printf("azure-identity server listening on http://localhost:%s", port)
	log.Fatal(http.ListenAndServe(":"+port, withCORS(router)))
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-User-ID")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("required env var %s is not set", key)
	}
	return v
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
