package client

import (
	"context"
	"fmt"

	"github.com/dotcommander/agent-sdk-go/claude"
)

// Provider represents an AI provider.
type Provider string

const (
	// ProviderAnthropic uses the Anthropic Claude API via agent-sdk-go.
	ProviderAnthropic Provider = "anthropic"

	// ProviderZAI uses synthetic responses for Z.AI API compatibility testing.
	ProviderZAI Provider = "zai"

	// ProviderSynthetic uses synthetic responses for testing.
	ProviderSynthetic Provider = "synthetic"
)

// ProviderConfig contains provider-specific configuration.
type ProviderConfig struct {
	Provider Provider
	Model    string
	APIKey   string
	BaseURL  string
}

// ProviderFactory creates clients for different providers.
type ProviderFactory struct {
	providers map[Provider]func(ctx context.Context, cfg ProviderConfig, opts ...claude.ClientOption) (Client, error)
}

// NewProviderFactory creates a new provider factory with default providers.
func NewProviderFactory() *ProviderFactory {
	f := &ProviderFactory{
		providers: make(map[Provider]func(ctx context.Context, cfg ProviderConfig, opts ...claude.ClientOption) (Client, error)),
	}

	// Register default providers
	f.Register(ProviderAnthropic, createAnthropicClient)
	f.Register(ProviderZAI, createZAIClient)
	f.Register(ProviderSynthetic, createSyntheticClient)

	return f
}

// Register registers a provider factory function.
func (f *ProviderFactory) Register(provider Provider, fn func(ctx context.Context, cfg ProviderConfig, opts ...claude.ClientOption) (Client, error)) {
	f.providers[provider] = fn
}

// Create creates a client for the specified provider.
func (f *ProviderFactory) Create(ctx context.Context, cfg ProviderConfig, opts ...claude.ClientOption) (Client, error) {
	fn, ok := f.providers[cfg.Provider]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrInvalidProvider, cfg.Provider)
	}
	return fn(ctx, cfg, opts...)
}

// createAnthropicClient creates a client using the Anthropic provider.
func createAnthropicClient(ctx context.Context, cfg ProviderConfig, opts ...claude.ClientOption) (Client, error) {
	// Build SDK options
	sdkOpts := make([]claude.ClientOption, 0, len(opts)+1)

	// Add model if specified
	if cfg.Model != "" {
		sdkOpts = append(sdkOpts, claude.WithModel(cfg.Model))
	}

	// Add user-provided options
	sdkOpts = append(sdkOpts, opts...)

	c, err := New(ctx, sdkOpts)
	if err != nil {
		return nil, fmt.Errorf("create anthropic client: %w", err)
	}
	return c, nil
}

// createZAIClient creates a Z.AI mock client for testing.
func createZAIClient(ctx context.Context, cfg ProviderConfig, opts ...claude.ClientOption) (Client, error) {
	return NewZAIClient(), nil
}

// createSyntheticClient creates a synthetic client for testing.
// Returns configurable responses without making real API calls.
func createSyntheticClient(ctx context.Context, cfg ProviderConfig, opts ...claude.ClientOption) (Client, error) {
	return NewSyntheticClient(), nil
}
