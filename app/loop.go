package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Loop configuration defaults.
const (
	// DefaultMaxIterations is the maximum agent loop iterations before stopping.
	DefaultMaxIterations = 50

	// DefaultMaxTokens is the token budget for a single agent run.
	DefaultMaxTokens = 100000

	// DefaultLoopTimeout is the maximum duration for the entire loop.
	DefaultLoopTimeout = 30 * time.Minute

	// DefaultShutdownTimeout is the maximum time to wait for shutdown hooks.
	DefaultShutdownTimeout = 30 * time.Second
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

	// StuckDetection enables automatic stuck pattern detection.
	// If nil, stuck detection is disabled.
	StuckDetection *StuckConfig

	// Hooks for extensibility
	OnIterationStart func(state *LoopState)
	OnIterationEnd   func(state *LoopState)
	OnError          func(err error, state *LoopState)

	// OnStuckPattern is called when a stuck pattern is detected.
	// Return true to continue the loop, false to stop.
	// If nil and a pattern is detected, the loop stops with an error.
	OnStuckPattern func(pattern *StuckPattern, state *LoopState) bool
}

// DefaultLoopConfig returns sensible defaults.
func DefaultLoopConfig() *LoopConfig {
	return &LoopConfig{
		MaxIterations:   DefaultMaxIterations,
		MaxTokens:       DefaultMaxTokens,
		Timeout:         DefaultLoopTimeout,
		ShutdownTimeout: DefaultShutdownTimeout,
		StopOnError:     false,
		MinScore:        0.0,
	}
}

// LoopRunner executes an agent loop.
type LoopRunner struct {
	loop          AgentLoop
	config        *LoopConfig
	stuckDetector *StuckDetector

	mu            sync.Mutex
	shutdownHooks []ShutdownHook
}

// NewLoopRunner creates a new loop runner.
func NewLoopRunner(loop AgentLoop, config *LoopConfig) *LoopRunner {
	if config == nil {
		config = DefaultLoopConfig()
	}
	r := &LoopRunner{
		loop:   loop,
		config: config,
	}
	// Initialize stuck detector if configured
	if config.StuckDetection != nil {
		r.stuckDetector = NewStuckDetector(config.StuckDetection)
	}
	return r
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
		timeout = DefaultShutdownTimeout
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

// handleStepError invokes the OnError hook and checks StopOnError policy.
// If StopOnError is true, returns a wrapped error; otherwise returns nil.
func (r *LoopRunner) handleStepError(err error, state *LoopState, phase string) error {
	if r.config.OnError != nil {
		r.config.OnError(err, state)
	}
	if r.config.StopOnError {
		return fmt.Errorf("%s: %w", phase, err)
	}
	return nil
}

// checkLimits checks context cancellation and iteration limits before each iteration.
func (r *LoopRunner) checkLimits(ctx context.Context, state *LoopState) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("loop cancelled: %w", err)
	}
	if r.config.MaxIterations > 0 && state.Iteration >= r.config.MaxIterations {
		return fmt.Errorf("max iterations (%d) reached", r.config.MaxIterations)
	}
	return nil
}

// runStuckDetection records verification feedback and checks for stuck patterns.
// Returns an error if a stuck pattern is detected and the loop should stop.
func (r *LoopRunner) runStuckDetection(feedback *Feedback, state *LoopState) error {
	if r.stuckDetector == nil {
		return nil
	}

	var stuckPattern *StuckPattern
	const taskID = "loop"

	if feedback != nil && !feedback.Valid {
		errMsg := "verification failed"
		if len(feedback.Issues) > 0 {
			errMsg = feedback.Issues[0]
		}
		stuckPattern = r.stuckDetector.RecordError(taskID, errMsg)
	} else if feedback != nil && feedback.Valid {
		stuckPattern = r.stuckDetector.RecordResult(taskID, true)
	}

	if stuckPattern != nil {
		if r.config.OnStuckPattern != nil {
			if !r.config.OnStuckPattern(stuckPattern, state) {
				return fmt.Errorf("stuck pattern detected: %s", stuckPattern.Message)
			}
		} else {
			return fmt.Errorf("stuck pattern detected: %s", stuckPattern.Message)
		}
	}

	return nil
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
		// Pre-iteration limit checks
		if err := r.checkLimits(ctx, state); err != nil {
			return state, err
		}

		state.Iteration++

		// Hook: iteration start
		if r.config.OnIterationStart != nil {
			r.config.OnIterationStart(state)
		}

		// Step 1: Gather context
		loopCtx, err := r.loop.GatherContext(ctx, state)
		if err != nil {
			if stepErr := r.handleStepError(err, state, "gather context"); stepErr != nil {
				return state, stepErr
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
			if stepErr := r.handleStepError(err, state, "decide action"); stepErr != nil {
				return state, stepErr
			}
		}
		state.LastAction = action

		// Step 3: Take action (skip if no action from decide phase)
		var result *Result
		if action != nil {
			result, err = r.loop.TakeAction(ctx, action)
			if err != nil {
				if stepErr := r.handleStepError(err, state, "take action"); stepErr != nil {
					return state, stepErr
				}
				result = &Result{Success: false, Error: err}
			}
		}
		state.LastResult = result

		// Step 4: Verify
		feedback, err := r.loop.Verify(ctx, state)
		if err != nil {
			if stepErr := r.handleStepError(err, state, "verify"); stepErr != nil {
				return state, stepErr
			}
		}
		state.LastVerify = feedback

		// Step 4b: Check for stuck patterns
		if err := r.runStuckDetection(feedback, state); err != nil {
			return state, err
		}

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
