// Package main demonstrates advanced features of the agent framework.
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/dotcommander/agent"
	"github.com/dotcommander/agent/app"
)

// CalculateInput is the input schema for the calculate tool.
type CalculateInput struct {
	Operation string  `json:"operation"`
	A         float64 `json:"a"`
	B         float64 `json:"b"`
}

// CalculateOutput is the output schema for the calculate tool.
type CalculateOutput struct {
	Result float64 `json:"result"`
	Error  string  `json:"error,omitempty"`
}

// GetTimeInput is the input for the get_time tool.
type GetTimeInput struct {
	Timezone string `json:"timezone"`
}

// GetTimeOutput is the output for the get_time tool.
type GetTimeOutput struct {
	Time     string `json:"time"`
	Timezone string `json:"timezone"`
}

func main() {
	// Create a calculator tool with type safety
	calcTool := agent.TypedTool[CalculateInput, CalculateOutput](
		"calculate",
		"Performs basic arithmetic operations",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"operation": map[string]any{
					"type":        "string",
					"description": "The operation to perform (add, subtract, multiply, divide)",
					"enum":        []string{"add", "subtract", "multiply", "divide"},
				},
				"a": map[string]any{
					"type":        "number",
					"description": "First operand",
				},
				"b": map[string]any{
					"type":        "number",
					"description": "Second operand",
				},
			},
			"required": []string{"operation", "a", "b"},
		},
		handleCalculate,
	)

	// Create a time tool
	timeTool := agent.TypedTool[GetTimeInput, GetTimeOutput](
		"get_time",
		"Gets the current time in a specific timezone",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"timezone": map[string]any{
					"type":        "string",
					"description": "IANA timezone (e.g., America/New_York, UTC)",
				},
			},
			"required": []string{"timezone"},
		},
		handleGetTime,
	)

	// Create app with custom run function
	application := app.New("advanced-agent", "1.0.0",
		agent.WithSystemPrompt("You are a helpful assistant with access to calculation and time tools."),
		agent.WithModel("claude-sonnet-4-20250514"),
		agent.WithTool(calcTool),
		agent.WithTool(timeTool),
		agent.WithRunFunc(customRunner),
	)

	if err := application.Run(); err != nil {
		log.Fatal(err)
	}
}

// handleCalculate implements the calculator tool logic.
func handleCalculate(ctx context.Context, input CalculateInput) (CalculateOutput, error) {
	var result float64

	switch input.Operation {
	case "add":
		result = input.A + input.B
	case "subtract":
		result = input.A - input.B
	case "multiply":
		result = input.A * input.B
	case "divide":
		if input.B == 0 {
			return CalculateOutput{
				Error: "division by zero",
			}, nil
		}
		result = input.A / input.B
	default:
		return CalculateOutput{
			Error: fmt.Sprintf("unknown operation: %s", input.Operation),
		}, nil
	}

	return CalculateOutput{Result: result}, nil
}

// handleGetTime implements the time tool logic.
func handleGetTime(ctx context.Context, input GetTimeInput) (GetTimeOutput, error) {
	loc, err := time.LoadLocation(input.Timezone)
	if err != nil {
		return GetTimeOutput{}, fmt.Errorf("invalid timezone: %w", err)
	}

	now := time.Now().In(loc)

	return GetTimeOutput{
		Time:     now.Format(time.RFC3339),
		Timezone: input.Timezone,
	}, nil
}

// customRunner demonstrates a custom run function with tool access.
func customRunner(ctx context.Context, a *app.App, args []string) error {
	if len(args) == 0 {
		// No args - demonstrate tool usage
		fmt.Println("=== Advanced Agent Demo ===")
		fmt.Println()
		fmt.Println("Tools available:")
		for _, tool := range a.Tools().List() {
			fmt.Printf("  - %s: %s\n", tool.Name, tool.Description)
		}
		fmt.Println("\nUsage: advanced-agent \"<your prompt>\"")
		fmt.Println("\nExample prompts:")
		fmt.Println("  - \"What is 42 multiplied by 2?\"")
		fmt.Println("  - \"What time is it in Tokyo?\"")
		fmt.Println("  - \"Calculate 100 divided by 5 and tell me the current time in UTC\"")
		return nil
	}

	// Send query to Claude
	prompt := args[0]
	fmt.Printf("Query: %s\n\n", prompt)

	response, err := a.Client().Query(ctx, prompt)
	if err != nil {
		return fmt.Errorf("query failed: %w", err)
	}

	fmt.Println("Response:")
	fmt.Println(response)

	return nil
}
