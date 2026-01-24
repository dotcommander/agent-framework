// Package main demonstrates the agent CLI framework.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/dotcommander/agent-framework/app"
	"github.com/dotcommander/agent-framework/tools"
)

func main() {
	// Create example tool
	greetTool := tools.TypedTool(
		"greet",
		"Greets a person by name",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "The name of the person to greet",
				},
			},
			"required": []string{"name"},
		},
		func(ctx context.Context, input struct {
			Name string `json:"name"`
		}) (struct {
			Message string `json:"message"`
		}, error) {
			return struct {
				Message string `json:"message"`
			}{
				Message: fmt.Sprintf("Hello, %s!", input.Name),
			}, nil
		},
	)

	// Create application
	application := app.New("agent", "1.0.0",
		app.WithSystemPrompt("You are a helpful assistant."),
		app.WithTool(greetTool),
	)

	// Run application
	if err := application.Run(); err != nil {
		log.Fatal(err)
	}
}
