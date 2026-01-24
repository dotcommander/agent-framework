package client

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestConcurrentQueryClose verifies there's no race condition between
// concurrent Query and Close calls.
func TestConcurrentQueryClose(t *testing.T) {
	t.Parallel()

	// Create a mock client implementation for testing
	client := &clientImpl{}
	client.connected.Store(true)

	var wg sync.WaitGroup
	const goroutines = 100

	// Launch multiple goroutines that query
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := context.Background()
			// Query will fail because we don't have a real claude client,
			// but we're testing the race condition on the connected field
			_, _ = client.Query(ctx, "test")
		}()
	}

	// Launch multiple goroutines that close
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Close will fail because we don't have a real claude client,
			// but we're testing the race condition on the connected field
			_ = client.Close()
		}()
	}

	// Launch multiple goroutines that check the state
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Just reading the connected state
			_ = client.connected.Load()
		}()
	}

	// Wait for all goroutines to complete
	wg.Wait()

	// Test passes if no race condition is detected
	// Run with: go test -race ./client/...
}

// TestConnectedStateTransitions verifies the connected state transitions work correctly.
func TestConnectedStateTransitions(t *testing.T) {
	t.Parallel()

	client := &clientImpl{}

	// Initial state should be false
	assert.False(t, client.connected.Load(), "initial state should be false")

	// Set to true
	client.connected.Store(true)
	assert.True(t, client.connected.Load(), "state should be true after Store(true)")

	// Set to false
	client.connected.Store(false)
	assert.False(t, client.connected.Load(), "state should be false after Store(false)")

	// Multiple transitions
	for i := 0; i < 100; i++ {
		client.connected.Store(i%2 == 0)
		expected := i%2 == 0
		assert.Equal(t, expected, client.connected.Load(), "state mismatch at iteration %d", i)
	}
}

// TestQueryWhenNotConnected verifies Query returns ErrNotConnected when client is not connected.
func TestQueryWhenNotConnected(t *testing.T) {
	t.Parallel()

	client := &clientImpl{}
	client.connected.Store(false)

	ctx := context.Background()
	result, err := client.Query(ctx, "test prompt")

	assert.Equal(t, "", result, "result should be empty")
	assert.ErrorIs(t, err, ErrNotConnected, "should return ErrNotConnected")
}

// TestQueryStreamWhenNotConnected verifies QueryStream returns ErrNotConnected when client is not connected.
func TestQueryStreamWhenNotConnected(t *testing.T) {
	t.Parallel()

	client := &clientImpl{}
	client.connected.Store(false)

	ctx := context.Background()
	msgChan, errChan := client.QueryStream(ctx, "test prompt")

	// Channels should be closed
	_, msgOpen := <-msgChan
	assert.False(t, msgOpen, "message channel should be closed")

	// Should receive ErrNotConnected on error channel
	err, errOpen := <-errChan
	assert.True(t, errOpen, "error channel should have an error")
	assert.ErrorIs(t, err, ErrNotConnected, "should receive ErrNotConnected")

	// Error channel should be closed after reading the error
	_, errStillOpen := <-errChan
	assert.False(t, errStillOpen, "error channel should be closed after error")
}

// TestCloseIdempotency verifies Close can be called multiple times safely.
func TestCloseIdempotency(t *testing.T) {
	t.Parallel()

	client := &clientImpl{}
	client.connected.Store(true)

	// First close sets connected to false
	err := client.Close()
	// Will fail because no real claude client, but that's ok
	_ = err

	assert.False(t, client.connected.Load(), "connected should be false after close")

	// Second close should be safe and return nil
	err = client.Close()
	assert.Nil(t, err, "second close should return nil")
	assert.False(t, client.connected.Load(), "connected should still be false")
}
