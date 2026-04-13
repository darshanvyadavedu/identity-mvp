package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/rekognition"
	"github.com/aws/aws-sdk-go-v2/service/textract"
	"github.com/joho/godotenv"
	"github.com/julienschmidt/httprouter"

	"user-authentication/db"
	"user-authentication/handlers"
)

func main() {
	// Connect to PostgreSQL (database: identification).
	_ = godotenv.Load("/Users/ayushwgadre/personal-projects/user-authentication/.env")
	db.Connect()
	//_ = godotenv.Load(".env") // fallback if run from backend/
	// AWS SDK.
	region := getenv("AWS_REGION", "us-east-1")
	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(), awsconfig.WithRegion(region))
	if err != nil {
		log.Fatalf("aws config: %v", err)
	}

	rekClient := rekognition.NewFromConfig(awsCfg)
	txtClient := textract.NewFromConfig(awsCfg)

	// Ensure the Rekognition face collection exists (idempotent).
	collectionID := getenv("REKOGNITION_COLLECTION_ID", "identity-verification")
	_, err = rekClient.CreateCollection(context.Background(), &rekognition.CreateCollectionInput{
		CollectionId: aws.String(collectionID),
	})
	if err != nil {
		log.Printf("face collection %q: %v (ignored if already exists)", collectionID, err)
	} else {
		log.Printf("face collection %q created", collectionID)
	}

	// Router.
	router := httprouter.New()

	router.GET("/", func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintln(w, "Endpoints:\n"+
			"  POST /api/sessions                          — create liveness session\n"+
			"  GET  /api/sessions/:sessionId/result        — fetch liveness result\n"+
			"  POST /api/documents                         — upload ID document (Textract AnalyzeID)\n"+
			"  POST /api/sessions/:sessionId/consent       — store consent & verified data\n",
		)
	})

	router.POST("/api/sessions", handlers.CreateSession(rekClient))
	router.GET("/api/sessions/:sessionId/result", handlers.GetLivenessResult(rekClient))
	router.POST("/api/documents", handlers.UploadDocument(rekClient, txtClient))
	router.POST("/api/sessions/:sessionId/consent", handlers.StoreConsent(rekClient))

	port := getenv("PORT", "8080")
	log.Printf("server listening on http://localhost:%s", port)
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

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
