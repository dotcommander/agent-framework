package agent

import (
	"context"

	"github.com/dotcommander/agent-framework/tools"
)

// stateKey is the context key for session state.
type stateKey struct{}

// State retrieves typed session state from context.
// Returns nil if no state was set or type doesn't match.
//
// Example:
//
//	type Session struct {
//	    TargetFile string
//	}
//
//	findTool := agent.Tool("find", "Find file", func(ctx context.Context, in FindInput) (string, error) {
//	    state := agent.State[Session](ctx)
//	    state.TargetFile = in.Path
//	    return "Found", nil
//	})
func State[T any](ctx context.Context) *T {
	v := ctx.Value(stateKey{})
	if v == nil {
		return nil
	}
	if state, ok := v.(*T); ok {
		return state
	}
	return nil
}

// WithState sets session state that tools can access via State[T](ctx).
// The state pointer is shared across all tool invocations in a session.
//
// Example:
//
//	type Session struct {
//	    Files []string
//	}
//
//	session := &Session{}
//	agent.Run("Help me find files",
//	    agent.WithState(session),
//	    agent.WithTool(findTool),
//	)
//	// After run, session.Files contains found files
func WithState[T any](state *T) Option {
	return func(b *Builder) {
		b.state = state
	}
}

// wrapToolWithState wraps a tool's handler to inject state into context.
func wrapToolWithState(tool *tools.Tool, state any) *tools.Tool {
	if state == nil {
		return tool
	}

	originalHandler := tool.Handler
	return &tools.Tool{
		Name:        tool.Name,
		Description: tool.Description,
		InputSchema: tool.InputSchema,
		Handler: func(ctx context.Context, input map[string]any) (any, error) {
			ctx = context.WithValue(ctx, stateKey{}, state)
			return originalHandler(ctx, input)
		},
	}
}
