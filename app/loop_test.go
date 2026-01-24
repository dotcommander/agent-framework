package app

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDefaultLoopConfig tests default configuration values.
func TestDefaultLoopConfig(t *testing.T) {
	t.Parallel()

	cfg := DefaultLoopConfig()
	assert.Equal(t, 50, cfg.MaxIterations)
	assert.Equal(t, 100000, cfg.MaxTokens)
	assert.Equal(t, 30*time.Minute, cfg.Timeout)
	assert.Equal(t, 30*time.Second, cfg.ShutdownTimeout)
	assert.False(t, cfg.StopOnError)
	assert.Equal(t, 0.0, cfg.MinScore)
}

// TestLoopRunnerNilConfig tests that nil config uses defaults.
func TestLoopRunnerNilConfig(t *testing.T) {
	t.Parallel()

	loop := NewSimpleLoop()
	runner := NewLoopRunner(loop, nil)
	assert.NotNil(t, runner.config)
	assert.Equal(t, 50, runner.config.MaxIterations)
}

// TestLoopRunnerBasicExecution tests basic loop execution.
func TestLoopRunnerBasicExecution(t *testing.T) {
	t.Parallel()

	iterations := 0
	loop := NewSimpleLoop(
		WithGatherFunc(func(ctx context.Context, state *LoopState) (*LoopContext, error) {
			iterations++
			return &LoopContext{State: make(map[string]any), TokenCount: 100}, nil
		}),
		WithContinueFunc(func(state *LoopState) bool {
			return state.Iteration < 3
		}),
	)

	runner := NewLoopRunner(loop, &LoopConfig{
		MaxIterations: 10,
	})

	state, err := runner.Run(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 3, state.Iteration)
	assert.Equal(t, 3, iterations)
	assert.NotZero(t, state.CompletedAt)
}

