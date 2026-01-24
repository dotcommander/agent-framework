// Package input provides input detection and processing.
package input

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Type represents the type of input detected.
type Type int

const (
	// TypeUnknown indicates the input type could not be determined.
	TypeUnknown Type = iota

	// TypeURL indicates the input is a URL.
	TypeURL

	// TypeFile indicates the input is a file path.
	TypeFile

	// TypeGlob indicates the input is a glob pattern.
	TypeGlob

	// TypeText indicates the input is plain text.
	TypeText

	// TypeStdin indicates the input comes from stdin.
	TypeStdin
)

// String returns the string representation of the Type.
func (t Type) String() string {
	switch t {
	case TypeURL:
		return "url"
	case TypeFile:
		return "file"
	case TypeGlob:
		return "glob"
	case TypeText:
		return "text"
	case TypeStdin:
		return "stdin"
	default:
		return "unknown"
	}
}

// Detect determines the type of input.
func Detect(value string) Type {
	if value == "" {
		return TypeUnknown
	}

	// Check for URL
	if isURL(value) {
		return TypeURL
	}

	// Check for glob pattern
	if isGlob(value) {
		return TypeGlob
	}

	// Check for file path
	if isFile(value) {
		return TypeFile
	}

	// Default to text
	return TypeText
}

// isURL checks if the value is a valid URL.
func isURL(value string) bool {
	if !strings.Contains(value, "://") {
		return false
	}
	u, err := url.Parse(value)
	return err == nil && u.Scheme != "" && u.Host != ""
}

// isGlob checks if the value contains glob patterns.
func isGlob(value string) bool {
	return strings.ContainsAny(value, "*?[]")
}

// isFile checks if the value is a valid file path.
func isFile(value string) bool {
	// Expand tilde
	if strings.HasPrefix(value, "~") {
		home, err := os.UserHomeDir()
		if err == nil {
			value = filepath.Join(home, value[1:])
		}
	}

	// Check if file exists
	_, err := os.Stat(value)
	return err == nil
}
