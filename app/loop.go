package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ShutdownHook is a function called during graceful shutdown.
// The context has the shutdown timeout applied.
type ShutdownHook func(ctx context.Context) error

// LoopState represents the current state of an agent loop iteration.
type LoopState struct {
	Iteration   int
	Context     *LoopContext
	LastAction  *Action
	LastResult  *Result
	LastVerify  *Feedback
	StartedAt   time.Time
	CompletedAt time.Time
}

// LoopContext holds gathered context for an iteration.
type LoopContext struct {
	Messages   []Message
	Tools      []ToolInfo
	State      map[string]any
	TokenCount int
}

// Message represents a conversation message.
type Message struct {
	Role    string
	Content string
}

// ToolInfo describes an available tool.
type ToolInfo struct {
	Name        string
	Description string
}

// Action represents an action to take.
type Action struct {
	Type         string         // "tool_call", "response", "delegate"
	ToolName     string         // For tool calls
	ToolInput    map[string]any // Tool arguments
	Response     string         // For direct responses
	Subagent     string         // For delegation
	SubagentTask string
}

// Result represents the outcome of an action.
type Result struct {
	Success bool
	Output  any
	Error   error
	Tokens  int
}

// Feedback represents verification feedback.
type Feedback struct {
	Valid    bool
	Issues   []string
	Warnings []string
	Score    float64 // 0.0 to 1.0
}

// AgentLoop defines the core agent loop interface.
// Pattern: gather context → take action → verify → repeat
type AgentLoop interface {
	// GatherContext collects relevant context for the current iteration.
	GatherContext(ctx context.Context, state *LoopState) (*LoopContext, error)

	// DecideAction determines what action to take based on context.
	DecideAction(ctx context.Context, state *LoopState) (*Action, error)

	// TakeAction executes the decided action.
	TakeAction(ctx context.Context, action *Action) (*Result, error)

	// Verify validates the result and provides feedback.
	Verify(ctx context.Context, state *LoopState) (*Feedback, error)

	// ShouldContinue determines if the loop should continue.
	ShouldContinue(state *LoopState) bool
}

// LoopConfig configures the agent loop behavior.
type LoopConfig struct {
	// MaxIterations limits loop cycles (safety). 0 = unlimited.
	MaxIterations int

	// MaxTokens limits total tokens before compaction.
	MaxTokens int

	// Timeout for the entire loop.
	Timeout time.Duration

	// ShutdownTimeout is the maximum time to wait for shutdown hooks.
	// Default: 30 seconds.
	ShutdownTimeout time.Duration

	// StopOnError halts the loop on first error.
	StopOnError bool

	// MinScore minimum verification score to continue.
	MinScore float64

	// Hooks for extensibility
	OnIterationStart func(state *LoopState)
	OnIterationEnd   func(state *LoopState)
	OnError          func(err error, state *LoopState)
}

// DefaultLoopConfig returns sensible defaults.
func DefaultLoopConfig() *LoopConfig {
	return &LoopConfig{
		MaxIterations:   50,
		MaxTokens:       100000,
		Timeout:         30 * time.Minute,
		ShutdownTimeout: 30 * time.Second,
		StopOnError:     false,
		MinScore:        0.0,
	}
}

// LoopRunner executes an agent loop.
type LoopRunner struct {
	loop   AgentLoop
	config *LoopConfig

	mu            sync.Mutex
	shutdownHooks []ShutdownHook
}

// NewLoopRunner creates a new loop runner.
func NewLoopRunner(loop AgentLoop, config *LoopConfig) *LoopRunner {
	if config == nil {
		config = DefaultLoopConfig()
	}
	return &LoopRunner{
		loop:   loop,
		config: config,
	}
}

// OnShutdown registers a hook to be called during graceful shutdown.
// Hooks are called in reverse registration order (LIFO).
// The hook receives a context with the shutdown timeout applied.
func (r *LoopRunner) OnShutdown(hook ShutdownHook) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.shutdownHooks = append(r.shutdownHooks, hook)
}

