package cli

import (
	"github.com/spf13/cobra"
)

// RunFunc is the function signature for command execution.
type RunFunc func(cmd *cobra.Command, args []string) error

// CommandBuilder helps build cobra commands with standard patterns.
type CommandBuilder struct {
	name  string
	short string
	long  string
	run   RunFunc
	flags *StandardFlags
}

// NewCommandBuilder creates a new command builder.
func NewCommandBuilder(name, short string) *CommandBuilder {
	return &CommandBuilder{
		name:  name,
		short: short,
		flags: &StandardFlags{},
	}
}

// WithLong sets the long description.
func (b *CommandBuilder) WithLong(long string) *CommandBuilder {
	b.long = long
	return b
}

// WithRun sets the run function.
func (b *CommandBuilder) WithRun(run RunFunc) *CommandBuilder {
	b.run = run
	return b
}

// Build creates the cobra command.
func (b *CommandBuilder) Build() *cobra.Command {
	cmd := &cobra.Command{
		Use:   b.name,
		Short: b.short,
		Long:  b.long,
		RunE:  b.run,
	}

	// Add standard flags
	AddStandardFlags(cmd, b.flags)

	return cmd
}

// GetFlags returns the standard flags.
func (b *CommandBuilder) GetFlags() *StandardFlags {
	return b.flags
}

// NewCommand creates a cobra command with standard flags.
func NewCommand(name, short string, runFunc RunFunc) *cobra.Command {
	return NewCommandBuilder(name, short).
		WithRun(runFunc).
		Build()
}
