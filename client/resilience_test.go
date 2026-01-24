package client

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sony/gobreaker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCircuitBreakerStateTransitions tests state transitions: Closed → Open → HalfOpen → Closed
func TestCircuitBreakerStateTransitions(t *testing.T) {
	t.Parallel()

	var stateChanges []string
	var mu sync.Mutex

	config := &ResilienceConfig{
		CircuitBreaker: &CircuitBreakerConfig{
			Name:                "test-cb",
			MaxRequests:         1,
			Interval:            100 * time.Millisecond,
			Timeout:             200 * time.Millisecond, // Short timeout for test
			ConsecutiveFailures: 3,
			OnStateChange: func(name string, from, to gobreaker.State) {
				mu.Lock()
				defer mu.Unlock()
				stateChanges = append(stateChanges, fmt.Sprintf("%s->%s", from.String(), to.String()))
			},
		},
	}

	wrapper := NewResilienceWrapper(config)
	ctx := context.Background()

	// Initially closed
	assert.Equal(t, gobreaker.StateClosed, wrapper.State())

	// Trigger 3 consecutive failures to open the circuit
	for range 3 {
		_, err := wrapper.Execute(ctx, func() (string, error) {
			return "", errors.New("failure")
		})
		assert.Error(t, err)
	}

	// Should be open now
	assert.Equal(t, gobreaker.StateOpen, wrapper.State())

	// Request during open state should fail immediately
	_, err := wrapper.Execute(ctx, func() (string, error) {
		return "success", nil
	})
	assert.ErrorIs(t, err, ErrCircuitOpen)

	// Wait for timeout to transition to half-open
	time.Sleep(250 * time.Millisecond)

	// Next successful request should transition to closed
	result, err := wrapper.Execute(ctx, func() (string, error) {
		return "recovered", nil
	})
	assert.NoError(t, err)
	assert.Equal(t, "recovered", result)

	// Should be closed now
	assert.Equal(t, gobreaker.StateClosed, wrapper.State())

	// Verify state transitions occurred
	mu.Lock()
	defer mu.Unlock()
	assert.Contains(t, stateChanges, "closed->open")
	assert.Contains(t, stateChanges, "open->half-open")
	assert.Contains(t, stateChanges, "half-open->closed")
}

// TestCircuitBreakerFailureThreshold tests that the circuit opens after reaching failure threshold
func TestCircuitBreakerFailureThreshold(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		consecutiveFailures uint32
		failureCount        int
		expectOpen          bool
	}{
		{
			name:                "below threshold",
			consecutiveFailures: 5,
			failureCount:        4,
			expectOpen:          false,
		},
		{
			name:                "at threshold",
			consecutiveFailures: 5,
			failureCount:        5,
			expectOpen:          true,
		},
		{
			name:                "above threshold",
			consecutiveFailures: 3,
			failureCount:        10,
			expectOpen:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			config := &ResilienceConfig{
				CircuitBreaker: &CircuitBreakerConfig{
					Name:                "test-threshold",
					MaxRequests:         1,
					Interval:            100 * time.Millisecond,
					Timeout:             1 * time.Second,
					ConsecutiveFailures: tt.consecutiveFailures,
				},
			}

			wrapper := NewResilienceWrapper(config)
			ctx := context.Background()

			// Generate failures
			for i := 0; i < tt.failureCount; i++ {
				_, err := wrapper.Execute(ctx, func() (string, error) {
					return "", errors.New("failure")
				})
				assert.Error(t, err)
			}

			// Check state
			if tt.expectOpen {
				assert.Equal(t, gobreaker.StateOpen, wrapper.State())
			} else {
				assert.Equal(t, gobreaker.StateClosed, wrapper.State())
			}
		})
	}
}