// runShutdownHooks executes all registered shutdown hooks in LIFO order.
// Returns the first error encountered, but continues executing all hooks.
func (r *LoopRunner) runShutdownHooks() error {
	r.mu.Lock()
	hooks := make([]ShutdownHook, len(r.shutdownHooks))
	copy(hooks, r.shutdownHooks)
	r.mu.Unlock()

	if len(hooks) == 0 {
		return nil
	}

	timeout := r.config.ShutdownTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var firstErr error
	// Execute in reverse order (LIFO)
	for i := len(hooks) - 1; i >= 0; i-- {
		if err := hooks[i](ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}

// Run executes the agent loop until completion or limit.
// Shutdown hooks are called when the loop exits (for any reason).
func (r *LoopRunner) Run(ctx context.Context) (state *LoopState, err error) {
	// Always run shutdown hooks on exit
	defer func() {
		if shutdownErr := r.runShutdownHooks(); shutdownErr != nil {
			// Use errors.Join for proper multi-error handling (Go 1.20+)
			if err != nil {
				err = errors.Join(err, fmt.Errorf("shutdown: %w", shutdownErr))
			} else {
				err = fmt.Errorf("shutdown: %w", shutdownErr)
			}
		}
	}()

	// Apply timeout if configured
	if r.config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.config.Timeout)
		defer cancel()
	}

	state = &LoopState{
		Iteration: 0,
		StartedAt: time.Now(),
	}

	for {
		// Check context cancellation
		if err := ctx.Err(); err != nil {
			return state, fmt.Errorf("loop cancelled: %w", err)
		}

		// Check iteration limit
		if r.config.MaxIterations > 0 && state.Iteration >= r.config.MaxIterations {
			return state, fmt.Errorf("max iterations (%d) reached", r.config.MaxIterations)
		}

		state.Iteration++

		// Hook: iteration start
		if r.config.OnIterationStart != nil {
			r.config.OnIterationStart(state)
		}

		// Step 1: Gather context
		loopCtx, err := r.loop.GatherContext(ctx, state)
		if err != nil {
			if r.config.OnError != nil {
				r.config.OnError(err, state)
			}
			if r.config.StopOnError {
				return state, fmt.Errorf("gather context: %w", err)
			}
		}
		state.Context = loopCtx

		// Check token limit
		if loopCtx != nil && r.config.MaxTokens > 0 && loopCtx.TokenCount > r.config.MaxTokens {
			return state, fmt.Errorf("token limit (%d) exceeded: %d", r.config.MaxTokens, loopCtx.TokenCount)
		}

		// Step 2: Decide action
		action, err := r.loop.DecideAction(ctx, state)
		if err != nil {
			if r.config.OnError != nil {
				r.config.OnError(err, state)
			}
			if r.config.StopOnError {
				return state, fmt.Errorf("decide action: %w", err)
			}
		}
		state.LastAction = action

		// Step 3: Take action (skip if no action from decide phase)
		var result *Result
		if action != nil {
			result, err = r.loop.TakeAction(ctx, action)
			if err != nil {
				if r.config.OnError != nil {
					r.config.OnError(err, state)
				}
				if r.config.StopOnError {
					return state, fmt.Errorf("take action: %w", err)
				}
				result = &Result{Success: false, Error: err}
			}
		}
		state.LastResult = result

		// Step 4: Verify
		feedback, err := r.loop.Verify(ctx, state)
		if err != nil {
			if r.config.OnError != nil {
				r.config.OnError(err, state)
			}
			if r.config.StopOnError {
				return state, fmt.Errorf("verify: %w", err)
			}
		}
		state.LastVerify = feedback

		// Hook: iteration end
		if r.config.OnIterationEnd != nil {
			r.config.OnIterationEnd(state)
		}

		// Check minimum score
		if feedback != nil && r.config.MinScore > 0 && feedback.Score < r.config.MinScore {
			return state, fmt.Errorf("verification score %.2f below minimum %.2f", feedback.Score, r.config.MinScore)
		}

		// Step 5: Should continue?
		if !r.loop.ShouldContinue(state) {
			state.CompletedAt = time.Now()
			return state, nil
		}
	}
}

