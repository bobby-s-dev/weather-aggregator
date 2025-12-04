package client

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"

	"github.com/sony/gobreaker"
	"go.uber.org/zap"
)

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type BaseClient struct {
	client        HTTPClient
	logger        *zap.Logger
	circuitBreaker *gobreaker.CircuitBreaker
	maxRetries    int
	retryDelay    time.Duration
	multiplier    float64
}

type ClientConfig struct {
	Timeout       time.Duration
	MaxRetries    int
	RetryDelay    time.Duration
	Multiplier    float64
	Threshold     int
	BreakerTimeout time.Duration
}

func NewBaseClient(name string, config ClientConfig, logger *zap.Logger) *BaseClient {
	httpClient := &http.Client{
		Timeout: config.Timeout,
	}
	
	// Circuit breaker settings
	breakerSettings := gobreaker.Settings{
		Name:        name,
		MaxRequests: 1,
		Interval:    0,
		Timeout:     config.BreakerTimeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return counts.Requests >= 3 && failureRatio >= 0.6
		},
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			logger.Info("Circuit breaker state changed",
				zap.String("client", name),
				zap.String("from", from.String()),
				zap.String("to", to.String()))
		},
	}
	
	return &BaseClient{
		client:        httpClient,
		logger:        logger,
		circuitBreaker: gobreaker.NewCircuitBreaker(breakerSettings),
		maxRetries:    config.MaxRetries,
		retryDelay:    config.RetryDelay,
		multiplier:    config.Multiplier,
	}
}

func (c *BaseClient) GetWithRetry(ctx context.Context, url string) ([]byte, error) {
	var response []byte
	var err error
	
	// Execute with circuit breaker
	_, execErr := c.circuitBreaker.Execute(func() (interface{}, error) {
		response, err = c.doGetWithRetry(ctx, url)
		return response, err
	})
	
	if execErr != nil {
		return nil, execErr
	}
	
	return response, err
}

func (c *BaseClient) doGetWithRetry(ctx context.Context, url string) ([]byte, error) {
	var lastErr error
	
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			// Calculate exponential backoff delay
			delay := time.Duration(float64(c.retryDelay) * math.Pow(c.multiplier, float64(attempt-1)))
			c.logger.Debug("Retrying request",
				zap.String("url", url),
				zap.Int("attempt", attempt),
				zap.Duration("delay", delay))
			
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}
		
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, fmt.Errorf("creating request failed: %w", err)
		}
		
		resp, err := c.client.Do(req)
		if err != nil {
			lastErr = err
			c.logger.Warn("HTTP request failed",
				zap.String("url", url),
				zap.Int("attempt", attempt),
				zap.Error(err))
			continue
		}
		
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			
			if err != nil {
				lastErr = err
				continue
			}
			
			c.logger.Debug("Request successful",
				zap.String("url", url),
				zap.Int("status", resp.StatusCode),
				zap.Int("body_size", len(body)))
			
			return body, nil
		}
		
		resp.Body.Close()
		lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
		
		// Don't retry on client errors (4xx) except 429 (rate limiting)
		if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != 429 {
			break
		}
	}
	
	return nil, fmt.Errorf("max retries exceeded, last error: %w", lastErr)
}