package csvwriter

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/nehalshaquib/solana-balance-reporter/internal/logger"
	"github.com/nehalshaquib/solana-balance-reporter/internal/solana"
)

// CSVWriter handles writing token balances to CSV files
type CSVWriter struct {
	csvDir string
	logger *logger.Logger
}

// New creates a new CSVWriter
func New(csvDir string, logger *logger.Logger) (*CSVWriter, error) {
	// Ensure CSV directory exists
	if err := os.MkdirAll(csvDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create CSV directory: %w", err)
	}

	return &CSVWriter{
		csvDir: csvDir,
		logger: logger,
	}, nil
}

// WriteBalances writes token balances to a CSV file
func (w *CSVWriter) WriteBalances(balances []*solana.TokenBalance) (string, error) {
	if len(balances) == 0 {
		return "", fmt.Errorf("no balances to write")
	}

	// Create hourly filename based on current time
	now := time.Now().UTC()
	filename := fmt.Sprintf("balance_%s.csv", now.Format("2006-01-02_15"))
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

	// Write header
	if err := writer.Write([]string{"wallet_address", "timestamp", "balance"}); err != nil {
		return "", fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Write balance data
	for _, balance := range balances {
		row := []string{
			balance.WalletAddress,
			balance.Timestamp.Format(time.RFC3339),
			strconv.FormatFloat(balance.Balance, 'f', -1, 64),
		}

		if err := writer.Write(row); err != nil {
			return "", fmt.Errorf("failed to write CSV row: %w", err)
		}
	}

	w.logger.Log(fmt.Sprintf("Successfully wrote %d balances to %s", len(balances), filepath))
	return filepath, nil
}
