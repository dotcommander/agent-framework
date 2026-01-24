// Package client provides resilience wrappers for API calls.
package client

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/sony/gobreaker"
)

// Resilience errors.
var (
	// ErrCircuitOpen indicates the circuit breaker is open.
	ErrCircuitOpen = errors.New("circuit breaker is open")

	// ErrRateLimited indicates rate limiting is active.
	ErrRateLimited = errors.New("rate limited")

	// ErrMaxRetriesExceeded indicates all retry attempts failed.
	ErrMaxRetriesExceeded = errors.New("max retries exceeded")
)

// ResilienceConfig configures resilience behavior.
type ResilienceConfig struct {
	// Circuit breaker settings
	CircuitBreaker *CircuitBreakerConfig

	// Retry settings
	Retry *RetryConfig

	// Rate limiter settings
	RateLimiter *RateLimiterConfig
}

// CircuitBreakerConfig configures the circuit breaker.
type CircuitBreakerConfig struct {
	// Name identifies the circuit breaker.
	Name string

	// MaxRequests is the maximum number of requests allowed in half-open state.
	MaxRequests uint32

	// Interval is the cyclic period for clearing counts in closed state.
	Interval time.Duration

	// Timeout is the duration in open state before transitioning to half-open.
	Timeout time.Duration

	// ConsecutiveFailures is the number of consecutive failures to trip the breaker.
	ConsecutiveFailures uint32

	// OnStateChange is called when the circuit breaker state changes.
	OnStateChange func(name string, from, to gobreaker.State)
}

// DefaultCircuitBreakerConfig returns sensible defaults.
func DefaultCircuitBreakerConfig() *CircuitBreakerConfig {
	return &CircuitBreakerConfig{
		Name:                "claude-api",
		MaxRequests:         1,
		Interval:            60 * time.Second,
		Timeout:             30 * time.Second,
		ConsecutiveFailures: 5,
	}
}

// RetryConfig configures retry behavior.
type RetryConfig struct {
	// MaxRetries is the maximum number of retry attempts.
	MaxRetries uint64

	// InitialInterval is the initial backoff interval.
	InitialInterval time.Duration

	// MaxInterval is the maximum backoff interval.
	MaxInterval time.Duration

	// Multiplier is the factor by which the interval increases.
	Multiplier float64

	// RandomizationFactor adds jitter to prevent thundering herd.
	RandomizationFactor float64
}

// DefaultRetryConfig returns sensible defaults.
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxRetries:          3,
		InitialInterval:     500 * time.Millisecond,
		MaxInterval:         30 * time.Second,
		Multiplier:          2.0,
		RandomizationFactor: 0.5,
	}
}

// RateLimiterConfig configures rate limiting with 429 detection.
type RateLimiterConfig struct {
	// InitialBackoff is the initial backoff after a 429.
	InitialBackoff time.Duration

	// MaxBackoff is the maximum backoff duration.
	MaxBackoff time.Duration

	// BackoffMultiplier increases backoff on consecutive 429s.
	BackoffMultiplier float64
}

// DefaultRateLimiterConfig returns sensible defaults.
func DefaultRateLimiterConfig() *RateLimiterConfig {
	return &RateLimiterConfig{
		InitialBackoff:    1 * time.Second,
		MaxBackoff:        60 * time.Second,
		BackoffMultiplier: 2.0,
	}
}

// RateLimitError represents a 429 response.
type RateLimitError struct {
	RetryAfter time.Duration
	Message    string
}

func (e *RateLimitError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("rate limited: retry after %v", e.RetryAfter)
	}
	return fmt.Sprintf("rate limited: %s", e.Message)
}

// ServerError represents a 5xx response.
type ServerError struct {
	StatusCode int
	Message    string
}

func (e *ServerError) Error() string {
	return fmt.Sprintf("server error %d: %s", e.StatusCode, e.Message)
}

// ResilienceWrapper wraps API calls with resilience patterns.
type ResilienceWrapper struct {
	cb          *gobreaker.CircuitBreaker
	retryConfig *RetryConfig
	rateLimiter *adaptiveRateLimiter
}

// adaptiveRateLimiter handles 429 responses with adaptive backoff.
type adaptiveRateLimiter struct {
	config         *RateLimiterConfig
	currentBackoff time.Duration
	lastRateLimit  time.Time
	mu             sync.Mutex
}

func newAdaptiveRateLimiter(config *RateLimiterConfig) *adaptiveRateLimiter {
	return &adaptiveRateLimiter{
		config:         config,
		currentBackoff: config.InitialBackoff,
	}
}

// shouldWait returns how long to wait before the next request.
func (r *adaptiveRateLimiter) shouldWait() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.lastRateLimit.IsZero() {
		return 0
	}

	elapsed := time.Since(r.lastRateLimit)
	if elapsed >= r.currentBackoff {
		return 0
	}

	return r.currentBackoff - elapsed
}

// recordRateLimit records a 429 response.
func (r *adaptiveRateLimiter) recordRateLimit(retryAfter time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	isFirstLimit := r.lastRateLimit.IsZero()
	r.lastRateLimit = time.Now()

	// Use server-provided retry-after if available
	if retryAfter > 0 {
		r.currentBackoff = retryAfter
		return
	}

	// First rate limit uses initial backoff, subsequent ones escalate
	if isFirstLimit {
		// Already at initial backoff, no change needed
		return
	}

	// Increase backoff exponentially for subsequent rate limits
	r.currentBackoff = time.Duration(float64(r.currentBackoff) * r.config.BackoffMultiplier)
	if r.currentBackoff > r.config.MaxBackoff {
		r.currentBackoff = r.config.MaxBackoff
	}
}

