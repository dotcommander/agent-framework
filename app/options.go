package app

import (
	"fmt"

	"github.com/dotcommander/agent-sdk-go/claude"
	"github.com/dotcommander/agent-framework/client"
	"github.com/dotcommander/agent-framework/output"
	"github.com/dotcommander/agent-framework/tools"
)

// Option is a functional option for configuring App.
type Option func(*App)

// WithSystemPrompt sets the system prompt.
func WithSystemPrompt(prompt string) Option {
	return func(a *App) {
		a.config.Client.SystemPrompt = prompt
		a.config.Client.SDKOptions = append(a.config.Client.SDKOptions,
			claude.WithSystemPrompt(prompt))
	}
}

// WithModel sets the AI model.
func WithModel(model string) Option {
	return func(a *App) {
		a.config.Client.Model = model
		a.config.Client.SDKOptions = append(a.config.Client.SDKOptions,
			claude.WithModel(model))
	}
}

// WithProvider sets the AI provider.
func WithProvider(provider string) Option {
	return func(a *App) {
		a.config.Provider.Provider = provider
	}
}

// WithTool adds a tool to the registry.
func WithTool(tool *tools.Tool) Option {
	return func(a *App) {
		if err := a.tools.Register(tool); err != nil {
			a.initErrs = append(a.initErrs, fmt.Errorf("register tool %q: %w", tool.Name, err))
		}
	}
}

// WithOutputFormat registers a custom output formatter.
func WithOutputFormat(formatter output.Formatter) Option {
	return func(a *App) {
		a.output.RegisterFormatter(formatter)
	}
}

// WithRunFunc sets the custom run function.
func WithRunFunc(fn RunFunc) Option {
	return func(a *App) {
		a.runFunc = fn
	}
}

// WithClient sets a custom client (useful for testing).
func WithClient(c client.Client) Option {
	return func(a *App) {
		a.client = c
	}
}

// WithSDKOption adds a Claude SDK client option.
func WithSDKOption(opt claude.ClientOption) Option {
	return func(a *App) {
		a.config.Client.SDKOptions = append(a.config.Client.SDKOptions, opt)
	}
}
