package services

// This file wires the provider-agnostic client interfaces to the concrete
// lib/aws or lib/azure implementations based on PROVIDER config.
// It lives in the services package to avoid a circular dependency:
//   lib/aws → app/clients (interface types) ≠ app/clients → lib/aws

import (
	"context"
	"log"

	"user-authentication/app/clients"
	"user-authentication/config"
	awslib "user-authentication/lib/aws"
	azurelib "user-authentication/lib/azure"
)

// newFaceClient returns the correct FaceClientInterface for the configured provider.
func newFaceClient() clients.FaceClientInterface {
	if config.Get().Provider == config.ProviderAzure {
		return azurelib.NewFaceClient()
	}
	return awslib.NewFaceClient()
}

// newDocumentClient returns the correct DocumentClientInterface for the configured provider.
func newDocumentClient() clients.DocumentClientInterface {
	if config.Get().Provider == config.ProviderAzure {
		return azurelib.NewDocumentClient()
	}
	return awslib.NewDocumentClient()
}

// InitProvider ensures the provider-specific face collection / face list exists.
// Call once at application startup before serving requests.
func InitProvider(ctx context.Context) {
	cfg := config.Get()
	switch cfg.Provider {
	case config.ProviderAzure:
		if err := azurelib.EnsureFaceList(ctx); err != nil {
			log.Printf("azure face list %q: %v (ignored if already exists)", cfg.AzureFaceListID, err)
		} else {
			log.Printf("azure face list %q ready", cfg.AzureFaceListID)
		}
	default:
		if err := awslib.EnsureCollection(ctx); err != nil {
			log.Printf("rekognition collection %q: %v (ignored if already exists)", cfg.RekognitionCollectionID, err)
		} else {
			log.Printf("rekognition collection %q ready", cfg.RekognitionCollectionID)
		}
	}
}
