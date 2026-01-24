package agent

// Option is a functional option for configuring builders.
type Option func(*Builder)

// WithModel sets the AI model.
func WithModel(m string) Option {
	return func(b *Builder) {
		b.Model(m)
	}
}

// WithSystem sets the system prompt.
func WithSystem(prompt string) Option {
	return func(b *Builder) {
		b.System(prompt)
	}
}

// WithMaxTurns limits conversation turns.
func WithMaxTurns(n int) Option {
	return func(b *Builder) {
		b.MaxTurns(n)
	}
}

// WithBudget sets a spending limit.
func WithBudget(usd float64) Option {
	return func(b *Builder) {
		b.Budget(usd)
	}
}

// WithTool adds a tool.
func WithTool(t ToolDef) Option {
	return func(b *Builder) {
		b.Tool(t)
	}
}

// WithWorkDir sets the working directory.
func WithWorkDir(path string) Option {
	return func(b *Builder) {
		b.WorkDir(path)
	}
}

// WithContext adds context files.
func WithContext(files ...string) Option {
	return func(b *Builder) {
		b.Context(files...)
	}
}

// WithEnv sets environment variables.
func WithEnv(vars map[string]string) Option {
	return func(b *Builder) {
		b.Env(vars)
	}
}
