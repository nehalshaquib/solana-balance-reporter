package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/nehalshaquib/solana-balance-reporter/internal/config"
	"github.com/nehalshaquib/solana-balance-reporter/internal/csvwriter"
	"github.com/nehalshaquib/solana-balance-reporter/internal/database"
	"github.com/nehalshaquib/solana-balance-reporter/internal/logger"
	"github.com/nehalshaquib/solana-balance-reporter/internal/mailer"
	"github.com/nehalshaquib/solana-balance-reporter/internal/reader"
	"github.com/nehalshaquib/solana-balance-reporter/internal/solana"
)

// Global variable to store current run timestamp
var currentRunTimestamp string
var timeFormatLock sync.Mutex

// iterationLock ensures only one iteration runs at a time
var iterationLock sync.Mutex
var isIterationRunning bool

func main() {
	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Printf("Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	log, err := logger.New(cfg.LogsDirPath)
	if err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer log.Close()

	log.Log("Solana Balance Reporter started")

	// Log configuration details (but mask sensitive info)
	log.Log(fmt.Sprintf("Configuration loaded - RPC URL: %s, Token Mint: %s, Email From: %s",
		maskString(cfg.SolanaRPCURL), cfg.TokenMintAddress, cfg.EmailFrom))
	log.Log(fmt.Sprintf("SMTP configured - Server: %s, Port: %d", cfg.SMTPServer, cfg.SMTPPort))
	log.Log(fmt.Sprintf("Performance settings - Timeout: %v, Max Retries: %d, Concurrency: %d",
		cfg.RPCTimeout, cfg.MaxRetries, cfg.ConcurrencyLimit))

	// Initialize database
	db, err := database.New(cfg.DatabasePath)
	if err != nil {
		log.LogError("Failed to initialize database", err)
		os.Exit(1)
	}
	defer db.Close()
	log.Log(fmt.Sprintf("Database initialized at %s", cfg.DatabasePath))

	// Initialize components
	addressReader := reader.New(cfg.AddressesFilePath, log)
	solanaClient := solana.New(cfg.SolanaRPCURL, cfg.TokenMintAddress, cfg.RPCTimeout, cfg.MaxRetries, log)
	csvWriter, err := csvwriter.New(cfg.CSVDirPath, log, db)
	if err != nil {
		log.LogError("Failed to initialize CSV writer", err)
		os.Exit(1)
	}
	mailClient := mailer.New(
		cfg.SMTPServer,
		cfg.SMTPPort,
		cfg.SMTPUsername,
		cfg.SMTPPassword,
		cfg.EmailFrom,
		cfg.EmailTo,
		cfg.MaxRetries,
		log,
	)

	// Setup ticker for periodic execution
	ticker := time.NewTicker(time.Duration(cfg.FetchIntervalMinutes) * time.Minute)
	defer ticker.Stop()

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Run once immediately (blocks until completed)
	runFetchAndReport(addressReader, solanaClient, csvWriter, mailClient, cfg, log)

	// Main loop
	for {
		select {
		case <-ticker.C:
			// Check if an iteration is already running
			if !isIterationRunning {
				// Launch in a goroutine to avoid blocking the select
				go func() {
					runFetchAndReport(addressReader, solanaClient, csvWriter, mailClient, cfg, log)
				}()
			} else {
				log.Log("Skipping scheduled run because previous iteration is still running")
			}
		case sig := <-sigChan:
			log.Log(fmt.Sprintf("Received signal %s, shutting down...", sig))
			return
		}
	}
}

// getRunTimestamp generates a consistent timestamp for the current run
func getRunTimestamp() string {
	timeFormatLock.Lock()
	defer timeFormatLock.Unlock()

	if currentRunTimestamp == "" {
		currentRunTimestamp = time.Now().UTC().Format("2006-01-02_15_04_05")
	}
	return currentRunTimestamp
}

// resetRunTimestamp clears the timestamp to prepare for the next run
func resetRunTimestamp() {
	timeFormatLock.Lock()
	defer timeFormatLock.Unlock()
	currentRunTimestamp = ""
}

// maskString masks sensitive data like API keys and tokens
func maskString(input string) string {
	if len(input) <= 10 {
		return "***"
	}

	// Keep the first 10 characters, mask the rest
	return input[:10] + "***"
}

// runFetchAndReport fetches balances and sends a report
func runFetchAndReport(
	addressReader *reader.AddressReader,
	solanaClient *solana.Client,
	csvWriter *csvwriter.CSVWriter,
	mailClient *mailer.Mailer,
	cfg *config.Config,
	log *logger.Logger,
) {
	// Lock the iteration to prevent overlap
	iterationLock.Lock()
	defer iterationLock.Unlock()

	// Mark that an iteration is running
	isIterationRunning = true
	defer func() {
		isIterationRunning = false
	}()

	// Reset the timestamp for a new run
	resetRunTimestamp()

	// Create a timestamp for this run - this ensures we use the same timestamp
	// for logs and CSV files throughout this iteration
	runTimestamp := getRunTimestamp()

	// Set up a dedicated log file for this iteration - just once at the beginning
	logFilename := fmt.Sprintf("activity_%s.log", runTimestamp)
	if err := log.SetFilename(logFilename); err != nil {
		fmt.Printf("Failed to set log filename: %v\n", err)
		return
	}

	log.Log("Starting balance fetch cycle")

	// Create a context with timeout for the entire operation
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.FetchIntervalMinutes)*time.Minute)
	defer cancel()

	// Read wallet addresses
	addresses, err := addressReader.ReadAddresses()
	if err != nil {
		log.LogError("Failed to read addresses", err)
		return
	}

	// Check if context is still valid
	if ctx.Err() != nil {
		log.LogError("Operation cancelled", ctx.Err())
		return
	}

	// Fetch token balances with context
	balances, errors := solanaClient.FetchTokenBalances(addresses, cfg.ConcurrencyLimit)

	// Log errors
	if len(errors) > 0 {
		log.Log(fmt.Sprintf("Encountered %d errors while fetching balances", len(errors)))
		for _, err := range errors {
			log.LogError("Fetch error", err)
		}
	}

	// If we have no balances, don't proceed
	if len(balances) == 0 {
		log.Log("No balances fetched, skipping report")
		return
	}

	// Check if context is still valid
	if ctx.Err() != nil {
		log.LogError("Operation cancelled", ctx.Err())
		return
	}

	// Write balances to CSV with the same timestamp as the log file
	csvFilename := fmt.Sprintf("balance_%s.csv", runTimestamp)
	csvPath, err := csvWriter.WriteBalancesWithFilename(balances, csvFilename)
	if err != nil {
		log.LogError("Failed to write balances to CSV", err)
		return
	}

	// Get change statistics for the email
	stats, err := csvWriter.GetChangeStats(balances)
	if err != nil {
		log.LogError("Failed to get change statistics", err)
		// Continue with nil stats if we failed
		stats = &csvwriter.ChangeStats{
			CurrentTimestamp: time.Now().UTC(),
			TotalAddresses:   len(balances),
		}
	}

	// Check if context is still valid
	if ctx.Err() != nil {
		log.LogError("Operation cancelled", ctx.Err())
		return
	}

	// Send email report with change statistics
	if err := mailClient.SendReport(csvPath, balances, stats); err != nil {
		log.LogError("Failed to send email report", err)
		return
	}

	log.Log("Balance fetch cycle completed successfully")
}