// TestCircuitBreakerSuccessThreshold tests recovery after successful requests
func TestCircuitBreakerSuccessThreshold(t *testing.T) {
	t.Parallel()

	config := &ResilienceConfig{
		CircuitBreaker: &CircuitBreakerConfig{
			Name:                "test-recovery",
			MaxRequests:         2, // Allow 2 requests in half-open
			Interval:            100 * time.Millisecond,
			Timeout:             200 * time.Millisecond,
			ConsecutiveFailures: 2,
		},
	}

	wrapper := NewResilienceWrapper(config)
	ctx := context.Background()

	// Open the circuit with failures
	for range 2 {
		_, _ = wrapper.Execute(ctx, func() (string, error) {
			return "", errors.New("failure")
		})
	}
	assert.Equal(t, gobreaker.StateOpen, wrapper.State())

	// Wait for half-open (add buffer for race detector)
	time.Sleep(350 * time.Millisecond)

	// Successful requests should close the circuit (may need multiple in half-open)
	for range 3 {
		_, err := wrapper.Execute(ctx, func() (string, error) {
			return "success", nil
		})
		if err == nil && wrapper.State() == gobreaker.StateClosed {
			break
		}
	}
	assert.Equal(t, gobreaker.StateClosed, wrapper.State())
}

// TestCircuitBreakerConcurrentRequests tests concurrent requests during state transitions
func TestCircuitBreakerConcurrentRequests(t *testing.T) {
	t.Parallel()

	config := &ResilienceConfig{
		CircuitBreaker: &CircuitBreakerConfig{
			Name:                "test-concurrent",
			MaxRequests:         1,
			Interval:            100 * time.Millisecond,
			Timeout:             500 * time.Millisecond,
			ConsecutiveFailures: 3,
		},
	}

	wrapper := NewResilienceWrapper(config)
	ctx := context.Background()

	// Open the circuit
	for range 3 {
		_, _ = wrapper.Execute(ctx, func() (string, error) {
			return "", errors.New("failure")
		})
	}

	// Launch concurrent requests during open state
	var wg sync.WaitGroup
	successCount := atomic.Int32{}
	failureCount := atomic.Int32{}

	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := wrapper.Execute(ctx, func() (string, error) {
				return "success", nil
			})
			if err != nil {
				failureCount.Add(1)
			} else {
				successCount.Add(1)
			}
		}()
	}

	wg.Wait()

	// All should fail during open state
	assert.Greater(t, failureCount.Load(), int32(0))
	assert.Equal(t, int32(10), failureCount.Load())
}

// TestRetryExponentialBackoff tests exponential backoff timing
func TestRetryExponentialBackoff(t *testing.T) {
	t.Parallel()

	config := &ResilienceConfig{
		Retry: &RetryConfig{
			MaxRetries:          3,
			InitialInterval:     100 * time.Millisecond,
			MaxInterval:         1 * time.Second,
			Multiplier:          2.0,
			RandomizationFactor: 0.0, // Disable jitter for predictable testing
		},
	}

	wrapper := NewResilienceWrapper(config)
	ctx := context.Background()

	attemptCount := atomic.Int32{}
	startTime := time.Now()

	_, err := wrapper.Execute(ctx, func() (string, error) {
		attemptCount.Add(1)
		return "", &ServerError{StatusCode: 503, Message: "service unavailable"}
	})

	elapsed := time.Since(startTime)
	attempts := attemptCount.Load()

	// Should have tried 4 times total (1 initial + 3 retries)
	assert.Equal(t, int32(4), attempts)
	assert.Error(t, err)

	// Total backoff: 100ms + 200ms + 400ms = 700ms minimum
	// Allow some overhead for execution time
	assert.GreaterOrEqual(t, elapsed, 700*time.Millisecond)
	assert.Less(t, elapsed, 2*time.Second) // Should not exceed max with 0 jitter
}

