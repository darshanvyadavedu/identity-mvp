package config

import "os"

// Provider identifies which biometric/document provider to use.
type Provider string

const (
	ProviderAWS   Provider = "aws"
	ProviderAzure Provider = "azure"
)

// Config holds all application configuration loaded from environment variables.
type Config struct {
	Provider Provider
	Port     string

	// Database
	DatabaseURL string
	DBHost      string
	DBPort      string
	DBUser      string
	DBPassword  string
	DBName      string
	DBSSLMode   string

	// Shared secrets
	HMACSecret    string
	EncryptionKey string

	// AWS
	AWSRegion               string
	RekognitionCollectionID string

	// Azure
	AzureFaceEndpoint string
	AzureFaceKey      string
	AzureDocEndpoint  string
	AzureDocKey       string
	AzureFaceListID   string
}

var cfg *Config

// Get returns the singleton Config, loading it on first call.
func Get() *Config {
	if cfg == nil {
		cfg = load()
	}
	return cfg
}

func load() *Config {
	return &Config{
		Provider: Provider(getenv("PROVIDER", "aws")),
		Port:     getenv("PORT", "8080"),

		DatabaseURL: os.Getenv("DATABASE_URL"),
		DBHost:      getenv("DB_HOST", "localhost"),
		DBPort:      getenv("DB_PORT", "5432"),
		DBUser:      getenv("DB_USER", "postgres"),
		DBPassword:  os.Getenv("DB_PASSWORD"),
		DBName:      getenv("DB_NAME", "identification"),
		DBSSLMode:   getenv("DB_SSLMODE", "disable"),

		HMACSecret:    os.Getenv("HMAC_SECRET"),
		EncryptionKey: os.Getenv("ENCRYPTION_KEY"),

		AWSRegion:               getenv("AWS_REGION", "us-east-1"),
		RekognitionCollectionID: getenv("REKOGNITION_COLLECTION_ID", "identity-verification"),

		AzureFaceEndpoint: os.Getenv("AZURE_FACE_ENDPOINT"),
		AzureFaceKey:      os.Getenv("AZURE_FACE_KEY"),
		AzureDocEndpoint:  os.Getenv("AZURE_DOCUMENT_ENDPOINT"),
		AzureDocKey:       os.Getenv("AZURE_DOCUMENT_KEY"),
		AzureFaceListID:   getenv("AZURE_FACE_LIST_ID", "identity-verification"),
	}
}

// CollectionID returns the provider-specific face collection/list identifier.
func (c *Config) CollectionID() string {
	if c.Provider == ProviderAzure {
		return c.AzureFaceListID
	}
	return c.RekognitionCollectionID
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
