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
	"time"

	"github.com/nehalshaquib/solana-balance-reporter/internal/logger"
)

// TokenBalance represents a token balance entry
type TokenBalance struct {
	WalletAddress string
	Balance       float64
	Timestamp     time.Time
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

// FetchTokenBalance fetches the token balance for a wallet address
func (c *Client) FetchTokenBalance(ctx context.Context, walletAddress string) (*TokenBalance, error) {
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
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Retry logic with exponential backoff
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			// Calculate exponential backoff
			backoff := time.Duration(math.Pow(2, float64(attempt-1))) * c.retryDelay
			c.logger.Log(fmt.Sprintf("Retrying fetch for %s (attempt %d/%d) after %v",
				walletAddress, attempt, c.maxRetries, backoff))

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
				// Continue with retry
			}
		}

		// Create a new request
		req, err := http.NewRequestWithContext(ctx, "POST", c.rpcURL, bytes.NewBuffer(requestJSON))
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
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
				return nil, fmt.Errorf("failed to fetch token balance after %d attempts: %w", c.maxRetries+1, err)
			}
			return nil, fmt.Errorf("failed to fetch token balance after %d attempts: status code %d", c.maxRetries+1, resp.StatusCode)
		}
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
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
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Check for RPC error
	if response.Error != nil {
		return nil, fmt.Errorf("RPC error %d: %s", response.Error.Code, response.Error.Message)
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

	return &TokenBalance{
		WalletAddress: walletAddress,
		Balance:       balance,
		Timestamp:     time.Now().UTC(),
	}, nil
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

			balance, err := c.FetchTokenBalance(ctx, address)
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
			errors = append(errors, fmt.Errorf("error fetching balance for address index %d: %w",
				result.index, result.err))
			c.logger.LogError(fmt.Sprintf("Failed to fetch balance for address %s",
				addresses[result.index]), result.err)
		} else {
			balances = append(balances, result.balance)

			// Log every 50 successful fetches
			if len(balances)%50 == 0 {
				c.logger.Log(fmt.Sprintf("Fetched %d/%d balances", len(balances), len(addresses)))
			}
		}
	}

	c.logger.Log(fmt.Sprintf("Completed fetching balances. Success: %d, Errors: %d",
		len(balances), len(errors)))

	return balances, errors
}