// TestRetryMaxRetriesLimit tests max retries limit
func TestRetryMaxRetriesLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		maxRetries     uint64
		expectedCalls  int32
		expectError    bool
		successOnTry   int32 // 0 means never succeed
	}{
		{
			name:          "no retries",
			maxRetries:    0,
			expectedCalls: 1,
			expectError:   true,
			successOnTry:  0,
		},
		{
			name:          "max 3 retries all fail",
			maxRetries:    3,
			expectedCalls: 4, // 1 initial + 3 retries
			expectError:   true,
			successOnTry:  0,
		},
		{
			name:          "succeed on second try",
			maxRetries:    3,
			expectedCalls: 2,
			expectError:   false,
			successOnTry:  2,
		},
		{
			name:          "succeed on last retry",
			maxRetries:    3,
			expectedCalls: 4,
			expectError:   false,
			successOnTry:  4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			config := &ResilienceConfig{
				Retry: &RetryConfig{
					MaxRetries:          tt.maxRetries,
					InitialInterval:     10 * time.Millisecond,
					MaxInterval:         100 * time.Millisecond,
					Multiplier:          2.0,
					RandomizationFactor: 0.0,
				},
			}

			wrapper := NewResilienceWrapper(config)
			ctx := context.Background()

			attemptCount := atomic.Int32{}

			_, err := wrapper.Execute(ctx, func() (string, error) {
				current := attemptCount.Add(1)
				if tt.successOnTry > 0 && current == tt.successOnTry {
					return "success", nil
				}
				return "", &ServerError{StatusCode: 500, Message: "error"}
			})

			assert.Equal(t, tt.expectedCalls, attemptCount.Load())
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestRetryIsRetryable tests isRetryable() with different error types
func TestRetryIsRetryable(t *testing.T) {
	t.Parallel()

	wrapper := NewResilienceWrapper(&ResilienceConfig{})

	tests := []struct {
		name       string
		err        error
		retryable  bool
	}{
		{
			name:      "nil error",
			err:       nil,
			retryable: false,
		},
		{
			name:      "rate limit error",
			err:       &RateLimitError{RetryAfter: 1 * time.Second, Message: "rate limited"},
			retryable: true,
		},
		{
			name:      "server error 500",
			err:       &ServerError{StatusCode: 500, Message: "internal server error"},
			retryable: true,
		},
		{
			name:      "server error 503",
			err:       &ServerError{StatusCode: 503, Message: "service unavailable"},
			retryable: true,
		},
		{
			name:      "server error 400",
			err:       &ServerError{StatusCode: 400, Message: "bad request"},
			retryable: false,
		},
		{
			name:      "context canceled",
			err:       context.Canceled,
			retryable: false,
		},
		{
			name:      "context deadline exceeded",
			err:       context.DeadlineExceeded,
			retryable: false,
		},
		{
			name:      "network timeout",
			err:       &net.DNSError{IsTimeout: true},
			retryable: true,
		},
		{
			name:      "connection refused",
			err:       errors.New("connection refused"),
			retryable: true,
		},
		{
			name:      "connection reset",
			err:       errors.New("connection reset by peer"),
			retryable: true,
		},
		{
			name:      "no such host",
			err:       errors.New("no such host"),
			retryable: true,
		},
		{
			name:      "network unreachable",
			err:       errors.New("network is unreachable"),
			retryable: true,
		},
		{
			name:      "i/o timeout",
			err:       errors.New("i/o timeout"),
			retryable: true,
		},
		{
			name:      "EOF error",
			err:       errors.New("unexpected EOF"),
			retryable: true,
		},
		{
			name:      "broken pipe",
			err:       errors.New("broken pipe"),
			retryable: true,
		},
		{
			name:      "unknown error",
			err:       errors.New("something went wrong"),
			retryable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := wrapper.isRetryable(tt.err)
			assert.Equal(t, tt.retryable, result)
		})
	}
}

// TestRetryContextCancellation tests context cancellation during retry
func TestRetryContextCancellation(t *testing.T) {
	t.Parallel()

	config := &ResilienceConfig{
		Retry: &RetryConfig{
			MaxRetries:      10,
			InitialInterval: 100 * time.Millisecond,
			MaxInterval:     1 * time.Second,
			Multiplier:      2.0,
		},
	}

	wrapper := NewResilienceWrapper(config)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	attemptCount := atomic.Int32{}
	startTime := time.Now()

	_, err := wrapper.Execute(ctx, func() (string, error) {
		attemptCount.Add(1)
		return "", &ServerError{StatusCode: 503, Message: "unavailable"}
	})

	elapsed := time.Since(startTime)

	// Should stop retrying when context is canceled
	assert.Error(t, err)
	// Should not complete all 10 retries
	assert.Less(t, attemptCount.Load(), int32(10))
	// Should complete within timeout + small buffer
	assert.Less(t, elapsed, 500*time.Millisecond)
}

// TestRateLimiter429Handling tests 429 response handling
func TestRateLimiter429Handling(t *testing.T) {
	t.Parallel()

	config := &ResilienceConfig{
		RateLimiter: &RateLimiterConfig{
			InitialBackoff:    100 * time.Millisecond,
			MaxBackoff:        1 * time.Second,
			BackoffMultiplier: 2.0,
		},
	}

	wrapper := NewResilienceWrapper(config)
	ctx := context.Background()

	// First request triggers rate limit
	_, err := wrapper.Execute(ctx, func() (string, error) {
		return "", &RateLimitError{RetryAfter: 0, Message: "too many requests"}
	})
	assert.Error(t, err)
	var rateErr *RateLimitError
	assert.ErrorAs(t, err, &rateErr)

	// Immediate next request should wait
	wait := wrapper.rateLimiter.shouldWait()
	assert.Greater(t, wait, time.Duration(0))
	assert.LessOrEqual(t, wait, 100*time.Millisecond)

	// Wait longer than backoff and verify it clears (add buffer for race detector)
	time.Sleep(200 * time.Millisecond)
	wait = wrapper.rateLimiter.shouldWait()
	assert.LessOrEqual(t, wait, 10*time.Millisecond, "backoff should be mostly cleared")
}

// TestRateLimiterRetryAfterHeader tests Retry-After header parsing
func TestRateLimiterRetryAfterHeader(t *testing.T) {
	t.Parallel()

	config := &ResilienceConfig{
		RateLimiter: &RateLimiterConfig{
			InitialBackoff:    100 * time.Millisecond,
			MaxBackoff:        5 * time.Second,
			BackoffMultiplier: 2.0,
		},
	}

	wrapper := NewResilienceWrapper(config)
	ctx := context.Background()

	// Server provides Retry-After
	retryAfter := 500 * time.Millisecond
	_, err := wrapper.Execute(ctx, func() (string, error) {
		return "", &RateLimitError{RetryAfter: retryAfter, Message: "rate limited"}
	})
	assert.Error(t, err)

	// Should use server-provided value
	wait := wrapper.rateLimiter.shouldWait()
	assert.Greater(t, wait, time.Duration(0))
	assert.LessOrEqual(t, wait, retryAfter)
}

// TestRateLimiterBackoffEscalation tests rate limit backoff escalation
func TestRateLimiterBackoffEscalation(t *testing.T) {
	t.Parallel()

	config := &ResilienceConfig{
		RateLimiter: &RateLimiterConfig{
			InitialBackoff:    100 * time.Millisecond,
			MaxBackoff:        1 * time.Second,
			BackoffMultiplier: 2.0,
		},
	}

	wrapper := NewResilienceWrapper(config)
	ctx := context.Background()

	// First rate limit
	_, _ = wrapper.Execute(ctx, func() (string, error) {
		return "", &RateLimitError{Message: "rate limited"}
	})
	firstBackoff := wrapper.rateLimiter.currentBackoff
	assert.Equal(t, 100*time.Millisecond, firstBackoff)

	// Wait and trigger second rate limit
	time.Sleep(150 * time.Millisecond)
	_, _ = wrapper.Execute(ctx, func() (string, error) {
		return "", &RateLimitError{Message: "rate limited"}
	})
	secondBackoff := wrapper.rateLimiter.currentBackoff
	assert.Equal(t, 200*time.Millisecond, secondBackoff)

	// Wait and trigger third rate limit
	time.Sleep(250 * time.Millisecond)
	_, _ = wrapper.Execute(ctx, func() (string, error) {
		return "", &RateLimitError{Message: "rate limited"}
	})
	thirdBackoff := wrapper.rateLimiter.currentBackoff
	assert.Equal(t, 400*time.Millisecond, thirdBackoff)

	// Success should reset
	wrapper.rateLimiter.recordSuccess()
	assert.Equal(t, config.RateLimiter.InitialBackoff, wrapper.rateLimiter.currentBackoff)
}

// TestRateLimiterMaxBackoff tests max backoff ceiling
func TestRateLimiterMaxBackoff(t *testing.T) {
	t.Parallel()

	config := &ResilienceConfig{
		RateLimiter: &RateLimiterConfig{
			InitialBackoff:    100 * time.Millisecond,
			MaxBackoff:        500 * time.Millisecond,
			BackoffMultiplier: 2.0,
		},
	}

	wrapper := NewResilienceWrapper(config)
	ctx := context.Background()

	// Trigger multiple rate limits to exceed max
	for range 5 {
		_, _ = wrapper.Execute(ctx, func() (string, error) {
			return "", &RateLimitError{Message: "rate limited"}
		})
		time.Sleep(100 * time.Millisecond)
	}

	// Should be capped at max backoff
	assert.Equal(t, config.RateLimiter.MaxBackoff, wrapper.rateLimiter.currentBackoff)
}

// TestIntegrationFullResilienceStack tests circuit breaker + retry + rate limiter interaction
func TestIntegrationFullResilienceStack(t *testing.T) {
	t.Parallel()

	// Test 1: Verify retries work with rate limit errors
	retryConfig := &ResilienceConfig{
		Retry: &RetryConfig{
			MaxRetries:          2,
			InitialInterval:     5 * time.Millisecond,
			MaxInterval:         20 * time.Millisecond,
			Multiplier:          2.0,
			RandomizationFactor: 0.0,
		},
	}

	retryWrapper := NewResilienceWrapper(retryConfig)
	ctx := context.Background()

	attemptCount := atomic.Int32{}
	attemptCount.Store(0)
	_, err := retryWrapper.Execute(ctx, func() (string, error) {
		attemptCount.Add(1)
		return "", &RateLimitError{Message: "rate limited"}
	})
	assert.Error(t, err)
	// Should have retried (initial + 2 retries = 3 attempts)
	assert.Equal(t, int32(3), attemptCount.Load(), "rate limit should trigger all retries")

	// Test 2: Verify circuit breaker opens and blocks requests
	cbConfig := &ResilienceConfig{
		CircuitBreaker: &CircuitBreakerConfig{
			Name:                "integration-test",
			MaxRequests:         1,
			Interval:            0,              // Disable count clearing
			Timeout:             10 * time.Second, // Long timeout so circuit stays open
			ConsecutiveFailures: 3,
		},
	}

	cbWrapper := NewResilienceWrapper(cbConfig)

	// Trip the circuit breaker with 3 failures
	for range 3 {
		_, _ = cbWrapper.Execute(ctx, func() (string, error) {
			return "", errors.New("failure")
		})
	}

	// Circuit should be open now
	assert.Equal(t, gobreaker.StateOpen, cbWrapper.State())

	// Next request should fail fast with circuit open error
	attemptCount.Store(0)
	_, err = cbWrapper.Execute(ctx, func() (string, error) {
		attemptCount.Add(1)
		return "should not reach", nil
	})
	assert.ErrorIs(t, err, ErrCircuitOpen)
	// Should not call the operation when circuit is open
	assert.Equal(t, int32(0), attemptCount.Load())
}

// TestIntegrationSimulatedAPIFailures tests recovery from simulated API failures
func TestIntegrationSimulatedAPIFailures(t *testing.T) {
	t.Parallel()

	config := &ResilienceConfig{
		CircuitBreaker: &CircuitBreakerConfig{
			Name:                "api-sim",
			MaxRequests:         1,
			Interval:            100 * time.Millisecond,
			Timeout:             500 * time.Millisecond,
			ConsecutiveFailures: 3,
		},
		Retry: &RetryConfig{
			MaxRetries:          5,
			InitialInterval:     50 * time.Millisecond,
			MaxInterval:         500 * time.Millisecond,
			Multiplier:          2.0,
			RandomizationFactor: 0.1,
		},
		RateLimiter: &RateLimiterConfig{
			InitialBackoff:    100 * time.Millisecond,
			MaxBackoff:        2 * time.Second,
			BackoffMultiplier: 2.0,
		},
	}

	wrapper := NewResilienceWrapper(config)
	ctx := context.Background()

	// Simulate intermittent failures followed by recovery
	callCount := atomic.Int32{}

	_, err := wrapper.Execute(ctx, func() (string, error) {
		count := callCount.Add(1)
		// Fail first 3 times, then succeed
		if count <= 3 {
			return "", &ServerError{StatusCode: 503, Message: "service unavailable"}
		}
		return "recovered", nil
	})

	// Should eventually succeed via retry
	assert.NoError(t, err)
	assert.Equal(t, int32(4), callCount.Load())
	assert.Equal(t, gobreaker.StateClosed, wrapper.State())
}

// TestDefaultConfigs tests default configuration constructors
func TestDefaultConfigs(t *testing.T) {
	t.Parallel()

	t.Run("circuit breaker defaults", func(t *testing.T) {
		t.Parallel()
		config := DefaultCircuitBreakerConfig()
		assert.Equal(t, "claude-api", config.Name)
		assert.Equal(t, uint32(1), config.MaxRequests)
		assert.Equal(t, 60*time.Second, config.Interval)
		assert.Equal(t, 30*time.Second, config.Timeout)
		assert.Equal(t, uint32(5), config.ConsecutiveFailures)
	})

	t.Run("retry defaults", func(t *testing.T) {
		t.Parallel()
		config := DefaultRetryConfig()
		assert.Equal(t, uint64(3), config.MaxRetries)
		assert.Equal(t, 500*time.Millisecond, config.InitialInterval)
		assert.Equal(t, 30*time.Second, config.MaxInterval)
		assert.Equal(t, 2.0, config.Multiplier)
		assert.Equal(t, 0.5, config.RandomizationFactor)
	})

	t.Run("rate limiter defaults", func(t *testing.T) {
		t.Parallel()
		config := DefaultRateLimiterConfig()
		assert.Equal(t, 1*time.Second, config.InitialBackoff)
		assert.Equal(t, 60*time.Second, config.MaxBackoff)
		assert.Equal(t, 2.0, config.BackoffMultiplier)
	})
}

// TestErrorTypes tests custom error types
func TestErrorTypes(t *testing.T) {
	t.Parallel()

	t.Run("RateLimitError with RetryAfter", func(t *testing.T) {
		t.Parallel()
		err := &RateLimitError{
			RetryAfter: 5 * time.Second,
			Message:    "too many requests",
		}
		expected := "rate limited: retry after 5s"
		assert.Equal(t, expected, err.Error())
	})

	t.Run("RateLimitError without RetryAfter", func(t *testing.T) {
		t.Parallel()
		err := &RateLimitError{
			Message: "quota exceeded",
		}
		expected := "rate limited: quota exceeded"
		assert.Equal(t, expected, err.Error())
	})

	t.Run("ServerError", func(t *testing.T) {
		t.Parallel()
		err := &ServerError{
			StatusCode: 503,
			Message:    "service temporarily unavailable",
		}
		expected := "server error 503: service temporarily unavailable"
		assert.Equal(t, expected, err.Error())
	})
}

// TestNewResilienceWrapperNilConfig tests wrapper creation with nil config
func TestNewResilienceWrapperNilConfig(t *testing.T) {
	t.Parallel()

	wrapper := NewResilienceWrapper(nil)
	require.NotNil(t, wrapper)

	// Should work without any resilience features
	ctx := context.Background()
	result, err := wrapper.Execute(ctx, func() (string, error) {
		return "success", nil
	})
	assert.NoError(t, err)
	assert.Equal(t, "success", result)

	// State should return closed when no circuit breaker
	assert.Equal(t, gobreaker.StateClosed, wrapper.State())

	// Counts should return empty when no circuit breaker
	counts := wrapper.Counts()
	assert.Equal(t, gobreaker.Counts{}, counts)
}

// TestWrapperStateAndCounts tests State() and Counts() methods
func TestWrapperStateAndCounts(t *testing.T) {
	t.Parallel()

	config := &ResilienceConfig{
		CircuitBreaker: &CircuitBreakerConfig{
			Name:                "test-state",
			MaxRequests:         1,
			Interval:            100 * time.Millisecond,
			Timeout:             1 * time.Second,
			ConsecutiveFailures: 2,
		},
	}

	wrapper := NewResilienceWrapper(config)
	ctx := context.Background()

	// Initial state
	assert.Equal(t, gobreaker.StateClosed, wrapper.State())
	counts := wrapper.Counts()
	assert.Equal(t, uint32(0), counts.Requests)
	assert.Equal(t, uint32(0), counts.ConsecutiveFailures)

	// Generate a failure
	_, _ = wrapper.Execute(ctx, func() (string, error) {
		return "", errors.New("failure")
	})

	counts = wrapper.Counts()
	assert.Equal(t, uint32(1), counts.Requests)
	assert.Equal(t, uint32(1), counts.TotalFailures)
	assert.Equal(t, uint32(1), counts.ConsecutiveFailures)

	// Generate another failure to open circuit
	_, _ = wrapper.Execute(ctx, func() (string, error) {
		return "", errors.New("failure")
	})

	// Circuit should be open after 2 consecutive failures
	// Note: gobreaker resets ALL counts when state transitions, so we only verify
	// the circuit opened (which proves the failures were counted correctly)
	assert.Equal(t, gobreaker.StateOpen, wrapper.State())
}

// TestIsNetworkError tests network error detection
func TestIsNetworkError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		err       error
		isNetwork bool
	}{
		{
			name:      "nil error",
			err:       nil,
			isNetwork: false,
		},
		{
			name:      "timeout error",
			err:       &timeoutError{},
			isNetwork: true,
		},
		{
			name:      "connection refused",
			err:       errors.New("connection refused"),
			isNetwork: true,
		},
		{
			name:      "connection reset",
			err:       errors.New("connection reset by peer"),
			isNetwork: true,
		},
		{
			name:      "no such host",
			err:       errors.New("no such host: example.com"),
			isNetwork: true,
		},
		{
			name:      "network unreachable",
			err:       errors.New("network is unreachable"),
			isNetwork: true,
		},
		{
			name:      "i/o timeout",
			err:       errors.New("i/o timeout"),
			isNetwork: true,
		},
		{
			name:      "EOF",
			err:       errors.New("EOF"),
			isNetwork: true,
		},
		{
			name:      "broken pipe",
			err:       errors.New("write: broken pipe"),
			isNetwork: true,
		},
		{
			name:      "case insensitive",
			err:       errors.New("Connection Refused"),
			isNetwork: true,
		},
		{
			name:      "non-network error",
			err:       errors.New("invalid argument"),
			isNetwork: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := isNetworkError(tt.err)
			assert.Equal(t, tt.isNetwork, result)
		})
	}
}

