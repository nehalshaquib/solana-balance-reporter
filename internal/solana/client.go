package solana

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/nehalshaquib/solana-balance-reporter/internal/logger"
)

const (
	// LAMPORTS_PER_SOL is the number of lamports in one SOL
	LAMPORTS_PER_SOL = 1000000000
)

// TokenBalance represents a token balance entry
type TokenBalance struct {
	WalletAddress string
	TokenBalance  float64
	SolanaBalance float64
	Timestamp     time.Time
	TokenError    error // Track if there was an error fetching token balance
	SolanaError   error // Track if there was an error fetching SOL balance
}

// Client represents a Solana RPC client
type Client struct {
	rpcURL     string
	tokenMint  string
	httpClient *http.Client
	logger     *logger.Logger
	maxRetries int
	retryDelay time.Duration
}

// New creates a new Solana RPC client
func New(rpcURL, tokenMint string, timeout time.Duration, maxRetries int, logger *logger.Logger) *Client {
	return &Client{
		rpcURL:     rpcURL,
		tokenMint:  tokenMint,
		httpClient: &http.Client{Timeout: timeout},
		logger:     logger,
		maxRetries: maxRetries,
		retryDelay: 500 * time.Millisecond,
	}
}

// isRetriableError checks if an error is retriable
func isRetriableError(err error) bool {
	if err == nil {
		return false
	}

	// Check for network/timeout errors that can be retried
	if strings.Contains(err.Error(), "timeout") ||
		strings.Contains(err.Error(), "connection") ||
		strings.Contains(err.Error(), "reset") ||
		strings.Contains(err.Error(), "EOF") ||
		strings.Contains(err.Error(), "broken pipe") {
		return true
	}

	// Rate limit errors are also retriable
	if strings.Contains(err.Error(), "rate") && strings.Contains(err.Error(), "limit") {
		return true
	}

	// Check HTTP status codes from RPC responses
	if strings.Contains(err.Error(), "429") || // Too Many Requests
		strings.Contains(err.Error(), "502") || // Bad Gateway
		strings.Contains(err.Error(), "503") || // Service Unavailable
		strings.Contains(err.Error(), "504") { // Gateway Timeout
		return true
	}

	return false
}

