package config

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all configuration for the application
type Config struct {
	SolanaRPCURL         string
	TokenMintAddress     string
	FetchIntervalMinutes int
	SMTPServer           string
	SMTPPort             int
	SMTPUsername         string
	SMTPPassword         string
	EmailFrom            string
	EmailTo              []string
	RPCTimeout           time.Duration
	MaxRetries           int
	ConcurrencyLimit     int
	AddressesFilePath    string
	CSVDirPath           string
	LogsDirPath          string
}

// LoadConfig loads configuration from environment variables
func LoadConfig() (*Config, error) {
	// Load .env file if it exists
	godotenv.Load()

	// Set default paths
	addressesPath := "addresses.txt"
	csvDirPath := "csv"
	logsDirPath := "logs"

	// Parse fetch interval with a default of 60 minutes
	fetchInterval := 60
	if val, exists := os.LookupEnv("FETCH_INTERVAL_MINUTES"); exists {
		if parsed, err := strconv.Atoi(val); err == nil && parsed > 0 {
			fetchInterval = parsed
		}
	}

	// Parse SMTP port with a default of 587
	smtpPort := 587
	if val, exists := os.LookupEnv("SMTP_PORT"); exists {
		if parsed, err := strconv.Atoi(val); err == nil && parsed > 0 {
			smtpPort = parsed
		}
	}

	// Parse timeout with a default of 10 seconds
	rpcTimeout := 10 * time.Second
	if val, exists := os.LookupEnv("RPC_TIMEOUT_SECONDS"); exists {
		if parsed, err := strconv.Atoi(val); err == nil && parsed > 0 {
			rpcTimeout = time.Duration(parsed) * time.Second
		}
	}

	// Parse max retries with a default of 3
	maxRetries := 3
	if val, exists := os.LookupEnv("MAX_RETRIES"); exists {
		if parsed, err := strconv.Atoi(val); err == nil && parsed > 0 {
			maxRetries = parsed
		}
	}

	// Parse concurrency limit with a default of 20
	concurrencyLimit := 20
	if val, exists := os.LookupEnv("CONCURRENCY_LIMIT"); exists {
		if parsed, err := strconv.Atoi(val); err == nil && parsed > 0 {
			concurrencyLimit = parsed
		}
	}

	// Parse email recipients
	emailTo := []string{}
	if val, exists := os.LookupEnv("EMAIL_TO"); exists && val != "" {
		emailTo = strings.Split(val, ",")
		// Trim spaces
		for i := range emailTo {
			emailTo[i] = strings.TrimSpace(emailTo[i])
		}
	}

	return &Config{
		SolanaRPCURL:         os.Getenv("SOLANA_RPC_URL"),
		TokenMintAddress:     os.Getenv("TOKEN_MINT_ADDRESS"),
		FetchIntervalMinutes: fetchInterval,
		SMTPServer:           os.Getenv("SMTP_SERVER"),
		SMTPPort:             smtpPort,
		SMTPUsername:         os.Getenv("SMTP_USERNAME"),
		SMTPPassword:         os.Getenv("SMTP_PASSWORD"),
		EmailFrom:            os.Getenv("EMAIL_FROM"),
		EmailTo:              emailTo,
		RPCTimeout:           rpcTimeout,
		MaxRetries:           maxRetries,
		ConcurrencyLimit:     concurrencyLimit,
		AddressesFilePath:    addressesPath,
		CSVDirPath:           csvDirPath,
		LogsDirPath:          logsDirPath,
	}, nil
}
