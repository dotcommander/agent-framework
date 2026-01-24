// Package config provides configuration types and functional options for agent applications.
package config

import (
	"time"

	"github.com/dotcommander/agent-sdk-go/claude"
)

// AppConfig contains application metadata.
type AppConfig struct {
	Name        string
	Version     string
	Description string
}

// ProviderConfig contains provider-specific settings.
type ProviderConfig struct {
	// Provider is the AI provider to use (anthropic, zai, synthetic).
	Provider string `json:"provider"`

	// Anthropic settings
	AnthropicAPIKey string `json:"-"` // Excluded from JSON serialization for security
	AnthropicModel  string `json:"anthropic_model,omitempty"`

	// ZAI settings
	ZAIAPIKey  string `json:"-"` // Excluded from JSON serialization for security
	ZAIModel   string `json:"zai_model,omitempty"`
	ZAIBaseURL string `json:"zai_base_url,omitempty"`

	// Synthetic settings (for testing)
	SyntheticResponse string `json:"synthetic_response,omitempty"`
}

// ClientConfig wraps SDK client options with additional agent-specific configuration.
type ClientConfig struct {
	// SystemPrompt is the system prompt to use for all queries.
	SystemPrompt string

	// Model is the default model to use.
	Model string

	// Timeout is the default timeout for queries.
	Timeout time.Duration

	// SDKOptions are passed directly to the Claude SDK client.
	SDKOptions []claude.ClientOption
}

// Config is the complete configuration for an agent application.
type Config struct {
	App      AppConfig
	Provider ProviderConfig
	Client   ClientConfig
}

// Option is a functional option for configuring Config.
type Option func(*Config)

// NewConfig creates a new Config with functional options.
func NewConfig(opts ...Option) *Config {
	cfg := &Config{
		Provider: ProviderConfig{
			Provider: "anthropic", // default
		},
		Client: ClientConfig{
			Timeout: 60 * time.Second,
		},
	}

	for _, opt := range opts {
		opt(cfg)
	}

	return cfg
}

// WithApp sets application metadata.
func WithApp(name, version, description string) Option {
	return func(c *Config) {
		c.App = AppConfig{
			Name:        name,
			Version:     version,
			Description: description,
		}
	}
}

// WithProvider sets the AI provider.
func WithProvider(provider string) Option {
	return func(c *Config) {
		c.Provider.Provider = provider
	}
}

// WithAnthropicKey sets the Anthropic API key.
func WithAnthropicKey(key string) Option {
	return func(c *Config) {
		c.Provider.AnthropicAPIKey = key
	}
}

// WithModel sets the default model.
func WithModel(model string) Option {
	return func(c *Config) {
		c.Client.Model = model
		c.Provider.AnthropicModel = model
	}
}

// WithSystemPrompt sets the system prompt.
func WithSystemPrompt(prompt string) Option {
	return func(c *Config) {
		c.Client.SystemPrompt = prompt
	}
}

// WithTimeout sets the default timeout.
func WithTimeout(timeout time.Duration) Option {
	return func(c *Config) {
		c.Client.Timeout = timeout
	}
}

// WithSDKOption adds a Claude SDK client option.
func WithSDKOption(opt claude.ClientOption) Option {
	return func(c *Config) {
		c.Client.SDKOptions = append(c.Client.SDKOptions, opt)
	}
}
