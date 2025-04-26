package reader

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/nehalshaquib/solana-balance-reporter/internal/logger"
)

// AddressReader handles reading addresses from a file
type AddressReader struct {
	filePath string
	logger   *logger.Logger
}

// New creates a new AddressReader
func New(filePath string, logger *logger.Logger) *AddressReader {
	return &AddressReader{
		filePath: filePath,
		logger:   logger,
	}
}

// ReadAddresses reads all addresses from the configured file
func (r *AddressReader) ReadAddresses() ([]string, error) {
	r.logger.Log(fmt.Sprintf("Reading addresses from %s", r.filePath))

	file, err := os.Open(r.filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open addresses file: %w", err)
	}
	defer file.Close()

	var addresses []string
	scanner := bufio.NewScanner(file)
	lineNumber := 0

	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		addresses = append(addresses, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading addresses file: %w", err)
	}

	r.logger.Log(fmt.Sprintf("Successfully loaded %d addresses", len(addresses)))
	return addresses, nil
}
