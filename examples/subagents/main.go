// Package main demonstrates parallel subagent execution.
//
// Subagents are isolated child agents with their own context that can
// run concurrently. This pattern is useful for decomposing complex tasks.
package main

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/dotcommander/agent/app"
)

func main() {
	fmt.Println("=== Parallel Subagent Execution Demo ===")
	fmt.Println()

	// Create a custom executor that simulates work
	executor := app.SubagentExecutorFunc(func(ctx context.Context, agent *app.Subagent) (*app.SubagentResult, error) {
		fmt.Printf("[%s] Starting task: %s\n", agent.Name, agent.Task)

		// Simulate work with random duration
		workDuration := time.Duration(100+rand.Intn(200)) * time.Millisecond
		time.Sleep(workDuration)

		// Access subagent's isolated context
		priority, _ := agent.Context.State["priority"].(string)
		fmt.Printf("[%s] Completed (priority: %s, duration: %v)\n",
			agent.Name, priority, workDuration)

		return &app.SubagentResult{
			Success: true,
			Output: map[string]any{
				"agent":    agent.Name,
				"task":     agent.Task,
				"duration": workDuration.String(),
				"priority": priority,
			},
			Tokens: 100 + rand.Intn(50),
		}, nil
	})

	// Configure subagent manager
	config := &app.SubagentConfig{
		MaxConcurrent:   3, // Run up to 3 subagents in parallel
		IsolateContext:  true,
		ShareTools:      true,
		PropagateCancel: true,
	}

	manager := app.NewSubagentManager(config, executor)

	// Spawn multiple subagents with different tasks and isolated contexts
	researcher := manager.Spawn("researcher", "Research Go concurrency patterns",
		app.WithSubagentPrompt("You are a code research assistant"),
		app.WithSubagentState(map[string]any{
			"priority": "high",
			"domain":   "concurrency",
		}),
		app.WithSubagentTools([]app.ToolInfo{
			{Name: "search", Description: "Search documentation"},
			{Name: "analyze", Description: "Analyze code patterns"},
		}),
	)

	analyzer := manager.Spawn("analyzer", "Analyze existing codebase structure",
		app.WithSubagentPrompt("You are a code analysis assistant"),
		app.WithSubagentState(map[string]any{
			"priority": "medium",
			"scope":    "project-wide",
		}),
		app.WithSubagentMaxTokens(50000),
	)

	implementer := manager.Spawn("implementer", "Implement the worker pool",
		app.WithSubagentPrompt("You are a code implementation assistant"),
		app.WithSubagentState(map[string]any{
			"priority": "high",
			"language": "go",
		}),
		app.WithSubagentMessages([]app.Message{
			{Role: "user", Content: "Create a worker pool implementation"},
		}),
	)

	reviewer := manager.Spawn("reviewer", "Review code for best practices",
		app.WithSubagentPrompt("You are a code review assistant"),
		app.WithSubagentState(map[string]any{
			"priority": "low",
			"strict":   true,
		}),
	)

	tester := manager.Spawn("tester", "Write comprehensive tests",
		app.WithSubagentPrompt("You are a testing assistant"),
		app.WithSubagentState(map[string]any{
			"priority":   "medium",
			"coverage":   "90%",
			"test_types": []string{"unit", "integration"},
		}),
	)

	// Print spawned agents
	fmt.Printf("Spawned %d subagents:\n", len(manager.List()))
	for _, agent := range manager.List() {
		fmt.Printf("  - %s (ID: %s): %s\n", agent.Name, agent.ID, agent.Task)
	}
	fmt.Println()

	// Run all subagents concurrently
	fmt.Println("Running all subagents in parallel (max 3 concurrent)...")
	fmt.Println()

	ctx := context.Background()
	startTime := time.Now()

	results, err := manager.RunAll(ctx)
	if err != nil {
		fmt.Printf("Error running subagents: %v\n", err)
		return
	}

	totalDuration := time.Since(startTime)

	// Display results
	fmt.Println("\n=== Results ===")
	fmt.Printf("Total execution time: %v\n", totalDuration)
	fmt.Printf("Total results: %d\n\n", len(results))

	// Filter and display successful results
	successful := app.FilterResults(results, true)
	fmt.Printf("Successful: %d\n", len(successful))
	for id, result := range successful {
		output := result.Output.(map[string]any)
		fmt.Printf("  [%s] %s - %s (tokens: %d)\n",
			id, output["agent"], output["task"], result.Tokens)
	}

	// Filter and display failed results
	failed := app.FilterResults(results, false)
	if len(failed) > 0 {
		fmt.Printf("\nFailed: %d\n", len(failed))
		for id, result := range failed {
			fmt.Printf("  [%s] Error: %v\n", id, result.Error)
		}
	}

	// Merge outputs and aggregate tokens
	outputs := app.MergeResults(results)
	totalTokens := app.AggregateTokens(results)

	fmt.Printf("\nTotal tokens used: %d\n", totalTokens)
	fmt.Printf("Outputs collected: %d\n", len(outputs))

	// Demonstrate running specific subagents
	fmt.Println("\n=== Running Specific Subagents ===")

	// Clear previous results
	manager.Clear()

	// Spawn new targeted agents
	fast1 := manager.Spawn("fast1", "Quick task 1",
		app.WithSubagentState(map[string]any{"priority": "high"}))
	fast2 := manager.Spawn("fast2", "Quick task 2",
		app.WithSubagentState(map[string]any{"priority": "high"}))

	// Run only specific agents
	specificResults, err := manager.RunAgents(ctx, fast1, fast2)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Ran %d specific subagents\n", len(specificResults))

	// Access individual agent results
	fmt.Printf("\nIndividual agent access:\n")
	fmt.Printf("  researcher ID: %s\n", researcher.ID)
	fmt.Printf("  analyzer ID: %s\n", analyzer.ID)
	fmt.Printf("  implementer ID: %s\n", implementer.ID)
	fmt.Printf("  reviewer ID: %s\n", reviewer.ID)
	fmt.Printf("  tester ID: %s\n", tester.ID)
}
