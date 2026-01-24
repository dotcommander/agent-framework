// Package client provides a clean wrapper around the Claude SDK client.
package client

import (
	"context"
	"sync/atomic"

	"github.com/dotcommander/agent-sdk-go/claude"
	"github.com/sony/gobreaker"
)

// ClientOption configures a Client.
type ClientOption func(*clientOptions)

type clientOptions struct {
	resilienceConfig *ResilienceConfig
}

// WithCircuitBreaker enables circuit breaker protection.
// Opens after consecutive failures, half-opens after timeout.
func WithCircuitBreaker(cfg *CircuitBreakerConfig) ClientOption {
	return func(o *clientOptions) {
		if o.resilienceConfig == nil {
			o.resilienceConfig = &ResilienceConfig{}
		}
		if cfg == nil {
			cfg = DefaultCircuitBreakerConfig()
		}
		o.resilienceConfig.CircuitBreaker = cfg
	}
}

// WithRetry enables retry with exponential backoff for transient failures.
func WithRetry(cfg *RetryConfig) ClientOption {
	return func(o *clientOptions) {
		if o.resilienceConfig == nil {
			o.resilienceConfig = &ResilienceConfig{}
		}
		if cfg == nil {
			cfg = DefaultRetryConfig()
		}
		o.resilienceConfig.Retry = cfg
	}
}

// WithRateLimiter enables adaptive rate limiting with 429 detection.
func WithRateLimiter(cfg *RateLimiterConfig) ClientOption {
	return func(o *clientOptions) {
		if o.resilienceConfig == nil {
			o.resilienceConfig = &ResilienceConfig{}
		}
		if cfg == nil {
			cfg = DefaultRateLimiterConfig()
		}
		o.resilienceConfig.RateLimiter = cfg
	}
}

// WithResilience enables all resilience features with default configs.
func WithResilience() ClientOption {
	return func(o *clientOptions) {
		o.resilienceConfig = &ResilienceConfig{
			CircuitBreaker: DefaultCircuitBreakerConfig(),
			Retry:          DefaultRetryConfig(),
			RateLimiter:    DefaultRateLimiterConfig(),
		}
	}
}

// Message represents a message from the AI.
type Message = claude.Message

// Querier is the minimal interface for querying an LLM.
type Querier interface {
	// Query sends a prompt and returns the complete response.
	Query(ctx context.Context, prompt string) (string, error)

	// Close releases resources associated with the client.
	Close() error
}

// StreamingQuerier extends Querier with streaming support.
type StreamingQuerier interface {
	Querier

	// QueryStream sends a prompt and streams the response.
	QueryStream(ctx context.Context, prompt string) (<-chan Message, <-chan error)
}

// Client provides backward compatibility as an alias for StreamingQuerier.
// Deprecated: Use Querier or StreamingQuerier directly for narrower contracts.
type Client = StreamingQuerier

// clientImpl implements the Client interface.
type clientImpl struct {
	claude     claude.Client
	connected  atomic.Bool
	resilience *ResilienceWrapper
}

// New creates a new Client with the given SDK options.
func New(ctx context.Context, sdkOpts []claude.ClientOption, clientOpts ...ClientOption) (Client, error) {
	// Apply client options
	opts := &clientOptions{}
	for _, opt := range clientOpts {
		opt(opts)
	}

	c, err := claude.NewClient(sdkOpts...)
	if err != nil {
		return nil, err
	}

	// Connect immediately
	if err := c.Connect(ctx); err != nil {
		return nil, err
	}

	client := &clientImpl{
		claude: c,
	}
	client.connected.Store(true)

	// Configure resilience if enabled
	if opts.resilienceConfig != nil {
		client.resilience = NewResilienceWrapper(opts.resilienceConfig)
	}

	return client, nil
}

// CircuitBreakerState returns the current circuit breaker state.
// Returns StateClosed if no circuit breaker is configured.
func (c *clientImpl) CircuitBreakerState() gobreaker.State {
	if c.resilience == nil {
		return gobreaker.StateClosed
	}
	return c.resilience.State()
}

// Query sends a prompt and returns the complete response.
func (c *clientImpl) Query(ctx context.Context, prompt string) (string, error) {
	if !c.connected.Load() {
		return "", ErrNotConnected
	}

	if c.claude == nil {
		return "", ErrNotConnected
	}

	// Use resilience wrapper if configured
	if c.resilience != nil {
		return c.resilience.Execute(ctx, func() (string, error) {
			return c.claude.Query(ctx, prompt)
		})
	}

	return c.claude.Query(ctx, prompt)
}

// QueryStream sends a prompt and streams the response.
func (c *clientImpl) QueryStream(ctx context.Context, prompt string) (<-chan Message, <-chan error) {
	if !c.connected.Load() || c.claude == nil {
		msgChan := make(chan Message)
		errChan := make(chan error, 1)
		errChan <- ErrNotConnected
		close(msgChan)
		close(errChan)
		return msgChan, errChan
	}
	return c.claude.QueryStream(ctx, prompt)
}

// Close releases resources associated with the client.
func (c *clientImpl) Close() error {
	if !c.connected.Load() {
		return nil
	}
	c.connected.Store(false)
	if c.claude == nil {
		return nil
	}
	return c.claude.Disconnect()
}