// SimpleLoop provides a basic AgentLoop implementation.
type SimpleLoop struct {
	gatherFn   func(ctx context.Context, state *LoopState) (*LoopContext, error)
	decideFn   func(ctx context.Context, state *LoopState) (*Action, error)
	actionFn   func(ctx context.Context, action *Action) (*Result, error)
	verifyFn   func(ctx context.Context, state *LoopState) (*Feedback, error)
	continueFn func(state *LoopState) bool
}

// SimpleLoopOption configures a SimpleLoop.
type SimpleLoopOption func(*SimpleLoop)

// WithGatherFunc sets the context gathering function.
func WithGatherFunc(fn func(ctx context.Context, state *LoopState) (*LoopContext, error)) SimpleLoopOption {
	return func(l *SimpleLoop) {
		l.gatherFn = fn
	}
}

// WithDecideFunc sets the action decision function.
func WithDecideFunc(fn func(ctx context.Context, state *LoopState) (*Action, error)) SimpleLoopOption {
	return func(l *SimpleLoop) {
		l.decideFn = fn
	}
}

// WithActionFunc sets the action execution function.
func WithActionFunc(fn func(ctx context.Context, action *Action) (*Result, error)) SimpleLoopOption {
	return func(l *SimpleLoop) {
		l.actionFn = fn
	}
}

// WithVerifyFunc sets the verification function.
func WithVerifyFunc(fn func(ctx context.Context, state *LoopState) (*Feedback, error)) SimpleLoopOption {
	return func(l *SimpleLoop) {
		l.verifyFn = fn
	}
}

// WithContinueFunc sets the continuation check function.
func WithContinueFunc(fn func(state *LoopState) bool) SimpleLoopOption {
	return func(l *SimpleLoop) {
		l.continueFn = fn
	}
}

// NewSimpleLoop creates a configurable simple loop.
func NewSimpleLoop(opts ...SimpleLoopOption) *SimpleLoop {
	l := &SimpleLoop{
		// Defaults
		gatherFn: func(ctx context.Context, state *LoopState) (*LoopContext, error) {
			return &LoopContext{State: make(map[string]any)}, nil
		},
		decideFn: func(ctx context.Context, state *LoopState) (*Action, error) {
			return &Action{Type: "response", Response: "No action configured"}, nil
		},
		actionFn: func(ctx context.Context, action *Action) (*Result, error) {
			return &Result{Success: true, Output: action.Response}, nil
		},
		verifyFn: func(ctx context.Context, state *LoopState) (*Feedback, error) {
			return &Feedback{Valid: true, Score: 1.0}, nil
		},
		continueFn: func(state *LoopState) bool {
			return state.Iteration < 1 // Default: single iteration
		},
	}

	for _, opt := range opts {
		opt(l)
	}

	return l
}

// GatherContext implements AgentLoop.
func (l *SimpleLoop) GatherContext(ctx context.Context, state *LoopState) (*LoopContext, error) {
	return l.gatherFn(ctx, state)
}

// DecideAction implements AgentLoop.
func (l *SimpleLoop) DecideAction(ctx context.Context, state *LoopState) (*Action, error) {
	return l.decideFn(ctx, state)
}

// TakeAction implements AgentLoop.
func (l *SimpleLoop) TakeAction(ctx context.Context, action *Action) (*Result, error) {
	return l.actionFn(ctx, action)
}

// Verify implements AgentLoop.
func (l *SimpleLoop) Verify(ctx context.Context, state *LoopState) (*Feedback, error) {
	return l.verifyFn(ctx, state)
}

// ShouldContinue implements AgentLoop.
func (l *SimpleLoop) ShouldContinue(state *LoopState) bool {
	return l.continueFn(state)
}
