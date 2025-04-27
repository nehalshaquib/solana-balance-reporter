package csvwriter

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/nehalshaquib/solana-balance-reporter/internal/database"
	"github.com/nehalshaquib/solana-balance-reporter/internal/logger"
	"github.com/nehalshaquib/solana-balance-reporter/internal/solana"
)

// CSVWriter handles writing token balances to CSV files
type CSVWriter struct {
	csvDir string
	logger *logger.Logger
	db     *database.DB
}

// BalanceComparisonRecord represents a record with current and previous balances
type BalanceComparisonRecord struct {
	WalletAddress            string
	LastSolanaBalance        float64
	CurrentSolanaBalance     float64
	ChangeInSolanaBalance    float64
	IsChangeDetectedInSolana string // "true", "false", or "N/A"
	LastTokenBalance         float64
	CurrentTokenBalance      float64
	ChangeInTokenBalance     float64
	IsChangeDetectedInToken  string // "true", "false", or "N/A"
	HasLastSolanaBalance     bool
	HasCurrentSolanaBalance  bool
	HasLastTokenBalance      bool
	HasCurrentTokenBalance   bool
}

// ChangeStats holds statistics about balance changes
type ChangeStats struct {
	SolanaChanges     int
	TokenChanges      int
	LastRunTimestamp  time.Time
	CurrentTimestamp  time.Time
	TotalAddresses    int
	SuccessfulFetches int
	SolanaFetchFailed int
	TokenFetchFailed  int
	BothFetchesFailed int
}

// New creates a new CSVWriter
func New(csvDir string, logger *logger.Logger, db *database.DB) (*CSVWriter, error) {
	// Ensure CSV directory exists
	if err := os.MkdirAll(csvDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create CSV directory: %w", err)
	}

	return &CSVWriter{
		csvDir: csvDir,
		logger: logger,
		db:     db,
	}, nil
}

// WriteBalances writes token balances to a CSV file with an auto-generated filename
func (w *CSVWriter) WriteBalances(balances []*solana.TokenBalance) (string, error) {
	// Create filename based on current time with seconds precision
	now := time.Now().UTC()
	filename := fmt.Sprintf("balance_%s.csv", now.Format("2006-01-02_15_04_05"))

	return w.WriteBalancesWithFilename(balances, filename)
}

// readPreviousCSV reads the previous CSV file and returns a map of wallet addresses to balance data
func (w *CSVWriter) readPreviousCSV(csvPath string) (map[string]map[string]string, error) {
	// Check if file exists
	if _, err := os.Stat(csvPath); os.IsNotExist(err) {
		return nil, nil // File doesn't exist
	}

	// Open the CSV file
	file, err := os.Open(csvPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open previous CSV file: %w", err)
	}
	defer file.Close()

	// Create CSV reader
	reader := csv.NewReader(file)

	// Read header
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV header: %w", err)
	}

	// Find column indices
	addressIdx := -1
	for i, col := range header {
		if col == "address" {
			addressIdx = i
			break
		}
	}

	if addressIdx == -1 {
		return nil, fmt.Errorf("address column not found in CSV")
	}

	// Read all records
	records := make(map[string]map[string]string)
	for {
		record, err := reader.Read()
		if err != nil {
			break // End of file or error
		}

		// Use address as key
		address := record[addressIdx]
		data := make(map[string]string)

		// Map all other columns
		for i, value := range record {
			if i != addressIdx {
				data[header[i]] = value
			}
		}

		records[address] = data
	}

	return records, nil
}

