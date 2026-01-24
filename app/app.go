// Package app provides the main application structure.
package app

import (
	"context"
	"fmt"

	"github.com/dotcommander/agent-framework/client"
	"github.com/dotcommander/agent-framework/config"
	"github.com/dotcommander/agent-framework/output"
	"github.com/dotcommander/agent-framework/tools"
	"github.com/spf13/cobra"
)

// RunFunc is the main application run function.
type RunFunc func(ctx context.Context, app *App, args []string) error

// App represents the main application.
type App struct {
	name    string
	version string
	config  *config.Config
	client  client.Client
	tools   *tools.Registry
	output  *output.Dispatcher
	runFunc RunFunc
	rootCmd *cobra.Command
}

// New creates a new App with the given name and version.
func New(name, version string, opts ...Option) *App {
	app := &App{
		name:    name,
		version: version,
		config:  config.NewConfig(),
		tools:   tools.NewRegistry(),
		output:  output.NewDispatcher(),
	}

	// Apply options
	for _, opt := range opts {
		opt(app)
	}

	// Register default formatters
	app.output.RegisterFormatter(output.NewJSONFormatter(true))
	app.output.RegisterFormatter(output.NewMarkdownFormatter())
	app.output.RegisterFormatter(output.NewTextFormatter())

	// Build root command
	app.rootCmd = &cobra.Command{
		Use:     name,
		Short:   app.config.App.Description,
		Version: version,
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.execute(cmd.Context(), args)
		},
	}

	return app
}

// execute runs the application.
func (a *App) execute(ctx context.Context, args []string) error {
	// Initialize client if not already set
	if a.client == nil {
		factory := client.NewProviderFactory()
		providerCfg := client.ProviderConfig{
			Provider: client.Provider(a.config.Provider.Provider),
			Model:    a.config.Client.Model,
			APIKey:   a.config.Provider.AnthropicAPIKey,
		}

		c, err := factory.Create(ctx, providerCfg, a.config.Client.SDKOptions...)
		if err != nil {
			return fmt.Errorf("create client: %w", err)
		}
		a.client = c
		defer a.client.Close()
	}

	// Run custom function if provided
	if a.runFunc != nil {
		return a.runFunc(ctx, a, args)
	}

	// Default behavior: simple query
	if len(args) == 0 {
		return fmt.Errorf("no prompt provided")
	}

	prompt := args[0]
	response, err := a.client.Query(ctx, prompt)
	if err != nil {
		return fmt.Errorf("query: %w", err)
	}

	fmt.Println(response)
	return nil
}

// Run executes the application.
func (a *App) Run() error {
	return a.rootCmd.Execute()
}

// Client returns the AI client.
func (a *App) Client() client.Client {
	return a.client
}

// Tools returns the tool registry.
func (a *App) Tools() *tools.Registry {
	return a.tools
}

// Output returns the output dispatcher.
func (a *App) Output() *output.Dispatcher {
	return a.output
}

// Config returns the application configuration.
func (a *App) Config() *config.Config {
	return a.config
}

// RootCmd returns the root cobra command.
func (a *App) RootCmd() *cobra.Command {
	return a.rootCmd
}