// FetchSolanaBalance fetches the native SOL balance for a wallet address
func (c *Client) FetchSolanaBalance(ctx context.Context, walletAddress string) (float64, error) {
	var resp *http.Response
	var err error

	// Prepare the JSON-RPC request for getBalance
	requestBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "getBalance",
		"params": []interface{}{
			walletAddress,
		},
	}

	requestJSON, err := json.Marshal(requestBody)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Retry logic with exponential backoff
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			// Calculate exponential backoff
			backoff := time.Duration(math.Pow(2, float64(attempt-1))) * c.retryDelay
			c.logger.Log(fmt.Sprintf("Retrying SOL balance fetch for %s (attempt %d/%d) after %v",
				walletAddress, attempt, c.maxRetries, backoff))

			select {
			case <-ctx.Done():
				return 0, ctx.Err()
			case <-time.After(backoff):
				// Continue with retry
			}
		}

		// Create a new request
		req, err := http.NewRequestWithContext(ctx, "POST", c.rpcURL, bytes.NewBuffer(requestJSON))
		if err != nil {
			return 0, fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		// Send the request
		resp, err = c.httpClient.Do(req)

		// Check for non-retriable errors
		if err != nil && !isRetriableError(err) {
			return 0, fmt.Errorf("non-retriable error fetching SOL balance: %w", err)
		}

		if err == nil && resp.StatusCode == http.StatusOK {
			break
		}

		if resp != nil {
			resp.Body.Close()
		}

		// If this was the last attempt, return the error
		if attempt == c.maxRetries {
			if err != nil {
				return 0, fmt.Errorf("failed to fetch SOL balance after %d attempts: %w", c.maxRetries+1, err)
			}
			return 0, fmt.Errorf("failed to fetch SOL balance after %d attempts: status code %d", c.maxRetries+1, resp.StatusCode)
		}
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to read response: %w", err)
	}

	// Parse the response
	var response struct {
		Result struct {
			Value int64 `json:"value"` // lamports
		} `json:"result"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return 0, fmt.Errorf("failed to parse response: %w", err)
	}

	// Check for RPC error
	if response.Error != nil {
		return 0, fmt.Errorf("RPC error %d: %s", response.Error.Code, response.Error.Message)
	}

	// Convert lamports to SOL
	solBalance := float64(response.Result.Value) / LAMPORTS_PER_SOL

	return solBalance, nil
}

// FetchTokenBalance fetches the token balance for a wallet address
func (c *Client) FetchTokenBalance(ctx context.Context, walletAddress string) (float64, error) {
	var resp *http.Response
	var err error

	// Prepare the JSON-RPC request
	requestBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "getTokenAccountsByOwner",
		"params": []interface{}{
			walletAddress,
			map[string]string{
				"mint": c.tokenMint,
			},
			map[string]string{
				"encoding": "jsonParsed",
			},
		},
	}

	requestJSON, err := json.Marshal(requestBody)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Retry logic with exponential backoff
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			// Calculate exponential backoff
			backoff := time.Duration(math.Pow(2, float64(attempt-1))) * c.retryDelay
			c.logger.Log(fmt.Sprintf("Retrying token balance fetch for %s (attempt %d/%d) after %v",
				walletAddress, attempt, c.maxRetries, backoff))

			select {
			case <-ctx.Done():
				return 0, ctx.Err()
			case <-time.After(backoff):
				// Continue with retry
			}
		}

		// Create a new request
		req, err := http.NewRequestWithContext(ctx, "POST", c.rpcURL, bytes.NewBuffer(requestJSON))
		if err != nil {
			return 0, fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		// Send the request
		resp, err = c.httpClient.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			break
		}

		if resp != nil {
			resp.Body.Close()
		}

		// If this was the last attempt, return the error
		if attempt == c.maxRetries {
			if err != nil {
				return 0, fmt.Errorf("failed to fetch token balance after %d attempts: %w", c.maxRetries+1, err)
			}
			return 0, fmt.Errorf("failed to fetch token balance after %d attempts: status code %d", c.maxRetries+1, resp.StatusCode)
		}
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to read response: %w", err)
	}

	// Parse the response
	var response struct {
		Result struct {
			Value []struct {
				Account struct {
					Data struct {
						Parsed struct {
							Info struct {
								TokenAmount struct {
									Amount   string  `json:"amount"`
									Decimals int     `json:"decimals"`
									UIAmount float64 `json:"uiAmount"`
								} `json:"tokenAmount"`
							} `json:"info"`
						} `json:"parsed"`
					} `json:"data"`
				} `json:"account"`
			} `json:"value"`
		} `json:"result"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return 0, fmt.Errorf("failed to parse response: %w", err)
	}

	// Check for RPC error
	if response.Error != nil {
		return 0, fmt.Errorf("RPC error %d: %s", response.Error.Code, response.Error.Message)
	}

	// Extract balance
	balance := 0.0
	if len(response.Result.Value) > 0 {
		// Get UI amount directly if available
		balance = response.Result.Value[0].Account.Data.Parsed.Info.TokenAmount.UIAmount

		// If UIAmount is 0, try to calculate from raw amount and decimals
		if balance == 0 {
			amountStr := response.Result.Value[0].Account.Data.Parsed.Info.TokenAmount.Amount
			decimals := response.Result.Value[0].Account.Data.Parsed.Info.TokenAmount.Decimals

			amount, ok := new(big.Int).SetString(amountStr, 10)
			if ok {
				divisor := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil))
				amountFloat := new(big.Float).SetInt(amount)
				amountFloat.Quo(amountFloat, divisor)

				balance, _ = amountFloat.Float64()
			}
		}
	}
	// When no accounts found, balance stays 0

	return balance, nil
}

