package config

import (
	"os"
	"strings"

	"github.com/charmbracelet/log"
)

type Config struct {
	ListenAddress string
	APIKey        string // for for internal service communication

	CredentialsKey string

	DatabaseURL     string
	NagomiGatewayURL  string
	NagomiReceiptsURL string
	ExchangeAPIURL  string

	S3Endpoint  string
	S3Bucket    string
	S3AccessKey string
	S3SecretKey string
	S3Region    string

	LogLevel  log.Level
	LogFormat string // "json" | "text"
}

// safely parse whatever port or address the user provides
// handdles cases like "8080", ":8080", "127.0.0.1:8080"
func parseAddress(port string) string {
	port = strings.TrimSpace(port)
	if strings.Contains(port, ":") {
		return port
	}
	return ":" + port
}

func Load() Config {
	apiKey := os.Getenv("API_KEY")
	if apiKey == "" {
		panic("API_KEY environment variable is required")
	}

	credentialsKey := os.Getenv("CREDENTIALS_KEY")
	if credentialsKey == "" {
		panic("CREDENTIALS_KEY environment variable is required (64 hex chars)")
	}

	nagomiGatewayURL := os.Getenv("NAGOMI_GATEWAY_URL")
	if nagomiGatewayURL == "" {
		panic("NAGOMI_GATEWAY_URL environment variable is required")
	}

	nagomiReceiptsURL := os.Getenv("NAGOMI_RECEIPTS_URL")
	if nagomiReceiptsURL == "" {
		// TODO: need to make this log print uniform with the app-wide logger
		// configuration. perhaps create the logger here, use it and then return it?
		log.Warn("NAGOMI_RECEIPTS_URL is not set!")
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		panic("DATABASE_URL environment variable is required")
	}

	exchangeAPIURL := os.Getenv("EXCHANGE_API_URL")
	if exchangeAPIURL == "" {
		panic("EXCHANGE_API_URL environment variable is required")
	}

	logLevel, err := log.ParseLevel(os.Getenv("LOG_LEVEL"))
	if err != nil {
		logLevel = log.InfoLevel
	}

	logFormat := strings.ToLower(strings.TrimSpace(os.Getenv("LOG_FORMAT")))
	if logFormat != "json" && logFormat != "text" {
		logFormat = "text"
	}

	listenAddr := os.Getenv("LISTEN_ADDRESS")
	if listenAddr == "" {
		listenAddr = "127.0.0.1:55555"
	}

	s3Endpoint := os.Getenv("S3_ENDPOINT")
	if s3Endpoint == "" {
		panic("S3_ENDPOINT environment variable is required")
	}

	s3Bucket := os.Getenv("S3_BUCKET")
	if s3Bucket == "" {
		panic("S3_BUCKET environment variable is required")
	}

	s3AccessKey := os.Getenv("S3_ACCESS_KEY")
	if s3AccessKey == "" {
		panic("S3_ACCESS_KEY environment variable is required")
	}

	s3SecretKey := os.Getenv("S3_SECRET_KEY")
	if s3SecretKey == "" {
		panic("S3_SECRET_KEY environment variable is required")
	}

	s3Region := os.Getenv("S3_REGION")
	if s3Region == "" {
		panic("S3_REGION environment variable is required")
	}

	return Config{
		ListenAddress:   parseAddress(listenAddr),
		APIKey:          apiKey,
		CredentialsKey:  credentialsKey,
		DatabaseURL:     databaseURL,
		NagomiGatewayURL:  nagomiGatewayURL,
		NagomiReceiptsURL: nagomiReceiptsURL,
		ExchangeAPIURL:  exchangeAPIURL,
		S3Endpoint:      s3Endpoint,
		S3Bucket:        s3Bucket,
		S3AccessKey:     s3AccessKey,
		S3SecretKey:     s3SecretKey,
		S3Region:        s3Region,
		LogLevel:        logLevel,
		LogFormat:       logFormat,
	}
}
