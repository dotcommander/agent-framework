// Package client provides a clean wrapper around the Claude SDK client.
package client

import (
	"context"

	"github.com/dotcommander/agent-sdk-go/claude"
)

// Message represents a message from the AI.
type Message = claude.Message

// Tool represents a tool available to the AI.
type Tool struct {
	Name        string
	Description string
	InputSchema map[string]any
	Handler     func(ctx context.Context, input map[string]any) (any, error)
}

// Client provides a simplified interface for interacting with Claude.
type Client interface {
	// Query sends a prompt and returns the complete response.
	Query(ctx context.Context, prompt string) (string, error)

	// QueryStream sends a prompt and streams the response.
	QueryStream(ctx context.Context, prompt string) (<-chan Message, <-chan error)

	// WithSystemPrompt returns a new client with the given system prompt.
	WithSystemPrompt(prompt string) Client

	// WithTools returns a new client with the given tools.
	WithTools(tools ...*Tool) Client

	// Close releases resources associated with the client.
	Close() error
}

// clientImpl implements the Client interface.
type clientImpl struct {
	claude    claude.Client
	connected bool
}

// New creates a new Client with the given SDK options.
func New(ctx context.Context, opts ...claude.ClientOption) (Client, error) {
	c, err := claude.NewClient(opts...)
	if err != nil {
		return nil, err
	}

	// Connect immediately
	if err := c.Connect(ctx); err != nil {
		return nil, err
	}

	return &clientImpl{
		claude:    c,
		connected: true,
	}, nil
}

// Query sends a prompt and returns the complete response.
func (c *clientImpl) Query(ctx context.Context, prompt string) (string, error) {
	if !c.connected {
		return "", ErrNotConnected
	}
	return c.claude.Query(ctx, prompt)
}

// QueryStream sends a prompt and streams the response.
func (c *clientImpl) QueryStream(ctx context.Context, prompt string) (<-chan Message, <-chan error) {
	if !c.connected {
		msgChan := make(chan Message)
		errChan := make(chan error, 1)
		errChan <- ErrNotConnected
		close(msgChan)
		close(errChan)
		return msgChan, errChan
	}
	return c.claude.QueryStream(ctx, prompt)
}

// WithSystemPrompt returns a new client with the given system prompt.
// Note: This is a simplified implementation. In production, you would create
// a new client with the system prompt option.
func (c *clientImpl) WithSystemPrompt(prompt string) Client {
	// For now, return self - a full implementation would create a new client
	return c
}

// WithTools returns a new client with the given tools.
// Note: This is a simplified implementation. In production, you would create
// a new client with the tools configured.
func (c *clientImpl) WithTools(tools ...*Tool) Client {
	// For now, return self - a full implementation would configure MCP servers
	return c
}

// Close releases resources associated with the client.
func (c *clientImpl) Close() error {
	if !c.connected {
		return nil
	}
	c.connected = false
	return c.claude.Disconnect()
}
