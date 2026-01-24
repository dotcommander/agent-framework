package client

import (
	"context"
	"sync"

	"github.com/dotcommander/agent-sdk-go/claude"
)

// SyntheticClient is a mock client for testing that doesn't make real API calls.
// It returns configurable responses and tracks all calls for assertions.
type SyntheticClient struct {
	mu sync.RWMutex

	// Response configuration
	response      string
	responseQueue []string
	responseFunc  func(prompt string) string
	queryError    error

	// Call tracking
	calls []SyntheticCall
}

// SyntheticCall records a single call to the synthetic client.
type SyntheticCall struct {
	Method string
	Prompt string
}

// NewSyntheticClient creates a new synthetic client with a default response.
func NewSyntheticClient() *SyntheticClient {
	return &SyntheticClient{
		response: "synthetic response",
	}
}

// SetResponse sets a fixed response for all queries.
func (c *SyntheticClient) SetResponse(response string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.response = response
	c.responseQueue = nil
	c.responseFunc = nil
}

// SetResponseQueue sets a queue of responses. Each query consumes the next response.
// When exhausted, falls back to the fixed response.
func (c *SyntheticClient) SetResponseQueue(responses ...string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.responseQueue = responses
	c.responseFunc = nil
}

// SetResponseFunc sets a function that generates responses based on the prompt.
func (c *SyntheticClient) SetResponseFunc(fn func(prompt string) string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.responseFunc = fn
	c.responseQueue = nil
}

// SetError configures the client to return an error on queries.
func (c *SyntheticClient) SetError(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.queryError = err
}

// Query returns the configured response and tracks the call.
func (c *SyntheticClient) Query(ctx context.Context, prompt string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.calls = append(c.calls, SyntheticCall{Method: "Query", Prompt: prompt})

	if c.queryError != nil {
		return "", c.queryError
	}

	return c.getResponse(prompt), nil
}

// QueryStream returns the configured response as a single message in the channel.
func (c *SyntheticClient) QueryStream(ctx context.Context, prompt string) (<-chan Message, <-chan error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.calls = append(c.calls, SyntheticCall{Method: "QueryStream", Prompt: prompt})

	msgChan := make(chan Message, 1)
	errChan := make(chan error, 1)

	if c.queryError != nil {
		errChan <- c.queryError
		close(msgChan)
		close(errChan)
		return msgChan, errChan
	}

	response := c.getResponse(prompt)
	msgChan <- c.makeMessage(response)
	close(msgChan)
	close(errChan)
	return msgChan, errChan
}

// Close is a no-op for the synthetic client.
func (c *SyntheticClient) Close() error {
	return nil
}

// Calls returns all recorded calls. Use for test assertions.
func (c *SyntheticClient) Calls() []SyntheticCall {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]SyntheticCall, len(c.calls))
	copy(result, c.calls)
	return result
}

// CallCount returns the total number of calls made.
func (c *SyntheticClient) CallCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.calls)
}

// LastCall returns the most recent call, or an empty SyntheticCall if none.
func (c *SyntheticClient) LastCall() SyntheticCall {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.calls) == 0 {
		return SyntheticCall{}
	}
	return c.calls[len(c.calls)-1]
}

// Reset clears all recorded calls and resets to default response.
func (c *SyntheticClient) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = nil
	c.response = "synthetic response"
	c.responseQueue = nil
	c.responseFunc = nil
	c.queryError = nil
}

// getResponse returns the next response. Must be called with lock held.
func (c *SyntheticClient) getResponse(prompt string) string {
	if c.responseFunc != nil {
		return c.responseFunc(prompt)
	}

	if len(c.responseQueue) > 0 {
		response := c.responseQueue[0]
		c.responseQueue = c.responseQueue[1:]
		return response
	}

	return c.response
}

// makeMessage creates an AssistantMessage with the given text content.
func (c *SyntheticClient) makeMessage(text string) Message {
	return &claude.AssistantMessage{
		MessageType: "assistant",
		Model:       "synthetic",
		Content: []claude.ContentBlock{
			&claude.TextBlock{
				MessageType: "text",
				Text:        text,
			},
		},
	}
}
