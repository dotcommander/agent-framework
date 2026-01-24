// Package main demonstrates the Agent Loop pattern.
//
// The agent loop follows a cycle: GatherContext -> DecideAction -> TakeAction -> Verify
// This pattern enables autonomous operation with self-correction capabilities.
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/dotcommander/agent/app"
)

func main() {
	fmt.Println("=== Agent Loop Pattern Demo ===")
	fmt.Println()

	// Track iteration state for the demo
	taskComplete := false
	counter := 0

	// Create a SimpleLoop with custom functions for each phase
	loop := app.NewSimpleLoop(
		// GatherContext: Collect relevant information for decision-making
		app.WithGatherFunc(func(ctx context.Context, state *app.LoopState) (*app.LoopContext, error) {
			fmt.Printf("[Iteration %d] Gathering context...\n", state.Iteration)

			// In a real agent, this would gather:
			// - Conversation history
			// - Available tools
			// - System state
			return &app.LoopContext{
				Messages: []app.Message{
					{Role: "user", Content: "Increment the counter until it reaches 3"},
				},
				Tools: []app.ToolInfo{
					{Name: "increment", Description: "Increment the counter by 1"},
					{Name: "check", Description: "Check if counter reached goal"},
				},
				State: map[string]any{
					"counter":    counter,
					"goal":       3,
					"start_time": time.Now(),
				},
				TokenCount: 100 * state.Iteration,
			}, nil
		}),

		// DecideAction: Determine what to do based on context
		app.WithDecideFunc(func(ctx context.Context, state *app.LoopState) (*app.Action, error) {
			fmt.Printf("[Iteration %d] Deciding action...\n", state.Iteration)

			currentCounter := state.Context.State["counter"].(int)
			goal := state.Context.State["goal"].(int)

			if currentCounter >= goal {
				// Goal reached, respond with completion
				return &app.Action{
					Type:     "response",
					Response: fmt.Sprintf("Task complete! Counter reached %d", goal),
				}, nil
			}

			// Need to increment - use a tool
			return &app.Action{
				Type:     "tool_call",
				ToolName: "increment",
				ToolInput: map[string]any{
					"amount": 1,
				},
			}, nil
		}),

		// TakeAction: Execute the decided action
		app.WithActionFunc(func(ctx context.Context, action *app.Action) (*app.Result, error) {
			fmt.Printf("[Action] Type=%s", action.Type)

			switch action.Type {
			case "tool_call":
				fmt.Printf(", Tool=%s\n", action.ToolName)
				// Simulate tool execution
				if action.ToolName == "increment" {
					counter++
					return &app.Result{
						Success: true,
						Output:  fmt.Sprintf("Counter incremented to %d", counter),
						Tokens:  50,
					}, nil
				}
			case "response":
				fmt.Printf(", Response=%s\n", action.Response)
				taskComplete = true
				return &app.Result{
					Success: true,
					Output:  action.Response,
					Tokens:  20,
				}, nil
			}

			return &app.Result{Success: false, Error: fmt.Errorf("unknown action")}, nil
		}),

		// Verify: Validate the result and provide feedback
		app.WithVerifyFunc(func(ctx context.Context, state *app.LoopState) (*app.Feedback, error) {
			fmt.Printf("[Iteration %d] Verifying result...\n", state.Iteration)

			if state.LastResult == nil || !state.LastResult.Success {
				return &app.Feedback{
					Valid:  false,
					Issues: []string{"Action failed to execute"},
					Score:  0.0,
				}, nil
			}

			return &app.Feedback{
				Valid:    true,
				Warnings: []string{},
				Score:    1.0,
			}, nil
		}),

		// ShouldContinue: Determine if loop should continue
		app.WithContinueFunc(func(state *app.LoopState) bool {
			// Stop if task is complete or max iterations reached
			shouldContinue := !taskComplete && state.Iteration < 10
			fmt.Printf("[Iteration %d] Continue=%v (taskComplete=%v)\n\n",
				state.Iteration, shouldContinue, taskComplete)
			return shouldContinue
		}),
	)

	// Configure the loop runner
	config := &app.LoopConfig{
		MaxIterations: 10,
		MaxTokens:     10000,
		Timeout:       30 * time.Second,
		StopOnError:   true,
		MinScore:      0.5,
		OnIterationStart: func(state *app.LoopState) {
			fmt.Printf("--- Starting iteration %d ---\n", state.Iteration)
		},
		OnIterationEnd: func(state *app.LoopState) {
			if state.LastResult != nil {
				fmt.Printf("Result: %v\n", state.LastResult.Output)
			}
		},
		OnError: func(err error, state *app.LoopState) {
			fmt.Printf("Error in iteration %d: %v\n", state.Iteration, err)
		},
	}

	// Create and run the loop
	runner := app.NewLoopRunner(loop, config)

	ctx := context.Background()
	finalState, err := runner.Run(ctx)

	// Print results
	fmt.Println("=== Loop Completed ===")
	if err != nil {
		fmt.Printf("Loop ended with error: %v\n", err)
	}
	fmt.Printf("Total iterations: %d\n", finalState.Iteration)
	fmt.Printf("Final counter value: %d\n", counter)
	if finalState.LastVerify != nil {
		fmt.Printf("Final verification score: %.2f\n", finalState.LastVerify.Score)
	}
	fmt.Printf("Duration: %v\n", finalState.CompletedAt.Sub(finalState.StartedAt))
}
