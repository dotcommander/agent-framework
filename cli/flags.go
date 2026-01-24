// Package cli provides CLI scaffolding utilities.
package cli

import (
	"github.com/spf13/cobra"
)

// StandardFlags represents common CLI flags.
type StandardFlags struct {
	Model    string
	Output   string
	Format   string
	Verbose  bool
	APIKey   string
	BaseURL  string
	Provider string
}

// AddStandardFlags adds standard flags to a cobra command.
func AddStandardFlags(cmd *cobra.Command, flags *StandardFlags) {
	cmd.Flags().StringVarP(&flags.Model, "model", "m", "", "AI model to use")
	cmd.Flags().StringVarP(&flags.Output, "output", "o", "", "Output file (default: stdout)")
	cmd.Flags().StringVarP(&flags.Format, "format", "f", "text", "Output format (text, json, markdown)")
	cmd.Flags().BoolVarP(&flags.Verbose, "verbose", "v", false, "Verbose output")
	cmd.Flags().StringVar(&flags.APIKey, "api-key", "", "API key for the provider")
	cmd.Flags().StringVar(&flags.BaseURL, "base-url", "", "Base URL for the provider API")
	cmd.Flags().StringVar(&flags.Provider, "provider", "anthropic", "AI provider (anthropic, zai, synthetic)")
}
