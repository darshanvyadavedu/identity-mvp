package services

import (
	"context"
	"log"
	"sync"

	"user-authentication/config"
	awslib "user-authentication/lib/aws"
	azurelib "user-authentication/lib/azure"
	easyocrlib "user-authentication/lib/easyocr"
	"user-authentication/lib/provider"
)

var (
	faceOnce   sync.Once
	activeFace provider.FaceProvider

	docOnce   sync.Once
	activeDoc provider.DocumentProvider
)

// ActiveFace returns the configured FaceProvider (liveness + face ops),
// initialising it on first call. Controlled by the PROVIDER env var.
func ActiveFace() provider.FaceProvider {
	faceOnce.Do(func() {
		cfg := config.Get()
		var err error
		switch cfg.Provider {
		case config.ProviderAzure:
			activeFace = azurelib.New()
		default:
			activeFace, err = awslib.New()
			if err != nil {
				log.Fatalf("aws face provider init: %v", err)
			}
		}
		if initErr := activeFace.EnsureResources(context.Background()); initErr != nil {
			log.Printf("face provider EnsureResources: %v (continuing)", initErr)
		}
	})
	return activeFace
}

// ActiveDoc returns the configured DocumentProvider (OCR),
// initialising it on first call. Controlled by the DOC_PROVIDER env var
// (defaults to the same value as PROVIDER).
func ActiveDoc() provider.DocumentProvider {
	docOnce.Do(func() {
		cfg := config.Get()
		var err error
		switch cfg.DocProvider {
		case config.ProviderEasyOCR:
			activeDoc = easyocrlib.New()
		case config.ProviderAzure:
			activeDoc = azurelib.New()
		default:
			activeDoc, err = awslib.New()
			if err != nil {
				log.Fatalf("aws doc provider init: %v", err)
			}
		}
		log.Printf("doc provider: %s", cfg.DocProvider)
	})
	return activeDoc
}
