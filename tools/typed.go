package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// TypedHandler creates a type-safe handler from a function.
// The input type T is automatically deserialized from map[string]any,
// and the output type R is returned as-is.
//
// Example:
//
//	type GreetInput struct {
//	    Name string `json:"name"`
//	}
//
//	type GreetOutput struct {
//	    Message string `json:"message"`
//	}
//
//	handler := TypedHandler(func(ctx context.Context, input GreetInput) (GreetOutput, error) {
//	    return GreetOutput{Message: "Hello, " + input.Name}, nil
//	})
func TypedHandler[T any, R any](fn func(ctx context.Context, input T) (R, error)) Handler {
	return func(ctx context.Context, input map[string]any) (any, error) {
		// Marshal and unmarshal to convert map to typed struct
		data, err := json.Marshal(input)
		if err != nil {
			return nil, fmt.Errorf("marshal input: %w", err)
		}

		var typedInput T
		if err := json.Unmarshal(data, &typedInput); err != nil {
			return nil, fmt.Errorf("unmarshal input: %w", err)
		}

		return fn(ctx, typedInput)
	}
}

// TypedTool creates a new tool with a typed handler.
func TypedTool[T any, R any](
	name, description string,
	schema map[string]any,
	fn func(ctx context.Context, input T) (R, error),
) *Tool {
	return NewTool(name, description, schema, TypedHandler(fn))
}
