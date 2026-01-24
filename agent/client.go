package agent

import (
	"context"

	"github.com/dotcommander/agent-framework/client"
	"github.com/dotcommander/agent-sdk-go/claude"
)

// SimpleClient provides direct LLM access without the full app.App machinery.
// Use for pipeline tools, scripts, and batch processing.
type SimpleClient struct {
	client client.Client
	model  string
}

// ClientOption configures a SimpleClient.
type ClientOption func(*clientConfig)

type clientConfig struct {
	model        string
	systemPrompt string
	env          map[string]string
	retry        bool
}

// WithClientModel sets the model for the client.
func WithClientModel(m string) ClientOption {
	return func(c *clientConfig) {
		c.model = ExpandModel(m)
	}
}

// WithClientSystem sets the system prompt.
func WithClientSystem(prompt string) ClientOption {
	return func(c *clientConfig) {
		c.systemPrompt = prompt
	}
}

// WithClientEnv sets environment variables (for provider config).
func WithClientEnv(env map[string]string) ClientOption {
	return func(c *clientConfig) {
		c.env = env
	}
}

// WithClientRetry enables retry with exponential backoff.
func WithClientRetry() ClientOption {
	return func(c *clientConfig) {
		c.retry = true
	}
}

// NewClient creates a SimpleClient for direct LLM queries.
// This is lighter weight than app.New() - no CLI, no tools, just queries.
//
// Example:
//
//	c, err := agent.NewClient(ctx,
//	    agent.WithClientModel("opus"),
//	    agent.WithClientSystem("You are helpful."),
//	    agent.WithClientRetry(),
//	)
//	defer c.Close()
//	response, err := c.Query(ctx, "Hello")
func NewClient(ctx context.Context, opts ...ClientOption) (*SimpleClient, error) {
	cfg := &clientConfig{
		model: DefaultModel,
		retry: true, // Retry by default
	}
	for _, opt := range opts {
		opt(cfg)
	}

	// Build SDK options
	sdkOpts := []claude.ClientOption{
		claude.WithModel(cfg.model),
	}
	if cfg.systemPrompt != "" {
		sdkOpts = append(sdkOpts, claude.WithSystemPrompt(cfg.systemPrompt))
	}
	if cfg.env != nil {
		sdkOpts = append(sdkOpts, claude.WithEnv(cfg.env))
	}

	// Build client options
	var clientOpts []client.ClientOption
	if cfg.retry {
		clientOpts = append(clientOpts, client.WithRetry(nil))
	}

	c, err := client.New(ctx, sdkOpts, clientOpts...)
	if err != nil {
		return nil, err
	}

	return &SimpleClient{
		client: c,
		model:  cfg.model,
	}, nil
}

// Query sends a prompt and returns the response.
func (c *SimpleClient) Query(ctx context.Context, prompt string) (string, error) {
	return c.client.Query(ctx, prompt)
}

// QueryJSON sends a prompt and parses the response as JSON.
//
// Example:
//
//	type Result struct {
//	    Value int `json:"value"`
//	}
//	result, err := c.QueryJSON[Result](ctx, "What is 2+2? Reply as JSON.")
func QueryJSON[T any](c *SimpleClient, ctx context.Context, prompt string) (*T, error) {
	response, err := c.Query(ctx, prompt)
	if err != nil {
		return nil, err
	}
	return ExtractJSON[T](response)
}

// QueryJSONArray sends a prompt and parses the response as a JSON array.
func QueryJSONArray[T any](c *SimpleClient, ctx context.Context, prompt string) ([]T, error) {
	response, err := c.Query(ctx, prompt)
	if err != nil {
		return nil, err
	}
	return ExtractJSONArray[T](response)
}

// Model returns the model being used.
func (c *SimpleClient) Model() string {
	return c.model
}

// Close releases resources.
func (c *SimpleClient) Close() error {
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}