// TestLoopRunnerMaxIterations tests iteration limit enforcement.
func TestLoopRunnerMaxIterations(t *testing.T) {
	t.Parallel()

	loop := NewSimpleLoop(
		WithContinueFunc(func(state *LoopState) bool {
			return true // Always continue
		}),
	)

	runner := NewLoopRunner(loop, &LoopConfig{
		MaxIterations: 5,
	})

	state, err := runner.Run(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max iterations (5) reached")
	assert.Equal(t, 5, state.Iteration)
}

// TestLoopRunnerMaxTokens tests token limit enforcement.
func TestLoopRunnerMaxTokens(t *testing.T) {
	t.Parallel()

	loop := NewSimpleLoop(
		WithGatherFunc(func(ctx context.Context, state *LoopState) (*LoopContext, error) {
			return &LoopContext{TokenCount: 50000}, nil
		}),
		WithContinueFunc(func(state *LoopState) bool {
			return true
		}),
	)

	runner := NewLoopRunner(loop, &LoopConfig{
		MaxIterations: 10,
		MaxTokens:     40000,
	})

	state, err := runner.Run(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token limit (40000) exceeded")
	assert.Equal(t, 1, state.Iteration)
}

// TestLoopRunnerTimeout tests timeout enforcement.
func TestLoopRunnerTimeout(t *testing.T) {
	t.Parallel()

	loop := NewSimpleLoop(
		WithGatherFunc(func(ctx context.Context, state *LoopState) (*LoopContext, error) {
			time.Sleep(100 * time.Millisecond)
			return &LoopContext{}, nil
		}),
		WithContinueFunc(func(state *LoopState) bool {
			return true
		}),
	)

	runner := NewLoopRunner(loop, &LoopConfig{
		Timeout:       200 * time.Millisecond,
		MaxIterations: 100,
	})

	_, err := runner.Run(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loop cancelled")
}

// TestLoopRunnerContextCancellation tests context cancellation.
func TestLoopRunnerContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	loop := NewSimpleLoop(
		WithGatherFunc(func(ctx context.Context, state *LoopState) (*LoopContext, error) {
			if state.Iteration == 2 {
				cancel()
			}
			return &LoopContext{}, nil
		}),
		WithContinueFunc(func(state *LoopState) bool {
			return true
		}),
	)

	runner := NewLoopRunner(loop, &LoopConfig{MaxIterations: 10})

	_, err := runner.Run(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loop cancelled")
}

// TestLoopRunnerMinScore tests minimum score enforcement.
func TestLoopRunnerMinScore(t *testing.T) {
	t.Parallel()

	loop := NewSimpleLoop(
		WithVerifyFunc(func(ctx context.Context, state *LoopState) (*Feedback, error) {
			return &Feedback{Valid: false, Score: 0.5}, nil
		}),
		WithContinueFunc(func(state *LoopState) bool {
			return true
		}),
	)

	runner := NewLoopRunner(loop, &LoopConfig{
		MinScore:      0.8,
		MaxIterations: 10,
	})

	state, err := runner.Run(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "verification score 0.50 below minimum 0.80")
	assert.Equal(t, 1, state.Iteration)
}

// TestGatherContextError tests error handling in GatherContext phase.
func TestGatherContextError(t *testing.T) {
	t.Parallel()

	gatherErr := errors.New("gather failed")
	errorCalled := false

	loop := NewSimpleLoop(
		WithGatherFunc(func(ctx context.Context, state *LoopState) (*LoopContext, error) {
			return nil, gatherErr
		}),
	)

	runner := NewLoopRunner(loop, &LoopConfig{
		StopOnError: true,
		OnError: func(err error, state *LoopState) {
			errorCalled = true
			assert.Equal(t, gatherErr, err)
		},
	})

	_, err := runner.Run(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gather context")
	assert.True(t, errorCalled)
}

// TestGatherContextErrorContinue tests continuing after GatherContext error.
func TestGatherContextErrorContinue(t *testing.T) {
	t.Parallel()

	errorCount := 0
	loop := NewSimpleLoop(
		WithGatherFunc(func(ctx context.Context, state *LoopState) (*LoopContext, error) {
			if state.Iteration <= 2 {
				return nil, errors.New("gather error")
			}
			return &LoopContext{}, nil
		}),
		WithContinueFunc(func(state *LoopState) bool {
			return state.Iteration < 3
		}),
	)

	runner := NewLoopRunner(loop, &LoopConfig{
		StopOnError: false,
		OnError: func(err error, state *LoopState) {
			errorCount++
		},
	})

	state, err := runner.Run(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 3, state.Iteration)
	assert.Equal(t, 2, errorCount) // Errors in iterations 1 and 2
}

// TestDecideActionError tests error handling in DecideAction phase.
func TestDecideActionError(t *testing.T) {
	t.Parallel()

	decideErr := errors.New("decide failed")
	errorCalled := false

	loop := NewSimpleLoop(
		WithDecideFunc(func(ctx context.Context, state *LoopState) (*Action, error) {
			return nil, decideErr
		}),
	)

	runner := NewLoopRunner(loop, &LoopConfig{
		StopOnError: true,
		OnError: func(err error, state *LoopState) {
			errorCalled = true
			assert.Equal(t, decideErr, err)
		},
	})

	_, err := runner.Run(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decide action")
	assert.True(t, errorCalled)
}

// TestDecideActionErrorContinue tests continuing after DecideAction error.
func TestDecideActionErrorContinue(t *testing.T) {
	t.Parallel()

	errorCount := 0
	loop := NewSimpleLoop(
		WithDecideFunc(func(ctx context.Context, state *LoopState) (*Action, error) {
			return nil, errors.New("decide error")
		}),
		WithContinueFunc(func(state *LoopState) bool {
			return state.Iteration < 2
		}),
	)

	runner := NewLoopRunner(loop, &LoopConfig{
		StopOnError: false,
		OnError: func(err error, state *LoopState) {
			errorCount++
		},
	})

	state, err := runner.Run(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, state.Iteration)
	assert.Equal(t, 2, errorCount)
}

// TestTakeActionError tests error handling in TakeAction phase.
func TestTakeActionError(t *testing.T) {
	t.Parallel()

	actionErr := errors.New("action failed")
	errorCalled := false

	loop := NewSimpleLoop(
		WithActionFunc(func(ctx context.Context, action *Action) (*Result, error) {
			return nil, actionErr
		}),
	)

	runner := NewLoopRunner(loop, &LoopConfig{
		StopOnError: true,
		OnError: func(err error, state *LoopState) {
			errorCalled = true
			assert.Equal(t, actionErr, err)
		},
	})

	_, err := runner.Run(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "take action")
	assert.True(t, errorCalled)
}

// TestTakeActionErrorContinue tests continuing after TakeAction error.
func TestTakeActionErrorContinue(t *testing.T) {
	t.Parallel()

	actionErr := errors.New("action failed")
	errorCount := 0

	loop := NewSimpleLoop(
		WithActionFunc(func(ctx context.Context, action *Action) (*Result, error) {
			return nil, actionErr
		}),
		WithContinueFunc(func(state *LoopState) bool {
			return state.Iteration < 2
		}),
	)

	runner := NewLoopRunner(loop, &LoopConfig{
		StopOnError: false,
		OnError: func(err error, state *LoopState) {
			errorCount++
		},
	})

	state, err := runner.Run(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, state.Iteration)
	assert.Equal(t, 2, errorCount)
	// Verify that result is created with error
	assert.NotNil(t, state.LastResult)
	assert.False(t, state.LastResult.Success)
	assert.Equal(t, actionErr, state.LastResult.Error)
}

// TestVerifyError tests error handling in Verify phase.
func TestVerifyError(t *testing.T) {
	t.Parallel()

	verifyErr := errors.New("verify failed")
	errorCalled := false

	loop := NewSimpleLoop(
		WithVerifyFunc(func(ctx context.Context, state *LoopState) (*Feedback, error) {
			return nil, verifyErr
		}),
	)

	runner := NewLoopRunner(loop, &LoopConfig{
		StopOnError: true,
		OnError: func(err error, state *LoopState) {
			errorCalled = true
			assert.Equal(t, verifyErr, err)
		},
	})

	_, err := runner.Run(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "verify")
	assert.True(t, errorCalled)
}

// TestVerifyErrorContinue tests continuing after Verify error.
func TestVerifyErrorContinue(t *testing.T) {
	t.Parallel()

	errorCount := 0
	loop := NewSimpleLoop(
		WithVerifyFunc(func(ctx context.Context, state *LoopState) (*Feedback, error) {
			return nil, errors.New("verify error")
		}),
		WithContinueFunc(func(state *LoopState) bool {
			return state.Iteration < 2
		}),
	)

	runner := NewLoopRunner(loop, &LoopConfig{
		StopOnError: false,
		OnError: func(err error, state *LoopState) {
			errorCount++
		},
	})

	state, err := runner.Run(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, state.Iteration)
	assert.Equal(t, 2, errorCount)
}

// TestIterationHooks tests OnIterationStart and OnIterationEnd hooks.
func TestIterationHooks(t *testing.T) {
	t.Parallel()

	var startIterations []int
	var endIterations []int

	loop := NewSimpleLoop(
		WithContinueFunc(func(state *LoopState) bool {
			return state.Iteration < 3
		}),
	)

	runner := NewLoopRunner(loop, &LoopConfig{
		MaxIterations: 10,
		OnIterationStart: func(state *LoopState) {
			startIterations = append(startIterations, state.Iteration)
		},
		OnIterationEnd: func(state *LoopState) {
			endIterations = append(endIterations, state.Iteration)
		},
	})

	_, err := runner.Run(context.Background())
	require.NoError(t, err)

	assert.Equal(t, []int{1, 2, 3}, startIterations)
	assert.Equal(t, []int{1, 2, 3}, endIterations)
}

// TestStateUpdates tests that LoopState is updated correctly.
func TestStateUpdates(t *testing.T) {
	t.Parallel()

	var capturedStates []*LoopState

	loop := NewSimpleLoop(
		WithGatherFunc(func(ctx context.Context, state *LoopState) (*LoopContext, error) {
			return &LoopContext{
				Messages:   []Message{{Role: "user", Content: "test"}},
				TokenCount: state.Iteration * 100,
			}, nil
		}),
		WithDecideFunc(func(ctx context.Context, state *LoopState) (*Action, error) {
			return &Action{Type: "response", Response: "test response"}, nil
		}),
		WithActionFunc(func(ctx context.Context, action *Action) (*Result, error) {
			return &Result{Success: true, Output: "result", Tokens: 50}, nil
		}),
		WithVerifyFunc(func(ctx context.Context, state *LoopState) (*Feedback, error) {
			return &Feedback{Valid: true, Score: 0.95}, nil
		}),
		WithContinueFunc(func(state *LoopState) bool {
			// Capture state at end of iteration
			stateCopy := *state
			capturedStates = append(capturedStates, &stateCopy)
			return state.Iteration < 2
		}),
	)

	runner := NewLoopRunner(loop, &LoopConfig{MaxIterations: 10})

	finalState, err := runner.Run(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, finalState.Iteration)

	// Verify state updates
	require.Len(t, capturedStates, 2)

	// Check first iteration
	state1 := capturedStates[0]
	assert.Equal(t, 1, state1.Iteration)
	assert.NotNil(t, state1.Context)
	assert.Equal(t, 100, state1.Context.TokenCount)
	assert.NotNil(t, state1.LastAction)
	assert.Equal(t, "response", state1.LastAction.Type)
	assert.NotNil(t, state1.LastResult)
	assert.True(t, state1.LastResult.Success)
	assert.NotNil(t, state1.LastVerify)
	assert.Equal(t, 0.95, state1.LastVerify.Score)

	// Check second iteration
	state2 := capturedStates[1]
	assert.Equal(t, 2, state2.Iteration)
	assert.Equal(t, 200, state2.Context.TokenCount)
}

// TestShutdownHooks tests shutdown hook registration and execution.
func TestShutdownHooks(t *testing.T) {
	t.Parallel()

	var executed []int

	loop := NewSimpleLoop()
	runner := NewLoopRunner(loop, &LoopConfig{})

	// Register hooks in order
	runner.OnShutdown(func(ctx context.Context) error {
		executed = append(executed, 1)
		return nil
	})
	runner.OnShutdown(func(ctx context.Context) error {
		executed = append(executed, 2)
		return nil
	})
	runner.OnShutdown(func(ctx context.Context) error {
		executed = append(executed, 3)
		return nil
	})

	_, err := runner.Run(context.Background())
	require.NoError(t, err)

	// Verify LIFO order (last registered runs first)
	assert.Equal(t, []int{3, 2, 1}, executed)
}

// TestShutdownHooksExecuteOnError tests that hooks run even when loop errors.
func TestShutdownHooksExecuteOnError(t *testing.T) {
	t.Parallel()

	hookExecuted := false

	loop := NewSimpleLoop(
		WithGatherFunc(func(ctx context.Context, state *LoopState) (*LoopContext, error) {
			return nil, errors.New("gather failed")
		}),
	)

	runner := NewLoopRunner(loop, &LoopConfig{StopOnError: true})
	runner.OnShutdown(func(ctx context.Context) error {
		hookExecuted = true
		return nil
	})

	_, err := runner.Run(context.Background())
	require.Error(t, err)
	assert.True(t, hookExecuted, "Shutdown hook should execute even on error")
}

// TestShutdownHooksLIFOOrder tests LIFO execution order in detail.
func TestShutdownHooksLIFOOrder(t *testing.T) {
	t.Parallel()

	order := make([]string, 0, 3)

	loop := NewSimpleLoop()
	runner := NewLoopRunner(loop, &LoopConfig{})

	runner.OnShutdown(func(ctx context.Context) error {
		order = append(order, "first")
		return nil
	})
	runner.OnShutdown(func(ctx context.Context) error {
		order = append(order, "second")
		return nil
	})
	runner.OnShutdown(func(ctx context.Context) error {
		order = append(order, "third")
		return nil
	})

	_, err := runner.Run(context.Background())
	require.NoError(t, err)

	// LIFO: third, second, first
	assert.Equal(t, []string{"third", "second", "first"}, order)
}

// TestShutdownHookError tests error handling from shutdown hooks.
func TestShutdownHookError(t *testing.T) {
	t.Parallel()

	hookErr := errors.New("shutdown hook failed")

	loop := NewSimpleLoop()
	runner := NewLoopRunner(loop, &LoopConfig{})

	runner.OnShutdown(func(ctx context.Context) error {
		return hookErr
	})

	_, err := runner.Run(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "shutdown:")
	assert.ErrorIs(t, err, hookErr)
}

// TestShutdownHookErrorWithLoopError tests both loop and shutdown errors.
func TestShutdownHookErrorWithLoopError(t *testing.T) {
	t.Parallel()

	loopErr := errors.New("loop failed")
	hookErr := errors.New("shutdown failed")

	loop := NewSimpleLoop(
		WithGatherFunc(func(ctx context.Context, state *LoopState) (*LoopContext, error) {
			return nil, loopErr
		}),
	)

	runner := NewLoopRunner(loop, &LoopConfig{StopOnError: true})
	runner.OnShutdown(func(ctx context.Context) error {
		return hookErr
	})

	_, err := runner.Run(context.Background())
	require.Error(t, err)
	// Should contain both errors (errors.Join preserves both)
	assert.Contains(t, err.Error(), "gather context")
	assert.Contains(t, err.Error(), "shutdown:")
}

// TestShutdownHookContinuesAfterError tests that all hooks run even if one fails.
func TestShutdownHookContinuesAfterError(t *testing.T) {
	t.Parallel()

	var executed []int

	loop := NewSimpleLoop()
	runner := NewLoopRunner(loop, &LoopConfig{})

	runner.OnShutdown(func(ctx context.Context) error {
		executed = append(executed, 1)
		return nil
	})
	runner.OnShutdown(func(ctx context.Context) error {
		executed = append(executed, 2)
		return errors.New("hook 2 failed")
	})
	runner.OnShutdown(func(ctx context.Context) error {
		executed = append(executed, 3)
		return nil
	})

	_, err := runner.Run(context.Background())
	require.Error(t, err)

	// All hooks should execute despite error
	assert.Equal(t, []int{3, 2, 1}, executed)
}

// TestShutdownHookTimeout tests shutdown timeout enforcement.
func TestShutdownHookTimeout(t *testing.T) {
	t.Parallel()

	loop := NewSimpleLoop()
	runner := NewLoopRunner(loop, &LoopConfig{
		ShutdownTimeout: 50 * time.Millisecond,
	})

	runner.OnShutdown(func(ctx context.Context) error {
		// Verify timeout is applied
		deadline, ok := ctx.Deadline()
		assert.True(t, ok, "Context should have deadline")
		assert.True(t, time.Until(deadline) <= 50*time.Millisecond)
		return nil
	})

	_, err := runner.Run(context.Background())
	require.NoError(t, err)
}

// TestShutdownHookDefaultTimeout tests default shutdown timeout.
func TestShutdownHookDefaultTimeout(t *testing.T) {
	t.Parallel()

	loop := NewSimpleLoop()
	runner := NewLoopRunner(loop, &LoopConfig{
		ShutdownTimeout: 0, // Zero should use default
	})

	runner.OnShutdown(func(ctx context.Context) error {
		deadline, ok := ctx.Deadline()
		assert.True(t, ok)
		// Should be close to 30 seconds (default)
		assert.True(t, time.Until(deadline) > 25*time.Second)
		assert.True(t, time.Until(deadline) <= 30*time.Second)
		return nil
	})

	_, err := runner.Run(context.Background())
	require.NoError(t, err)
}

// TestSimpleLoopDefaults tests default SimpleLoop behavior.
func TestSimpleLoopDefaults(t *testing.T) {
	t.Parallel()

	loop := NewSimpleLoop()

	// Test default GatherContext
	ctx, err := loop.GatherContext(context.Background(), &LoopState{})
	require.NoError(t, err)
	assert.NotNil(t, ctx)
	assert.NotNil(t, ctx.State)

	// Test default DecideAction
	action, err := loop.DecideAction(context.Background(), &LoopState{})
	require.NoError(t, err)
	assert.Equal(t, "response", action.Type)

	// Test default TakeAction
	result, err := loop.TakeAction(context.Background(), action)
	require.NoError(t, err)
	assert.True(t, result.Success)

	// Test default Verify
	feedback, err := loop.Verify(context.Background(), &LoopState{})
	require.NoError(t, err)
	assert.True(t, feedback.Valid)
	assert.Equal(t, 1.0, feedback.Score)

	// Test default ShouldContinue
	state := &LoopState{Iteration: 0}
	assert.True(t, loop.ShouldContinue(state))
	state.Iteration = 1
	assert.False(t, loop.ShouldContinue(state))
}

// TestSimpleLoopOptions tests functional options for SimpleLoop.
func TestSimpleLoopOptions(t *testing.T) {
	t.Parallel()

	customGatherCalled := false
	customDecideCalled := false
	customActionCalled := false
	customVerifyCalled := false
	customContinueCalled := false

	loop := NewSimpleLoop(
		WithGatherFunc(func(ctx context.Context, state *LoopState) (*LoopContext, error) {
			customGatherCalled = true
			return &LoopContext{}, nil
		}),
		WithDecideFunc(func(ctx context.Context, state *LoopState) (*Action, error) {
			customDecideCalled = true
			return &Action{Type: "custom"}, nil
		}),
		WithActionFunc(func(ctx context.Context, action *Action) (*Result, error) {
			customActionCalled = true
			return &Result{Success: true}, nil
		}),
		WithVerifyFunc(func(ctx context.Context, state *LoopState) (*Feedback, error) {
			customVerifyCalled = true
			return &Feedback{Valid: true, Score: 0.8}, nil
		}),
		WithContinueFunc(func(state *LoopState) bool {
			customContinueCalled = true
			return false
		}),
	)

	_, _ = loop.GatherContext(context.Background(), &LoopState{})
	assert.True(t, customGatherCalled)

	action, _ := loop.DecideAction(context.Background(), &LoopState{})
	assert.True(t, customDecideCalled)
	assert.Equal(t, "custom", action.Type)

	_, _ = loop.TakeAction(context.Background(), &Action{})
	assert.True(t, customActionCalled)

	feedback, _ := loop.Verify(context.Background(), &LoopState{})
	assert.True(t, customVerifyCalled)
	assert.Equal(t, 0.8, feedback.Score)

	_ = loop.ShouldContinue(&LoopState{})
	assert.True(t, customContinueCalled)
}

// TestLoopStateTimestamps tests that timestamps are set correctly.
func TestLoopStateTimestamps(t *testing.T) {
	t.Parallel()

	loop := NewSimpleLoop()
	runner := NewLoopRunner(loop, &LoopConfig{})

	before := time.Now()
	state, err := runner.Run(context.Background())
	after := time.Now()

	require.NoError(t, err)
	assert.True(t, state.StartedAt.After(before) || state.StartedAt.Equal(before))
	assert.True(t, state.StartedAt.Before(after))
	assert.True(t, state.CompletedAt.After(state.StartedAt) || state.CompletedAt.Equal(state.StartedAt))
	assert.True(t, state.CompletedAt.Before(after) || state.CompletedAt.Equal(after))
}

// TestErrorPhaseIdentification tests error messages identify the phase.
func TestErrorPhaseIdentification(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		setupLoop     func() *SimpleLoop
		expectedPhase string
	}{
		{
			name: "gather_phase",
			setupLoop: func() *SimpleLoop {
				return NewSimpleLoop(
					WithGatherFunc(func(ctx context.Context, state *LoopState) (*LoopContext, error) {
						return nil, errors.New("gather error")
					}),
				)
			},
			expectedPhase: "gather context",
		},
		{
			name: "decide_phase",
			setupLoop: func() *SimpleLoop {
				return NewSimpleLoop(
					WithDecideFunc(func(ctx context.Context, state *LoopState) (*Action, error) {
						return nil, errors.New("decide error")
					}),
				)
			},
			expectedPhase: "decide action",
		},
		{
			name: "action_phase",
			setupLoop: func() *SimpleLoop {
				return NewSimpleLoop(
					WithActionFunc(func(ctx context.Context, action *Action) (*Result, error) {
						return nil, errors.New("action error")
					}),
				)
			},
			expectedPhase: "take action",
		},
		{
			name: "verify_phase",
			setupLoop: func() *SimpleLoop {
				return NewSimpleLoop(
					WithVerifyFunc(func(ctx context.Context, state *LoopState) (*Feedback, error) {
						return nil, errors.New("verify error")
					}),
				)
			},
			expectedPhase: "verify",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loop := tt.setupLoop()
			runner := NewLoopRunner(loop, &LoopConfig{StopOnError: true})

			_, err := runner.Run(context.Background())
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedPhase)
		})
	}
}

// TestOnErrorReceivesCorrectState tests that OnError hook gets correct state.
func TestOnErrorReceivesCorrectState(t *testing.T) {
	t.Parallel()

	testErr := errors.New("test error")
	var capturedState *LoopState

	loop := NewSimpleLoop(
		WithDecideFunc(func(ctx context.Context, state *LoopState) (*Action, error) {
			if state.Iteration == 2 {
				return nil, testErr
			}
			return &Action{Type: "response"}, nil
		}),
		WithContinueFunc(func(state *LoopState) bool {
			return state.Iteration < 3
		}),
	)

	runner := NewLoopRunner(loop, &LoopConfig{
		StopOnError: false,
		OnError: func(err error, state *LoopState) {
			if err == testErr {
				stateCopy := *state
				capturedState = &stateCopy
			}
		},
	})

	_, err := runner.Run(context.Background())
	require.NoError(t, err)

	require.NotNil(t, capturedState, "OnError should have been called")
	assert.Equal(t, 2, capturedState.Iteration)
}

// TestNoShutdownHooks tests that no hooks is handled correctly.
func TestNoShutdownHooks(t *testing.T) {
	t.Parallel()

	loop := NewSimpleLoop()
	runner := NewLoopRunner(loop, &LoopConfig{})

	// Don't register any hooks
	state, err := runner.Run(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, state.Iteration)
}

// TestConcurrentOnShutdownRegistration tests thread-safe hook registration.
func TestConcurrentOnShutdownRegistration(t *testing.T) {
	t.Parallel()

	loop := NewSimpleLoop()
	runner := NewLoopRunner(loop, &LoopConfig{})

	// Register hooks concurrently
	done := make(chan bool)
	for range 10 {
		go func() {
			runner.OnShutdown(func(ctx context.Context) error {
				return nil
			})
			done <- true
		}()
	}

	for range 10 {
		<-done
	}

	// Should have 10 hooks registered
	runner.mu.Lock()
	hookCount := len(runner.shutdownHooks)
	runner.mu.Unlock()

	assert.Equal(t, 10, hookCount)
}

// TestLoopRunnerStuckDetectionRepeatError tests stuck detection for repeated errors.
func TestLoopRunnerStuckDetectionRepeatError(t *testing.T) {
	t.Parallel()

	loop := NewSimpleLoop(
		WithVerifyFunc(func(ctx context.Context, state *LoopState) (*Feedback, error) {
			// Always fail with same issue
			return &Feedback{Valid: false, Issues: []string{"build failed: missing import"}}, nil
		}),
		WithContinueFunc(func(state *LoopState) bool {
			return true // Always continue
		}),
	)

	runner := NewLoopRunner(loop, &LoopConfig{
		MaxIterations:  10,
		StuckDetection: DefaultStuckConfig(),
	})

	state, err := runner.Run(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stuck pattern detected")
	assert.Contains(t, err.Error(), "same error 3 times")
	// Should stop at iteration 3 (when repeat detected)
	assert.Equal(t, 3, state.Iteration)
}

// TestLoopRunnerStuckDetectionOscillation tests stuck detection for oscillating results.
func TestLoopRunnerStuckDetectionOscillation(t *testing.T) {
	t.Parallel()

	iteration := 0
	loop := NewSimpleLoop(
		WithVerifyFunc(func(ctx context.Context, state *LoopState) (*Feedback, error) {
			iteration++
			// Alternate between pass and fail
			passed := iteration%2 == 0
			return &Feedback{Valid: passed, Issues: []string{"test failed"}}, nil
		}),
		WithContinueFunc(func(state *LoopState) bool {
			return true
		}),
	)

	runner := NewLoopRunner(loop, &LoopConfig{
		MaxIterations:  10,
		StuckDetection: DefaultStuckConfig(),
	})

	state, err := runner.Run(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stuck pattern detected")
	assert.Contains(t, err.Error(), "oscillating")
	// Should stop at iteration 4 (when oscillation detected)
	assert.Equal(t, 4, state.Iteration)
}

// TestLoopRunnerStuckDetectionWithHandler tests OnStuckPattern hook.
func TestLoopRunnerStuckDetectionWithHandler(t *testing.T) {
	t.Parallel()

	var capturedPattern *StuckPattern
	continueLoop := true

	loop := NewSimpleLoop(
		WithVerifyFunc(func(ctx context.Context, state *LoopState) (*Feedback, error) {
			return &Feedback{Valid: false, Issues: []string{"same error"}}, nil
		}),
		WithContinueFunc(func(state *LoopState) bool {
			return true
		}),
	)

	runner := NewLoopRunner(loop, &LoopConfig{
		MaxIterations:  10,
		StuckDetection: DefaultStuckConfig(),
		OnStuckPattern: func(pattern *StuckPattern, state *LoopState) bool {
			capturedPattern = pattern
			return continueLoop
		},
	})

	// First: handler returns true, loop continues until max iterations
	_, err := runner.Run(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max iterations")
	require.NotNil(t, capturedPattern)
	assert.Equal(t, PatternRepeatAttempt, capturedPattern.Type)

	// Second: handler returns false, loop stops immediately
	continueLoop = false
	capturedPattern = nil

	runner2 := NewLoopRunner(loop, &LoopConfig{
		MaxIterations:  10,
		StuckDetection: DefaultStuckConfig(),
		OnStuckPattern: func(pattern *StuckPattern, state *LoopState) bool {
			capturedPattern = pattern
			return continueLoop
		},
	})

	state, err := runner2.Run(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stuck pattern detected")
	assert.Equal(t, 3, state.Iteration)
}

// TestLoopRunnerStuckDetectionDisabled tests that nil StuckDetection disables detection.
func TestLoopRunnerStuckDetectionDisabled(t *testing.T) {
	t.Parallel()

	loop := NewSimpleLoop(
		WithVerifyFunc(func(ctx context.Context, state *LoopState) (*Feedback, error) {
			return &Feedback{Valid: false, Issues: []string{"same error"}}, nil
		}),
		WithContinueFunc(func(state *LoopState) bool {
			return state.Iteration < 5
		}),
	)

	runner := NewLoopRunner(loop, &LoopConfig{
		MaxIterations:  10,
		StuckDetection: nil, // Disabled
	})

	state, err := runner.Run(context.Background())
	require.NoError(t, err)
	// Should complete normally without stuck detection
	assert.Equal(t, 5, state.Iteration)
}

// TestLoopRunnerStuckDetectionNoFalsePositive tests that different errors don't trigger.
func TestLoopRunnerStuckDetectionNoFalsePositive(t *testing.T) {
	t.Parallel()

	iteration := 0
	loop := NewSimpleLoop(
		WithVerifyFunc(func(ctx context.Context, state *LoopState) (*Feedback, error) {
			iteration++
			// Different error each time
			return &Feedback{
				Valid:  false,
				Issues: []string{fmt.Sprintf("error %d", iteration)},
			}, nil
		}),
		WithContinueFunc(func(state *LoopState) bool {
			return state.Iteration < 5
		}),
	)

	runner := NewLoopRunner(loop, &LoopConfig{
		MaxIterations:  10,
		StuckDetection: DefaultStuckConfig(),
	})

	state, err := runner.Run(context.Background())
	require.NoError(t, err)
	// Should complete without stuck detection (different errors)
	assert.Equal(t, 5, state.Iteration)
}