// timeoutError is a test helper that implements net.Error
type timeoutError struct{}

func (e *timeoutError) Error() string   { return "timeout" }
func (e *timeoutError) Timeout() bool   { return true }
func (e *timeoutError) Temporary() bool { return true }

// TestRetryWithJitter tests that randomization factor adds jitter
func TestRetryWithJitter(t *testing.T) {
	t.Parallel()

	config := &ResilienceConfig{
		Retry: &RetryConfig{
			MaxRetries:          2,
			InitialInterval:     100 * time.Millisecond,
			MaxInterval:         1 * time.Second,
			Multiplier:          2.0,
			RandomizationFactor: 0.5, // 50% jitter
		},
	}

	wrapper := NewResilienceWrapper(config)
	ctx := context.Background()

	attemptTimes := make([]time.Time, 0)
	var mu sync.Mutex

	_, _ = wrapper.Execute(ctx, func() (string, error) {
		mu.Lock()
		attemptTimes = append(attemptTimes, time.Now())
		mu.Unlock()
		return "", &ServerError{StatusCode: 500, Message: "error"}
	})

	// With jitter, intervals should vary
	// We can't test exact values, but we can verify retries happened
	assert.Equal(t, 3, len(attemptTimes)) // 1 initial + 2 retries
}

// TestCircuitBreakerCounts tests that circuit breaker counts are tracked correctly
func TestCircuitBreakerCounts(t *testing.T) {
	t.Parallel()

	config := &ResilienceConfig{
		CircuitBreaker: &CircuitBreakerConfig{
			Name:                "test-counts",
			MaxRequests:         10,
			Interval:            1 * time.Second,
			Timeout:             1 * time.Second,
			ConsecutiveFailures: 5,
		},
	}

	wrapper := NewResilienceWrapper(config)
	ctx := context.Background()

	// Mix of successes and failures
	for range 3 {
		_, _ = wrapper.Execute(ctx, func() (string, error) {
			return "success", nil
		})
	}

	for range 2 {
		_, _ = wrapper.Execute(ctx, func() (string, error) {
			return "", errors.New("failure")
		})
	}

	counts := wrapper.Counts()
	assert.Equal(t, uint32(5), counts.Requests)
	assert.Equal(t, uint32(3), counts.TotalSuccesses)
	assert.Equal(t, uint32(2), counts.TotalFailures)
	assert.Equal(t, uint32(2), counts.ConsecutiveFailures)
	assert.Equal(t, uint32(0), counts.ConsecutiveSuccesses)
}
