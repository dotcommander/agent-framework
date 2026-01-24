package agent

import (
	"encoding/json"
	"os"
	"strings"
)

// Response wraps an agent response with metadata and helper methods.
type Response struct {
	// Content is the raw text response
	Content string

	// Usage contains token consumption details
	Usage *TokenUsage

	// ToolCalls lists tools that were invoked during this response
	ToolCalls []ToolCall

	// Model is the model that generated this response
	Model string

	// SessionID for conversation continuity
	SessionID string
}

// TokenUsage tracks token consumption.
type TokenUsage struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
	// CostUSD is estimated cost (optional, may be 0)
	CostUSD float64
}

// ToolCall records a tool invocation.
type ToolCall struct {
	Name   string
	Input  map[string]any
	Output any
	Error  error
}

// String returns the content (implements fmt.Stringer).
func (r *Response) String() string {
	if r == nil {
		return ""
	}
	return r.Content
}

// JSON unmarshals the content into the provided value.
func (r *Response) JSON(v any) error {
	if r == nil {
		return json.Unmarshal([]byte(""), v)
	}
	return json.Unmarshal([]byte(r.Content), v)
}

// SaveTo writes the content to a file.
func (r *Response) SaveTo(path string) error {
	content := ""
	if r != nil {
		content = r.Content
	}
	return os.WriteFile(path, []byte(content), 0644)
}

// Lines returns content split by newlines.
func (r *Response) Lines() []string {
	if r == nil || r.Content == "" {
		return nil
	}
	return strings.Split(r.Content, "\n")
}

// Contains checks if content contains substring.
func (r *Response) Contains(s string) bool {
	if r == nil {
		return false
	}
	return strings.Contains(r.Content, s)
}

// IsEmpty returns true if content is empty or whitespace only.
func (r *Response) IsEmpty() bool {
	if r == nil {
		return true
	}
	return strings.TrimSpace(r.Content) == ""
}

// Bytes returns content as byte slice.
func (r *Response) Bytes() []byte {
	if r == nil {
		return nil
	}
	return []byte(r.Content)
}

// HasToolCalls returns true if any tools were called.
func (r *Response) HasToolCalls() bool {
	return r != nil && len(r.ToolCalls) > 0
}

// ToolCallsByName returns tool calls matching the given name.
func (r *Response) ToolCallsByName(name string) []ToolCall {
	if r == nil {
		return nil
	}
	var result []ToolCall
	for _, tc := range r.ToolCalls {
		if tc.Name == name {
			result = append(result, tc)
		}
	}
	return result
}
