// Package pathutil provides shared path security utilities.
package pathutil

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Security-related errors.
var (
	ErrPathTraversal   = errors.New("path traversal detected")
	ErrPathOutsideBase = errors.New("path outside allowed base directory")
)

// SanitizeError returns a user-safe error message that doesn't expose
// sensitive path information. The original error is preserved for logging.
// Use this when returning errors that might contain full file paths.
func SanitizeError(operation string, err error) error {
	if err == nil {
		return nil
	}

	// Check for known error types and return sanitized messages
	if os.IsNotExist(err) {
		return fmt.Errorf("%s: file not found", operation)
	}
	if os.IsPermission(err) {
		return fmt.Errorf("%s: permission denied", operation)
	}
	if os.IsTimeout(err) {
		return fmt.Errorf("%s: operation timed out", operation)
	}

	// For path-related errors, return generic message
	// This prevents exposing absolute paths in error messages
	return fmt.Errorf("%s: operation failed", operation)
}

// traversalPatterns contains patterns that indicate path traversal attempts.
var traversalPatterns = []string{
	"../",
	"..\\",
	"/etc/passwd",
	"/etc/shadow",
	"..%2f",
	"..%5c",
	"%2e%2e/",
	"%2e%2e\\",
}

// ContainsTraversal checks if a path contains obvious traversal patterns.
func ContainsTraversal(path string) bool {
	// Normalize path separators for checking
	normalized := filepath.ToSlash(path)

	for _, pattern := range traversalPatterns {
		if strings.Contains(strings.ToLower(normalized), strings.ToLower(pattern)) {
			return true
		}
	}

	return false
}
