package agent

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ExtractJSON extracts and parses the first JSON object from an LLM response.
// Handles common LLM quirks: markdown code blocks, leading text, trailing text.
//
// Example:
//
//	type Result struct {
//	    Answer string `json:"answer"`
//	}
//	result, err := agent.ExtractJSON[Result](response)
func ExtractJSON[T any](response string) (*T, error) {
	cleaned := extractJSONString(response, '{', '}')
	if cleaned == "" {
		return nil, fmt.Errorf("no JSON object found in response")
	}

	var result T
	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		return nil, fmt.Errorf("parse JSON: %w", err)
	}
	return &result, nil
}

// ExtractJSONArray extracts and parses the first JSON array from an LLM response.
// Handles common LLM quirks: markdown code blocks, leading text, streaming duplicates.
//
// Example:
//
//	type Item struct {
//	    Name string `json:"name"`
//	}
//	items, err := agent.ExtractJSONArray[Item](response)
func ExtractJSONArray[T any](response string) ([]T, error) {
	cleaned := extractJSONString(response, '[', ']')
	if cleaned == "" {
		return nil, fmt.Errorf("no JSON array found in response")
	}

	var result []T
	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		return nil, fmt.Errorf("parse JSON array: %w", err)
	}
	return result, nil
}

// ExtractMarkdown removes markdown code block wrappers from LLM responses.
// Handles ```markdown, ```json, and bare ``` wrappers.
//
// Example:
//
//	clean := agent.ExtractMarkdown(response)
func ExtractMarkdown(content string) string {
	content = strings.TrimSpace(content)

	// Try common code block prefixes
	prefixes := []string{"```markdown", "```json", "```"}
	for _, prefix := range prefixes {
		if after, found := strings.CutPrefix(content, prefix); found {
			content = after
			// Remove trailing ```
			if idx := strings.LastIndex(content, "```"); idx != -1 {
				content = content[:idx]
			}
			break
		}
	}

	return strings.TrimSpace(content)
}

// extractJSONString finds and extracts JSON from a response.
// Uses bracket matching to find the first complete JSON structure.
func extractJSONString(response string, open, close byte) string {
	response = strings.TrimSpace(response)

	// Handle empty responses
	if response == "" {
		return ""
	}

	// Find the start of JSON
	start := strings.IndexByte(response, open)
	if start == -1 {
		return ""
	}

	// Track bracket nesting to find the matching close
	depth := 0
	end := -1
	inString := false
	escaped := false

	for i := start; i < len(response); i++ {
		b := response[i]

		// Handle escape sequences in strings
		if escaped {
			escaped = false
			continue
		}
		if b == '\\' && inString {
			escaped = true
			continue
		}

		// Track string boundaries
		if b == '"' {
			inString = !inString
			continue
		}

		// Only count brackets outside strings
		if !inString {
			if b == open {
				depth++
			} else if b == close {
				depth--
				if depth == 0 {
					end = i
					break
				}
			}
		}
	}

	if end == -1 || start >= end {
		return ""
	}

	return response[start : end+1]
}

// MustExtractJSON extracts JSON or panics. Use for tests and scripts.
func MustExtractJSON[T any](response string) *T {
	result, err := ExtractJSON[T](response)
	if err != nil {
		panic(fmt.Sprintf("ExtractJSON: %v", err))
	}
	return result
}

// MustExtractJSONArray extracts JSON array or panics. Use for tests and scripts.
func MustExtractJSONArray[T any](response string) []T {
	result, err := ExtractJSONArray[T](response)
	if err != nil {
		panic(fmt.Sprintf("ExtractJSONArray: %v", err))
	}
	return result
}