// recordSuccess resets backoff on successful request.
func (r *adaptiveRateLimiter) recordSuccess() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.currentBackoff = r.config.InitialBackoff
	r.lastRateLimit = time.Time{}
}

// NewResilienceWrapper creates a new resilience wrapper.
func NewResilienceWrapper(config *ResilienceConfig) *ResilienceWrapper {
	if config == nil {
		config = &ResilienceConfig{}
	}

	w := &ResilienceWrapper{}

	// Configure circuit breaker
	if config.CircuitBreaker != nil {
		cbConfig := config.CircuitBreaker
		settings := gobreaker.Settings{
			Name:        cbConfig.Name,
			MaxRequests: cbConfig.MaxRequests,
			Interval:    cbConfig.Interval,
			Timeout:     cbConfig.Timeout,
			ReadyToTrip: func(counts gobreaker.Counts) bool {
				return counts.ConsecutiveFailures >= cbConfig.ConsecutiveFailures
			},
			OnStateChange: cbConfig.OnStateChange,
		}
		w.cb = gobreaker.NewCircuitBreaker(settings)
	}

	// Configure retry
	if config.Retry != nil {
		w.retryConfig = config.Retry
	}

	// Configure rate limiter
	if config.RateLimiter != nil {
		w.rateLimiter = newAdaptiveRateLimiter(config.RateLimiter)
	}

	return w
}

// Execute runs the operation with all configured resilience patterns.
func (w *ResilienceWrapper) Execute(ctx context.Context, operation func() (string, error)) (string, error) {
	// Check rate limiter first
	if w.rateLimiter != nil {
		if wait := w.rateLimiter.shouldWait(); wait > 0 {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(wait):
			}
		}
	}

	// Wrap with circuit breaker if configured
	if w.cb != nil {
		return w.executeWithCircuitBreaker(ctx, operation)
	}

	// Just retry if no circuit breaker
	return w.executeWithRetry(ctx, operation)
}

// executeWithCircuitBreaker runs the operation through the circuit breaker.
func (w *ResilienceWrapper) executeWithCircuitBreaker(ctx context.Context, operation func() (string, error)) (string, error) {
	result, err := w.cb.Execute(func() (any, error) {
		return w.executeWithRetry(ctx, operation)
	})

	if err != nil {
		if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
			return "", fmt.Errorf("%w: %v", ErrCircuitOpen, err)
		}
		return "", err
	}

	return result.(string), nil
}

// executeWithRetry runs the operation with retry logic.
func (w *ResilienceWrapper) executeWithRetry(ctx context.Context, operation func() (string, error)) (string, error) {
	if w.retryConfig == nil {
		return w.executeSingle(ctx, operation)
	}

	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.InitialInterval = w.retryConfig.InitialInterval
	expBackoff.MaxInterval = w.retryConfig.MaxInterval
	expBackoff.Multiplier = w.retryConfig.Multiplier
	expBackoff.RandomizationFactor = w.retryConfig.RandomizationFactor

	b := backoff.WithMaxRetries(expBackoff, w.retryConfig.MaxRetries)
	b = backoff.WithContext(b, ctx)

	var result string
	var lastErr error

	retryOp := func() error {
		var err error
		result, err = w.executeSingle(ctx, operation)
		if err != nil {
			lastErr = err
			if w.isRetryable(err) {
				return err // Retry
			}
			return backoff.Permanent(err) // Don't retry
		}
		return nil
	}

	if err := backoff.Retry(retryOp, b); err != nil {
		if lastErr != nil {
			return "", lastErr
		}
		return "", err
	}

	return result, nil
}

// executeSingle executes the operation once, handling rate limits.
func (w *ResilienceWrapper) executeSingle(_ context.Context, operation func() (string, error)) (string, error) {
	result, err := operation()

	if err != nil {
		// Handle rate limit errors
		var rateErr *RateLimitError
		if errors.As(err, &rateErr) && w.rateLimiter != nil {
			w.rateLimiter.recordRateLimit(rateErr.RetryAfter)
			return "", err
		}
		return "", err
	}

	// Record success
	if w.rateLimiter != nil {
		w.rateLimiter.recordSuccess()
	}

	return result, nil
}

// isRetryable determines if an error should trigger a retry.
func (w *ResilienceWrapper) isRetryable(err error) bool {
	if err == nil {
		return false
	}

	// Context errors are not retryable (check first - DeadlineExceeded implements net.Error)
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	// Rate limit errors are retryable
	var rateErr *RateLimitError
	if errors.As(err, &rateErr) {
		return true
	}

	// Server errors (5xx) are retryable
	var serverErr *ServerError
	if errors.As(err, &serverErr) {
		return serverErr.StatusCode >= 500 && serverErr.StatusCode < 600
	}

	// Network errors are retryable
	if isNetworkError(err) {
		return true
	}

	return false
}

// isNetworkError checks if the error is a transient network error.
func isNetworkError(err error) bool {
	if err == nil {
		return false
	}

	// Check for net.Error timeout
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	// Check for common network error strings
	errStr := err.Error()
	networkIndicators := []string{
		"connection refused",
		"connection reset",
		"no such host",
		"network is unreachable",
		"i/o timeout",
		"EOF",
		"broken pipe",
	}

	for _, indicator := range networkIndicators {
		if strings.Contains(strings.ToLower(errStr), strings.ToLower(indicator)) {
			return true
		}
	}

	return false
}

// State returns the current circuit breaker state.
func (w *ResilienceWrapper) State() gobreaker.State {
	if w.cb == nil {
		return gobreaker.StateClosed
	}
	return w.cb.State()
}

// Counts returns the current circuit breaker counts.
func (w *ResilienceWrapper) Counts() gobreaker.Counts {
	if w.cb == nil {
		return gobreaker.Counts{}
	}
	return w.cb.Counts()
}