// CompareBalances compares current balances with the previous run
func (w *CSVWriter) CompareBalances(currentBalances []*solana.TokenBalance) ([]*BalanceComparisonRecord, *ChangeStats, error) {
	// Get last run info from database
	lastRun, err := w.db.GetLastRun()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get last run info: %w", err)
	}

	// Initialize stats
	stats := &ChangeStats{
		CurrentTimestamp: time.Now().UTC(),
		TotalAddresses:   len(currentBalances),
	}

	// Count successful and failed fetches
	for _, balance := range currentBalances {
		if balance.SolanaError == nil && balance.TokenError == nil {
			stats.SuccessfulFetches++
		} else if balance.SolanaError != nil && balance.TokenError != nil {
			stats.BothFetchesFailed++
		} else if balance.SolanaError != nil {
			stats.SolanaFetchFailed++
		} else if balance.TokenError != nil {
			stats.TokenFetchFailed++
		}
	}

	// If no previous run, return records with only current balances
	if lastRun == nil {
		w.logger.Log("No previous run found, only current balances will be included")

		comparisonRecords := make([]*BalanceComparisonRecord, len(currentBalances))
		for i, balance := range currentBalances {
			comparisonRecords[i] = &BalanceComparisonRecord{
				WalletAddress:            balance.WalletAddress,
				CurrentSolanaBalance:     balance.SolanaBalance,
				CurrentTokenBalance:      balance.TokenBalance,
				HasCurrentSolanaBalance:  balance.SolanaError == nil,
				HasCurrentTokenBalance:   balance.TokenError == nil,
				IsChangeDetectedInSolana: "N/A",
				IsChangeDetectedInToken:  "N/A",
			}
		}

		return comparisonRecords, stats, nil
	}

	// Set last run timestamp in stats
	stats.LastRunTimestamp = lastRun.Timestamp

	// Read previous balance data from CSV file
	prevBalances, err := w.readPreviousCSV(lastRun.CsvPath)
	if err != nil {
		w.logger.LogError("Failed to read previous CSV file", err)
		// Return only current balances if we can't read previous
		comparisonRecords := make([]*BalanceComparisonRecord, len(currentBalances))
		for i, balance := range currentBalances {
			comparisonRecords[i] = &BalanceComparisonRecord{
				WalletAddress:            balance.WalletAddress,
				CurrentSolanaBalance:     balance.SolanaBalance,
				CurrentTokenBalance:      balance.TokenBalance,
				HasCurrentSolanaBalance:  balance.SolanaError == nil,
				HasCurrentTokenBalance:   balance.TokenError == nil,
				IsChangeDetectedInSolana: "N/A",
				IsChangeDetectedInToken:  "N/A",
			}
		}
		return comparisonRecords, stats, nil
	}

	// Create comparison records
	comparisonRecords := make([]*BalanceComparisonRecord, len(currentBalances))

	for i, currentBalance := range currentBalances {
		// Initialize record with current balances
		record := &BalanceComparisonRecord{
			WalletAddress:            currentBalance.WalletAddress,
			CurrentSolanaBalance:     currentBalance.SolanaBalance,
			CurrentTokenBalance:      currentBalance.TokenBalance,
			HasCurrentSolanaBalance:  currentBalance.SolanaError == nil,
			HasCurrentTokenBalance:   currentBalance.TokenError == nil,
			IsChangeDetectedInSolana: "N/A",
			IsChangeDetectedInToken:  "N/A",
		}

		// Find previous balance for this address
		prevData, found := prevBalances[currentBalance.WalletAddress]
		if found {
			// Parse previous Solana balance
			if solBalStr, exists := prevData["current_solana_balance"]; exists && solBalStr != "N/A" {
				if solBal, err := strconv.ParseFloat(solBalStr, 64); err == nil {
					record.LastSolanaBalance = solBal
					record.HasLastSolanaBalance = true
				}
			}

			// Parse previous token balance
			if tokenBalStr, exists := prevData["current_token_balance"]; exists && tokenBalStr != "N/A" {
				if tokenBal, err := strconv.ParseFloat(tokenBalStr, 64); err == nil {
					record.LastTokenBalance = tokenBal
					record.HasLastTokenBalance = true
				}
			}

			// Calculate changes and detect changes
			if record.HasLastSolanaBalance && record.HasCurrentSolanaBalance {
				record.ChangeInSolanaBalance = record.CurrentSolanaBalance - record.LastSolanaBalance
				if record.ChangeInSolanaBalance != 0 {
					record.IsChangeDetectedInSolana = "true"
					stats.SolanaChanges++
				} else {
					record.IsChangeDetectedInSolana = "false"
				}
			}

			if record.HasLastTokenBalance && record.HasCurrentTokenBalance {
				record.ChangeInTokenBalance = record.CurrentTokenBalance - record.LastTokenBalance
				if record.ChangeInTokenBalance != 0 {
					record.IsChangeDetectedInToken = "true"
					stats.TokenChanges++
				} else {
					record.IsChangeDetectedInToken = "false"
				}
			}
		}

		comparisonRecords[i] = record
	}

	return comparisonRecords, stats, nil
}