// FetchBalances fetches both SOL and token balances for a wallet address
func (c *Client) FetchBalances(ctx context.Context, walletAddress string) (*TokenBalance, error) {
	// Create channels for the results
	solChan := make(chan struct {
		balance float64
		err     error
	})
	tokenChan := make(chan struct {
		balance float64
		err     error
	})

	// Fetch SOL balance
	go func() {
		balance, err := c.FetchSolanaBalance(ctx, walletAddress)
		solChan <- struct {
			balance float64
			err     error
		}{balance, err}
	}()

	// Fetch token balance
	go func() {
		balance, err := c.FetchTokenBalance(ctx, walletAddress)
		tokenChan <- struct {
			balance float64
			err     error
		}{balance, err}
	}()

	// Wait for both results
	solResult := <-solChan
	tokenResult := <-tokenChan

	// Create the result
	result := &TokenBalance{
		WalletAddress: walletAddress,
		TokenBalance:  tokenResult.balance,
		SolanaBalance: solResult.balance,
		Timestamp:     time.Now().UTC(),
		TokenError:    tokenResult.err,
		SolanaError:   solResult.err,
	}

	// If both failed, return an error
	if solResult.err != nil && tokenResult.err != nil {
		return result, fmt.Errorf("failed to fetch balances: SOL error: %v, token error: %v", solResult.err, tokenResult.err)
	}

	return result, nil
}

// FetchTokenBalances fetches token balances for multiple wallet addresses concurrently
func (c *Client) FetchTokenBalances(addresses []string, concurrencyLimit int) ([]*TokenBalance, []error) {
	balances := make([]*TokenBalance, 0, len(addresses))
	errors := make([]error, 0)

	// Create a semaphore channel to limit concurrency
	sem := make(chan struct{}, concurrencyLimit)
	resultCh := make(chan struct {
		balance *TokenBalance
		err     error
		index   int
	}, len(addresses))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c.logger.Log(fmt.Sprintf("Starting to fetch balances for %d addresses with concurrency limit %d",
		len(addresses), concurrencyLimit))

	// Start fetching balances
	for i, address := range addresses {
		sem <- struct{}{} // Acquire semaphore

		go func(i int, address string) {
			defer func() { <-sem }() // Release semaphore

			balance, err := c.FetchBalances(ctx, address)
			resultCh <- struct {
				balance *TokenBalance
				err     error
				index   int
			}{balance, err, i}
		}(i, address)
	}

	// Collect results
	for i := 0; i < len(addresses); i++ {
		result := <-resultCh

		if result.err != nil {
			address := addresses[result.index]
			errors = append(errors, fmt.Errorf("error fetching balance for address %s: %w",
				address, result.err))
			c.logger.LogError(fmt.Sprintf("Failed to fetch balance for address %s",
				address), result.err)
		}

		// Always add the balance record, even if there was an error
		// The balance will have error fields set if fetching failed
		balances = append(balances, result.balance)

		// Log every 50 fetches
		if (i+1)%50 == 0 {
			c.logger.Log(fmt.Sprintf("Fetched %d/%d balances", i+1, len(addresses)))
		}
	}

	// Count successful and failed fetches for SOL and token
	successSol := 0
	failedSol := 0
	successToken := 0
	failedToken := 0

	for _, balance := range balances {
		if balance.SolanaError == nil {
			successSol++
		} else {
			failedSol++
		}

		if balance.TokenError == nil {
			successToken++
		} else {
			failedToken++
		}
	}

	c.logger.Log(fmt.Sprintf("Completed fetching balances. SOL: (Success: %d, Errors: %d), Token: (Success: %d, Errors: %d)",
		successSol, failedSol, successToken, failedToken))

	return balances, errors
}
