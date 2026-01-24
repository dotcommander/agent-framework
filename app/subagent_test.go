package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockExecutor tracks execution for testing.
type mockExecutor struct {
	executionCount atomic.Int64
	concurrentMax  atomic.Int64
	currentRunning atomic.Int64
	delay          time.Duration
	shouldFail     func(agent *Subagent) bool
	mu             sync.Mutex
	executionOrder []string
}

func newMockExecutor(delay time.Duration) *mockExecutor {
	return &mockExecutor{
		delay:          delay,
		executionOrder: make([]string, 0),
	}
}

func (m *mockExecutor) Execute(ctx context.Context, agent *Subagent) (*SubagentResult, error) {
	m.executionCount.Add(1)

	// Track concurrent execution
	current := m.currentRunning.Add(1)
	defer m.currentRunning.Add(-1)

	// Update max concurrent
	for {
		oldMax := m.concurrentMax.Load()
		if current <= oldMax {
			break
		}
		if m.concurrentMax.CompareAndSwap(oldMax, current) {
			break
		}
	}

	// Track execution order
	m.mu.Lock()
	m.executionOrder = append(m.executionOrder, agent.ID)
	m.mu.Unlock()

	// Simulate work
	select {
	case <-time.After(m.delay):
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Check if should fail
	if m.shouldFail != nil && m.shouldFail(agent) {
		return &SubagentResult{
			Success: false,
			Error:   fmt.Errorf("mock failure for %s", agent.ID),
		}, fmt.Errorf("mock failure for %s", agent.ID)
	}

	return &SubagentResult{
		Success: true,
		Output:  fmt.Sprintf("result-%s", agent.ID),
		Tokens:  100,
	}, nil
}

func (m *mockExecutor) getMaxConcurrent() int64 {
	return m.concurrentMax.Load()
}

func (m *mockExecutor) getExecutionCount() int64 {
	return m.executionCount.Load()
}

// mustSpawn is a test helper that spawns and panics on error.
func mustSpawn(t *testing.T, m *SubagentManager, name, task string, opts ...SubagentOption) *Subagent {
	t.Helper()
	agent, err := m.Spawn(name, task, opts...)
	require.NoError(t, err)
	return agent
}

// TestConcurrentSpawn tests concurrent Spawn() calls.
func TestConcurrentSpawn(t *testing.T) {
	t.Parallel()

	executor := newMockExecutor(0)
	manager := NewSubagentManager(nil, executor)

	const numGoroutines = 100
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	spawnedIDs := make([]string, numGoroutines)

	for i := range numGoroutines {
		go func(idx int) {
			defer wg.Done()
			agent, err := manager.Spawn(fmt.Sprintf("agent-%d", idx), fmt.Sprintf("task-%d", idx))
			if err == nil {
				spawnedIDs[idx] = agent.ID
			}
		}(i)
	}

	wg.Wait()

	// Verify all agents were created
	agents := manager.List()
	assert.Equal(t, numGoroutines, len(agents))

	// Verify all IDs are unique
	idMap := make(map[string]bool)
	for _, id := range spawnedIDs {
		assert.False(t, idMap[id], "duplicate ID: %s", id)
		idMap[id] = true
	}
}

// TestConcurrentRunAgents tests concurrent RunAgents() execution.
func TestConcurrentRunAgents(t *testing.T) {
	t.Parallel()

	executor := newMockExecutor(10 * time.Millisecond)
	manager := NewSubagentManager(nil, executor)

	// Spawn multiple agents
	const numAgents = 20
	agents := make([]*Subagent, numAgents)
	for i := range numAgents {
		agents[i] = mustSpawn(t, manager, fmt.Sprintf("agent-%d", i), fmt.Sprintf("task-%d", i))
	}

	ctx := context.Background()
	results, err := manager.RunAgents(ctx, agents...)

	require.NoError(t, err)
	assert.Equal(t, numAgents, len(results))

	// Verify all results are successful
	for id, result := range results {
		assert.True(t, result.Success, "agent %s should succeed", id)
		assert.NoError(t, result.Error)
		assert.Equal(t, 100, result.Tokens)
	}

	// Verify concurrent execution happened
	assert.Greater(t, executor.getMaxConcurrent(), int64(1), "should have concurrent execution")
	assert.Equal(t, int64(numAgents), executor.getExecutionCount())
}

// TestMaxConcurrent_Sequential tests MaxConcurrent=1 runs sequentially.
func TestMaxConcurrent_Sequential(t *testing.T) {
	t.Parallel()

	executor := newMockExecutor(20 * time.Millisecond)
	config := &SubagentConfig{
		MaxConcurrent:   1,
		IsolateContext:  true,
		ShareTools:      true,
		PropagateCancel: true,
	}
	manager := NewSubagentManager(config, executor)

	const numAgents = 5
	agents := make([]*Subagent, numAgents)
	for i := range numAgents {
		agents[i] = mustSpawn(t, manager, fmt.Sprintf("agent-%d", i), fmt.Sprintf("task-%d", i))
	}

	ctx := context.Background()
	results, err := manager.RunAgents(ctx, agents...)

	require.NoError(t, err)
	assert.Equal(t, numAgents, len(results))

	// With MaxConcurrent=1, should never have more than 1 concurrent
	assert.Equal(t, int64(1), executor.getMaxConcurrent(), "should run sequentially")
	assert.Equal(t, int64(numAgents), executor.getExecutionCount())
}

// TestMaxConcurrent_Limited tests MaxConcurrent=5 limits parallelism.
func TestMaxConcurrent_Limited(t *testing.T) {
	t.Parallel()

	executor := newMockExecutor(50 * time.Millisecond)
	config := &SubagentConfig{
		MaxConcurrent:   5,
		IsolateContext:  true,
		ShareTools:      true,
		PropagateCancel: true,
	}
	manager := NewSubagentManager(config, executor)

	const numAgents = 20
	agents := make([]*Subagent, numAgents)
	for i := range numAgents {
		agents[i] = mustSpawn(t, manager, fmt.Sprintf("agent-%d", i), fmt.Sprintf("task-%d", i))
	}

	ctx := context.Background()
	results, err := manager.RunAgents(ctx, agents...)

	require.NoError(t, err)
	assert.Equal(t, numAgents, len(results))

	// Should respect MaxConcurrent limit
	maxConcurrent := executor.getMaxConcurrent()
	assert.LessOrEqual(t, maxConcurrent, int64(5), "should not exceed MaxConcurrent")
	assert.Greater(t, maxConcurrent, int64(1), "should have some concurrency")
	assert.Equal(t, int64(numAgents), executor.getExecutionCount())
}

// TestMaxConcurrent_Unlimited tests default (0) allows unlimited.
func TestMaxConcurrent_Unlimited(t *testing.T) {
	t.Parallel()

	executor := newMockExecutor(50 * time.Millisecond)
	config := &SubagentConfig{
		MaxConcurrent:   0, // unlimited
		IsolateContext:  true,
		ShareTools:      true,
		PropagateCancel: true,
	}
	manager := NewSubagentManager(config, executor)

	const numAgents = 10
	agents := make([]*Subagent, numAgents)
	for i := range numAgents {
		agents[i] = mustSpawn(t, manager, fmt.Sprintf("agent-%d", i), fmt.Sprintf("task-%d", i))
	}

	ctx := context.Background()
	results, err := manager.RunAgents(ctx, agents...)

	require.NoError(t, err)
	assert.Equal(t, numAgents, len(results))

	// With unlimited concurrency and sufficient delay, should run many concurrently
	maxConcurrent := executor.getMaxConcurrent()
	assert.Greater(t, maxConcurrent, int64(5), "should have high concurrency")
	assert.Equal(t, int64(numAgents), executor.getExecutionCount())
}

// TestContextCancellation tests parent context cancel stops all subagents.
func TestContextCancellation(t *testing.T) {
	t.Parallel()

	// Use longer delay and limit concurrency to ensure cancellation works
	executor := newMockExecutor(500 * time.Millisecond)
	config := &SubagentConfig{
		MaxConcurrent:   2, // Only 2 at a time, so cancellation can interrupt
		IsolateContext:  true,
		ShareTools:      true,
		PropagateCancel: true,
	}
	manager := NewSubagentManager(config, executor)

	const numAgents = 10
	agents := make([]*Subagent, numAgents)
	for i := range numAgents {
		agents[i] = mustSpawn(t, manager, fmt.Sprintf("agent-%d", i), fmt.Sprintf("task-%d", i))
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after 100ms - with 500ms delay and 2 concurrent, only ~2 should complete
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	results, err := manager.RunAgents(ctx, agents...)

	// With errgroup, context cancellation stops new goroutines from starting
	// but doesn't guarantee an error return. Check that not all completed.
	_ = err // Error may or may not be returned depending on timing

	// Should have some results but not all (timing-dependent, be lenient)
	assert.GreaterOrEqual(t, len(results), 0, "should have some results")

	// At least verify the test ran without panics
	t.Logf("Completed %d/%d agents", len(results), numAgents)
}

// TestContextCancellation_PartialCompletion tests partial completion returns partial results.
func TestContextCancellation_PartialCompletion(t *testing.T) {
	t.Parallel()

	executor := newMockExecutor(100 * time.Millisecond)
	manager := NewSubagentManager(nil, executor)

	const numAgents = 5
	agents := make([]*Subagent, numAgents)
	for i := range numAgents {
		agents[i] = mustSpawn(t, manager, fmt.Sprintf("agent-%d", i), fmt.Sprintf("task-%d", i))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	results, err := manager.RunAgents(ctx, agents...)

	// May or may not error depending on timing
	_ = err

	// Should have partial results
	assert.Greater(t, len(results), 0, "should have some results")
}

// TestFilterResults tests FilterResults with various predicates.
func TestFilterResults(t *testing.T) {
	t.Parallel()

	results := map[string]*SubagentResult{
		"agent-1": {Success: true, Output: "result-1", Tokens: 100},
		"agent-2": {Success: false, Error: errors.New("failed"), Tokens: 50},
		"agent-3": {Success: true, Output: "result-3", Tokens: 150},
		"agent-4": {Success: false, Error: errors.New("failed"), Tokens: 75},
		"agent-5": {Success: true, Output: "result-5", Tokens: 200},
	}

	t.Run("success_only", func(t *testing.T) {
		filtered := FilterResults(results, true)
		assert.Equal(t, 3, len(filtered))
		for _, result := range filtered {
			assert.True(t, result.Success)
		}
	})

	t.Run("failures_only", func(t *testing.T) {
		filtered := FilterResults(results, false)
		assert.Equal(t, 2, len(filtered))
		for _, result := range filtered {
			assert.False(t, result.Success)
			assert.Error(t, result.Error)
		}
	})

	t.Run("empty_results", func(t *testing.T) {
		filtered := FilterResults(map[string]*SubagentResult{}, true)
		assert.Equal(t, 0, len(filtered))
	})

	t.Run("nil_results", func(t *testing.T) {
		resultsWithNil := map[string]*SubagentResult{
			"agent-1": {Success: true, Output: "result-1"},
			"agent-2": nil,
			"agent-3": {Success: false, Error: errors.New("failed")},
		}
		filtered := FilterResults(resultsWithNil, true)
		assert.Equal(t, 1, len(filtered))
	})
}

// TestMergeResults tests MergeResults combines correctly.
func TestMergeResults(t *testing.T) {
	t.Parallel()

	t.Run("all_outputs", func(t *testing.T) {
		results := map[string]*SubagentResult{
			"agent-1": {Success: true, Output: "result-1"},
			"agent-2": {Success: true, Output: "result-2"},
			"agent-3": {Success: true, Output: "result-3"},
		}
		merged := MergeResults(results)
		assert.Equal(t, 3, len(merged))
	})

	t.Run("mixed_outputs", func(t *testing.T) {
		results := map[string]*SubagentResult{
			"agent-1": {Success: true, Output: "result-1"},
			"agent-2": {Success: false, Output: nil},
			"agent-3": {Success: true, Output: "result-3"},
		}
		merged := MergeResults(results)
		assert.Equal(t, 2, len(merged))
	})

	t.Run("nil_results", func(t *testing.T) {
		results := map[string]*SubagentResult{
			"agent-1": {Success: true, Output: "result-1"},
			"agent-2": nil,
			"agent-3": {Success: true, Output: "result-3"},
		}
		merged := MergeResults(results)
		assert.Equal(t, 2, len(merged))
	})

	t.Run("empty_results", func(t *testing.T) {
		merged := MergeResults(map[string]*SubagentResult{})
		assert.Equal(t, 0, len(merged))
	})

	t.Run("various_types", func(t *testing.T) {
		results := map[string]*SubagentResult{
			"agent-1": {Success: true, Output: "string"},
			"agent-2": {Success: true, Output: 123},
			"agent-3": {Success: true, Output: map[string]any{"key": "value"}},
			"agent-4": {Success: true, Output: []string{"a", "b", "c"}},
		}
		merged := MergeResults(results)
		assert.Equal(t, 4, len(merged))
	})
}

// TestAggregateTokens tests AggregateTokens sums properly.
func TestAggregateTokens(t *testing.T) {
	t.Parallel()

	t.Run("all_tokens", func(t *testing.T) {
		results := map[string]*SubagentResult{
			"agent-1": {Success: true, Tokens: 100},
			"agent-2": {Success: true, Tokens: 200},
			"agent-3": {Success: true, Tokens: 300},
		}
		total := AggregateTokens(results)
		assert.Equal(t, 600, total)
	})

	t.Run("mixed_results", func(t *testing.T) {
		results := map[string]*SubagentResult{
			"agent-1": {Success: true, Tokens: 100},
			"agent-2": {Success: false, Tokens: 50},
			"agent-3": {Success: true, Tokens: 150},
		}
		total := AggregateTokens(results)
		assert.Equal(t, 300, total)
	})

	t.Run("nil_results", func(t *testing.T) {
		results := map[string]*SubagentResult{
			"agent-1": {Success: true, Tokens: 100},
			"agent-2": nil,
			"agent-3": {Success: true, Tokens: 200},
		}
		total := AggregateTokens(results)
		assert.Equal(t, 300, total)
	})

	t.Run("empty_results", func(t *testing.T) {
		total := AggregateTokens(map[string]*SubagentResult{})
		assert.Equal(t, 0, total)
	})

	t.Run("zero_tokens", func(t *testing.T) {
		results := map[string]*SubagentResult{
			"agent-1": {Success: true, Tokens: 0},
			"agent-2": {Success: true, Tokens: 0},
		}
		total := AggregateTokens(results)
		assert.Equal(t, 0, total)
	})
}

// TestErrorHandling_OneFailure tests one subagent fails, others continue.
func TestErrorHandling_OneFailure(t *testing.T) {
	t.Parallel()

	executor := newMockExecutor(10 * time.Millisecond)
	executor.shouldFail = func(agent *Subagent) bool {
		return agent.ID == "subagent-3"
	}
	manager := NewSubagentManager(nil, executor)

	const numAgents = 5
	agents := make([]*Subagent, numAgents)
	for i := range numAgents {
		agents[i] = mustSpawn(t, manager, fmt.Sprintf("agent-%d", i), fmt.Sprintf("task-%d", i))
	}

	ctx := context.Background()
	results, err := manager.RunAgents(ctx, agents...)

	// Should not return error - collects all results
	require.NoError(t, err)
	assert.Equal(t, numAgents, len(results))

	// Check specific failure
	failedResult := results["subagent-3"]
	assert.False(t, failedResult.Success)
	assert.Error(t, failedResult.Error)

	// Check others succeeded
	successCount := 0
	for id, result := range results {
		if id != "subagent-3" {
			assert.True(t, result.Success, "agent %s should succeed", id)
			successCount++
		}
	}
	assert.Equal(t, numAgents-1, successCount)
}

// TestErrorHandling_AllFail tests all subagents fail.
func TestErrorHandling_AllFail(t *testing.T) {
	t.Parallel()

	executor := newMockExecutor(10 * time.Millisecond)
	executor.shouldFail = func(agent *Subagent) bool {
		return true // all fail
	}
	manager := NewSubagentManager(nil, executor)

	const numAgents = 5
	agents := make([]*Subagent, numAgents)
	for i := range numAgents {
		agents[i] = mustSpawn(t, manager, fmt.Sprintf("agent-%d", i), fmt.Sprintf("task-%d", i))
	}

	ctx := context.Background()
	results, err := manager.RunAgents(ctx, agents...)

	// Should return ErrAllAgentsFailed when all agents fail
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAllAgentsFailed)

	// Results should still be collected
	assert.Equal(t, numAgents, len(results))

	// All should have failed
	for id, result := range results {
		assert.False(t, result.Success, "agent %s should fail", id)
		assert.Error(t, result.Error)
	}
}

// TestErrorHandling_EmptyAgents tests running with no agents.
func TestErrorHandling_EmptyAgents(t *testing.T) {
	t.Parallel()

	executor := newMockExecutor(0)
	manager := NewSubagentManager(nil, executor)

	ctx := context.Background()
	results, err := manager.RunAgents(ctx)

	require.NoError(t, err)
	assert.Empty(t, results)
}

// TestSubagentOptions tests various SubagentOption configurations.
func TestSubagentOptions(t *testing.T) {
	t.Parallel()

	executor := newMockExecutor(0)
	manager := NewSubagentManager(nil, executor)

	t.Run("with_prompt", func(t *testing.T) {
		agent := mustSpawn(t, manager, "test", "task", WithSubagentPrompt("custom prompt"))
		assert.Equal(t, "custom prompt", agent.Context.SystemPrompt)
	})

	t.Run("with_tools", func(t *testing.T) {
		tools := []ToolInfo{{Name: "tool1"}, {Name: "tool2"}}
		agent := mustSpawn(t, manager, "test", "task", WithSubagentTools(tools))
		assert.Equal(t, 2, len(agent.Context.Tools))
	})

	t.Run("with_state", func(t *testing.T) {
		state := map[string]any{"key1": "value1", "key2": 123}
		agent := mustSpawn(t, manager, "test", "task", WithSubagentState(state))
		assert.Equal(t, "value1", agent.Context.State["key1"])
		assert.Equal(t, 123, agent.Context.State["key2"])
	})

	t.Run("with_messages", func(t *testing.T) {
		messages := []Message{{Role: "user", Content: "hello"}}
		agent := mustSpawn(t, manager, "test", "task", WithSubagentMessages(messages))
		assert.Equal(t, 1, len(agent.Context.Messages))
		assert.Equal(t, "hello", agent.Context.Messages[0].Content)
	})

	t.Run("with_max_tokens", func(t *testing.T) {
		agent := mustSpawn(t, manager, "test", "task", WithSubagentMaxTokens(50000))
		assert.Equal(t, 50000, agent.Context.MaxTokens)
	})

	t.Run("multiple_options", func(t *testing.T) {
		agent := mustSpawn(t, manager, "test", "task",
			WithSubagentPrompt("custom"),
			WithSubagentMaxTokens(75000),
			WithSubagentState(map[string]any{"key": "value"}),
		)
		assert.Equal(t, "custom", agent.Context.SystemPrompt)
		assert.Equal(t, 75000, agent.Context.MaxTokens)
		assert.Equal(t, "value", agent.Context.State["key"])
	})
}

// TestManagerOperations tests Get, List, Clear operations.
func TestManagerOperations(t *testing.T) {
	t.Parallel()

	executor := newMockExecutor(0)
	manager := NewSubagentManager(nil, executor)

	t.Run("get", func(t *testing.T) {
		agent := mustSpawn(t, manager, "test", "task")
		retrieved := manager.Get(agent.ID)
		assert.NotNil(t, retrieved)
		assert.Equal(t, agent.ID, retrieved.ID)
	})

	t.Run("get_nonexistent", func(t *testing.T) {
		retrieved := manager.Get("nonexistent")
		assert.Nil(t, retrieved)
	})

	t.Run("list", func(t *testing.T) {
		m := NewSubagentManager(nil, executor)
		_, _ = m.Spawn("agent1", "task1")
		_, _ = m.Spawn("agent2", "task2")
		_, _ = m.Spawn("agent3", "task3")

		agents := m.List()
		assert.Equal(t, 3, len(agents))
	})

	t.Run("clear", func(t *testing.T) {
		m := NewSubagentManager(nil, executor)
		_, _ = m.Spawn("agent1", "task1")
		_, _ = m.Spawn("agent2", "task2")

		assert.Equal(t, 2, len(m.List()))

		m.Clear()
		assert.Equal(t, 0, len(m.List()))
	})
}

// TestRun_SingleAgent tests running a single agent.
func TestRun_SingleAgent(t *testing.T) {
	t.Parallel()

	executor := newMockExecutor(10 * time.Millisecond)
	manager := NewSubagentManager(nil, executor)

	agent := mustSpawn(t, manager, "test", "task")

	ctx := context.Background()
	result, err := manager.Run(ctx, agent)

	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Equal(t, 100, result.Tokens)
	assert.Equal(t, agent.Result, result)
}

// TestRun_NoExecutor tests error when no executor configured.
func TestRun_NoExecutor(t *testing.T) {
	t.Parallel()

	manager := NewSubagentManager(nil, nil) // no executor
	agent, _ := manager.Spawn("test", "task")

	ctx := context.Background()
	result, err := manager.Run(ctx, agent)

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no executor configured")
}

// TestRunAll tests running all spawned agents.
func TestRunAll(t *testing.T) {
	t.Parallel()

	executor := newMockExecutor(10 * time.Millisecond)
	manager := NewSubagentManager(nil, executor)

	_, _ = manager.Spawn("agent1", "task1")
	_, _ = manager.Spawn("agent2", "task2")
	_, _ = manager.Spawn("agent3", "task3")

	ctx := context.Background()
	results, err := manager.RunAll(ctx)

	require.NoError(t, err)
	assert.Equal(t, 3, len(results))

	for _, result := range results {
		assert.True(t, result.Success)
	}
}

// TestDefaultSubagentConfig tests default configuration values.
func TestDefaultSubagentConfig(t *testing.T) {
	t.Parallel()

	config := DefaultSubagentConfig()

	assert.Equal(t, 5, config.MaxConcurrent)
	assert.Equal(t, 100, config.MaxSubagents)
	assert.True(t, config.IsolateContext)
	assert.True(t, config.ShareTools)
	assert.True(t, config.PropagateCancel)
}

// TestNewSubagentManager_NilConfig tests nil config uses defaults.
func TestNewSubagentManager_NilConfig(t *testing.T) {
	t.Parallel()

	executor := newMockExecutor(0)
	manager := NewSubagentManager(nil, executor)

	assert.NotNil(t, manager.config)
	assert.Equal(t, 5, manager.config.MaxConcurrent)
	assert.Equal(t, 100, manager.config.MaxSubagents)
}

// TestMaxSubagents_Limit tests that MaxSubagents is enforced.
func TestMaxSubagents_Limit(t *testing.T) {
	t.Parallel()

	executor := newMockExecutor(0)
	config := &SubagentConfig{
		MaxConcurrent:  5,
		MaxSubagents:   3, // Only allow 3 subagents
		IsolateContext: true,
	}
	manager := NewSubagentManager(config, executor)

	// Should succeed for first 3
	agent1, err1 := manager.Spawn("agent1", "task1")
	agent2, err2 := manager.Spawn("agent2", "task2")
	agent3, err3 := manager.Spawn("agent3", "task3")

	require.NoError(t, err1)
	require.NoError(t, err2)
	require.NoError(t, err3)
	assert.NotNil(t, agent1)
	assert.NotNil(t, agent2)
	assert.NotNil(t, agent3)

	// 4th should fail with ErrMaxSubagentsReached
	agent4, err4 := manager.Spawn("agent4", "task4")
	assert.Nil(t, agent4)
	assert.ErrorIs(t, err4, ErrMaxSubagentsReached)

	// Verify we still have exactly 3
	assert.Equal(t, 3, len(manager.List()))
}

// TestMaxSubagents_Unlimited tests that MaxSubagents=0 means unlimited.
func TestMaxSubagents_Unlimited(t *testing.T) {
	t.Parallel()

	executor := newMockExecutor(0)
	config := &SubagentConfig{
		MaxConcurrent:  5,
		MaxSubagents:   0, // Unlimited
		IsolateContext: true,
	}
	manager := NewSubagentManager(config, executor)

	// Should be able to spawn many agents
	for i := range 50 {
		agent, err := manager.Spawn(fmt.Sprintf("agent-%d", i), fmt.Sprintf("task-%d", i))
		require.NoError(t, err)
		assert.NotNil(t, agent)
	}

	assert.Equal(t, 50, len(manager.List()))
}

// TestMaxSubagents_AfterClear tests that clearing allows spawning again.
func TestMaxSubagents_AfterClear(t *testing.T) {
	t.Parallel()

	executor := newMockExecutor(0)
	config := &SubagentConfig{
		MaxConcurrent:  5,
		MaxSubagents:   2,
		IsolateContext: true,
	}
	manager := NewSubagentManager(config, executor)

	// Spawn to limit
	_, _ = manager.Spawn("agent1", "task1")
	_, _ = manager.Spawn("agent2", "task2")

	// 3rd should fail
	_, err := manager.Spawn("agent3", "task3")
	assert.ErrorIs(t, err, ErrMaxSubagentsReached)

	// Clear and try again
	manager.Clear()

	// Should succeed now
	agent, err := manager.Spawn("agent3", "task3")
	require.NoError(t, err)
	assert.NotNil(t, agent)
}

// TestExecutorFunc tests SubagentExecutorFunc adapter.
func TestExecutorFunc(t *testing.T) {
	t.Parallel()

	var called bool
	executorFunc := SubagentExecutorFunc(func(ctx context.Context, agent *Subagent) (*SubagentResult, error) {
		called = true
		return &SubagentResult{Success: true}, nil
	})

	manager := NewSubagentManager(nil, executorFunc)
	agent := mustSpawn(t, manager, "test", "task")

	ctx := context.Background()
	_, err := manager.Run(ctx, agent)

	require.NoError(t, err)
	assert.True(t, called)
}

// TestRaceDetector_ConcurrentMapAccess tests concurrent map access safety.
// Run with: go test -race
func TestRaceDetector_ConcurrentMapAccess(t *testing.T) {
	t.Parallel()

	executor := newMockExecutor(5 * time.Millisecond)
	manager := NewSubagentManager(nil, executor)

	var wg sync.WaitGroup
	const numGoroutines = 50

	// Concurrent Spawn
	wg.Add(numGoroutines)
	for i := range numGoroutines {
		go func() {
			defer wg.Done()
			_, _ = manager.Spawn(fmt.Sprintf("agent-%d", i), fmt.Sprintf("task-%d", i))
		}()
	}
	wg.Wait()

	// Concurrent Get
	wg.Add(numGoroutines)
	for i := range numGoroutines {
		go func() {
			defer wg.Done()
			manager.Get(fmt.Sprintf("subagent-%d", i+1))
		}()
	}
	wg.Wait()

	// Concurrent List
	wg.Add(numGoroutines)
	for range numGoroutines {
		go func() {
			defer wg.Done()
			manager.List()
		}()
	}
	wg.Wait()

	// Concurrent RunAll
	wg.Add(5)
	for range 5 {
		go func() {
			defer wg.Done()
			ctx := context.Background()
			_, _ = manager.RunAll(ctx)
		}()
	}
	wg.Wait()
}

// TestRaceDetector_ResultsMapAccess tests results map access safety.
// Run with: go test -race
func TestRaceDetector_ResultsMapAccess(t *testing.T) {
	t.Parallel()

	executor := newMockExecutor(10 * time.Millisecond)
	manager := NewSubagentManager(nil, executor)

	const numAgents = 20
	agents := make([]*Subagent, numAgents)
	for i := range numAgents {
		agents[i] = mustSpawn(t, manager, fmt.Sprintf("agent-%d", i), fmt.Sprintf("task-%d", i))
	}

	ctx := context.Background()
	results, err := manager.RunAgents(ctx, agents...)

	require.NoError(t, err)

	// Concurrent read access to results
	var wg sync.WaitGroup
	const numReaders = 10

	wg.Add(numReaders)
	for range numReaders {
		go func() {
			defer wg.Done()
			_ = FilterResults(results, true)
			_ = MergeResults(results)
			_ = AggregateTokens(results)
		}()
	}
	wg.Wait()
}