// WriteBalancesWithFilename writes token balances to a CSV file with the specified filename
func (w *CSVWriter) WriteBalancesWithFilename(balances []*solana.TokenBalance, filename string) (string, error) {
	if len(balances) == 0 {
		return "", fmt.Errorf("no balances to write")
	}

	// Get comparison records
	comparisonRecords, stats, err := w.CompareBalances(balances)
	if err != nil {
		w.logger.LogError("Failed to compare balances", err)
		// Continue with just current balances
	}

	filepath := filepath.Join(w.csvDir, filename)
	w.logger.Log(fmt.Sprintf("Writing %d balances to %s", len(balances), filepath))

	// Create the CSV file
	file, err := os.Create(filepath)
	if err != nil {
		return "", fmt.Errorf("failed to create CSV file: %w", err)
	}
	defer file.Close()

	// Create CSV writer
	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header with all the comparison columns
	header := []string{
		"address",
		"last_solana_balance",
		"current_solana_balance",
		"change_in_solana_balance",
		"is_change_detected_in_solana_balance",
		"last_token_balance",
		"current_token_balance",
		"change_in_token_balance",
		"is_change_detected_in_token_balance",
	}

	if err := writer.Write(header); err != nil {
		return "", fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Write comparison data
	for _, record := range comparisonRecords {
		// Format values for CSV
		lastSolBalance := "N/A"
		if record.HasLastSolanaBalance {
			lastSolBalance = strconv.FormatFloat(record.LastSolanaBalance, 'f', -1, 64)
		}

		currentSolBalance := "N/A"
		if record.HasCurrentSolanaBalance {
			currentSolBalance = strconv.FormatFloat(record.CurrentSolanaBalance, 'f', -1, 64)
		}

		changeSolBalance := "N/A"
		if record.HasLastSolanaBalance && record.HasCurrentSolanaBalance {
			changeSolBalance = strconv.FormatFloat(record.ChangeInSolanaBalance, 'f', -1, 64)
		}

		lastTokenBalance := "N/A"
		if record.HasLastTokenBalance {
			lastTokenBalance = strconv.FormatFloat(record.LastTokenBalance, 'f', -1, 64)
		}

		currentTokenBalance := "N/A"
		if record.HasCurrentTokenBalance {
			currentTokenBalance = strconv.FormatFloat(record.CurrentTokenBalance, 'f', -1, 64)
		}

		changeTokenBalance := "N/A"
		if record.HasLastTokenBalance && record.HasCurrentTokenBalance {
			changeTokenBalance = strconv.FormatFloat(record.ChangeInTokenBalance, 'f', -1, 64)
		}

		row := []string{
			record.WalletAddress,
			lastSolBalance,
			currentSolBalance,
			changeSolBalance,
			record.IsChangeDetectedInSolana,
			lastTokenBalance,
			currentTokenBalance,
			changeTokenBalance,
			record.IsChangeDetectedInToken,
		}

		if err := writer.Write(row); err != nil {
			return "", fmt.Errorf("failed to write CSV row: %w", err)
		}
	}

	// Update the last run info in database
	err = w.db.UpdateLastRun(time.Now().UTC(), filepath)
	if err != nil {
		w.logger.LogError("Failed to update last run info in database", err)
		// Continue anyway, just log the error
	}

	// Log the results
	w.logger.Log(fmt.Sprintf("Successfully wrote %d balances to %s", len(balances), filepath))
	w.logger.Log(fmt.Sprintf("Balance changes detected: SOL: %d, Token: %d", stats.SolanaChanges, stats.TokenChanges))

	if !stats.LastRunTimestamp.IsZero() {
		w.logger.Log(fmt.Sprintf("Compared with balances from: %s", stats.LastRunTimestamp.Format("2006-01-02 15:04:05 UTC")))
	} else {
		w.logger.Log("No previous balances to compare with")
	}

	return filepath, nil
}

// GetChangeStats returns statistics about the current run
func (w *CSVWriter) GetChangeStats(balances []*solana.TokenBalance) (*ChangeStats, error) {
	_, stats, err := w.CompareBalances(balances)
	if err != nil {
		return nil, err
	}

	return stats, nil
}
