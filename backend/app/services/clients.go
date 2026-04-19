package services

import (
	"context"
	"log"
	"sync"

	"user-authentication/config"
	awslib "user-authentication/lib/aws"
	azurelib "user-authentication/lib/azure"
	"user-authentication/lib/provider"
)

var (
	providerOnce sync.Once
	activeProvider provider.IdentityProvider
)

// Active returns the configured IdentityProvider, initialising it on first call.
// Provider resources (face list, collection) are created here via EnsureResources.
func Active() provider.IdentityProvider {
	providerOnce.Do(func() {
		cfg := config.Get()
		var err error
		switch cfg.Provider {
		case config.ProviderAzure:
			activeProvider = azurelib.New()
		default:
			activeProvider, err = awslib.New()
			if err != nil {
				log.Fatalf("aws provider init: %v", err)
			}
		}
		if initErr := activeProvider.EnsureResources(context.Background()); initErr != nil {
			log.Printf("provider EnsureResources: %v (continuing)", initErr)
		}
	})
	return activeProvider
}
