package client

import (
	"context"
	"sync"

	"github.com/dotcommander/agent-sdk-go/claude"
)

// MockClient is the interface for test clients that don't make real API calls.
type MockClient interface {
	Client

	// SetResponse sets a fixed response for all queries.
	SetResponse(response string)

	// SetResponseQueue sets a queue of responses consumed in order.
	SetResponseQueue(responses ...string)

	// SetResponseFunc sets a function that generates responses based on prompt.
	SetResponseFunc(fn func(prompt string) string)

	// SetError configures the client to return an error on queries.
	SetError(err error)

	// Calls returns all recorded calls for test assertions.
	Calls() []MockCall

	// CallCount returns the total number of calls made.
	CallCount() int

	// LastCall returns the most recent call.
	LastCall() MockCall

	// Reset clears all recorded calls and resets to default response.
	Reset()
}

// MockCall records a single call to a mock client.
type MockCall struct {
	Method string
	Prompt string
}

// mockClientBase is the shared implementation for mock clients.
type mockClientBase struct {
	mu sync.RWMutex

	model         string
	response      string
	responseQueue []string
	responseFunc  func(prompt string) string
	queryError    error
	calls         []MockCall
}

func newMockClientBase(model string) *mockClientBase {
	return &mockClientBase{
		model:    model,
		response: model + " response",
	}
}

func (c *mockClientBase) SetResponse(response string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.response = response
	c.responseQueue = nil
	c.responseFunc = nil
}

func (c *mockClientBase) SetResponseQueue(responses ...string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.responseQueue = responses
	c.responseFunc = nil
}

func (c *mockClientBase) SetResponseFunc(fn func(prompt string) string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.responseFunc = fn
	c.responseQueue = nil
}

func (c *mockClientBase) SetError(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.queryError = err
}

func (c *mockClientBase) Query(ctx context.Context, prompt string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.calls = append(c.calls, MockCall{Method: "Query", Prompt: prompt})

	if c.queryError != nil {
		return "", c.queryError
	}

	return c.getResponse(prompt), nil
}

func (c *mockClientBase) QueryStream(ctx context.Context, prompt string) (<-chan Message, <-chan error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.calls = append(c.calls, MockCall{Method: "QueryStream", Prompt: prompt})

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

func (c *mockClientBase) Close() error {
	return nil
}

func (c *mockClientBase) Calls() []MockCall {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]MockCall, len(c.calls))
	copy(result, c.calls)
	return result
}

func (c *mockClientBase) CallCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.calls)
}

func (c *mockClientBase) LastCall() MockCall {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.calls) == 0 {
		return MockCall{}
	}
	return c.calls[len(c.calls)-1]
}

func (c *mockClientBase) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = nil
	c.response = c.model + " response"
	c.responseQueue = nil
	c.responseFunc = nil
	c.queryError = nil
}

func (c *mockClientBase) getResponse(prompt string) string {
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

func (c *mockClientBase) makeMessage(text string) Message {
	return &claude.AssistantMessage{
		MessageType: "assistant",
		Model:       c.model,
		Content: []claude.ContentBlock{
			&claude.TextBlock{
				MessageType: "text",
				Text:        text,
			},
		},
	}
}
