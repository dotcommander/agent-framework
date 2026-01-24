package agent

// Provider represents an LLM API provider configuration.
// Use for multi-provider support (Anthropic, Z.AI, Synthetic, etc.)
type Provider struct {
	Name       string // Display name
	BaseURL    string // API base URL (empty = default)
	AuthEnvVar string // Environment variable for auth token
	Model      string // Default model for this provider
}

// Common providers with sensible defaults.
var (
	// ProviderAnthropic is the default Anthropic API.
	ProviderAnthropic = &Provider{
		Name:  "anthropic",
		Model: ModelOpus,
	}

	// ProviderZAI is Z.AI's Claude-compatible API.
	ProviderZAI = &Provider{
		Name:       "zai",
		BaseURL:    "https://api.z.ai/api/anthropic",
		AuthEnvVar: "ZAI_API_KEY",
		Model:      "GLM-4.7",
	}

	// ProviderSynthetic is Synthetic.new's API.
	ProviderSynthetic = &Provider{
		Name:       "synthetic",
		BaseURL:    "https://api.synthetic.new/anthropic",
		AuthEnvVar: "SYNTHETIC_API_KEY",
		Model:      "hf:zai-org/GLM-4.7",
	}
)

// ProviderEnv returns environment variables for non-Anthropic providers.
// Pass to WithClientEnv() when creating a client.
//
// Example:
//
//	client, err := agent.NewClient(ctx,
//	    agent.WithClientModel(agent.ProviderZAI.Model),
//	    agent.WithClientEnv(agent.ProviderEnv(agent.ProviderZAI)),
//	)
func ProviderEnv(p *Provider) map[string]string {
	if p == nil || p.Name == "anthropic" {
		return nil
	}

	env := make(map[string]string)
	if p.BaseURL != "" {
		env["ANTHROPIC_BASE_URL"] = p.BaseURL
	}
	if p.AuthEnvVar != "" {
		// The SDK reads ANTHROPIC_AUTH_TOKEN, so we set it from the provider's env var
		env["ANTHROPIC_AUTH_TOKEN_VAR"] = p.AuthEnvVar
	}
	return env
}

// Providers returns a map of all built-in providers by name.
func Providers() map[string]*Provider {
	return map[string]*Provider{
		"anthropic": ProviderAnthropic,
		"zai":       ProviderZAI,
		"synthetic": ProviderSynthetic,
	}
}

// GetProvider returns a provider by name, or nil if not found.
func GetProvider(name string) *Provider {
	return Providers()[name]
}
