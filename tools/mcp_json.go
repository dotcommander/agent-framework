// Package tools provides MCP JSON utilities.
//
// This file contains JSON encoding/decoding helpers with size limits
// to prevent memory exhaustion attacks. These are shared by both
// MCPServer and MCPClient.
package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// MCP-related limits for security.
const (
	// DefaultMaxJSONSize is the maximum size for JSON payloads (1MB).
	DefaultMaxJSONSize = 1 * 1024 * 1024
)

// DecodeJSONSafe decodes JSON with size limits to prevent memory exhaustion.
func DecodeJSONSafe(data []byte, v any, maxSize int64) error {
	if int64(len(data)) > maxSize {
		return fmt.Errorf("JSON payload exceeds maximum size of %d bytes", maxSize)
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields() // Optional: stricter parsing

	return decoder.Decode(v)
}

// DecodeJSONFromReaderSafe decodes JSON from a reader with size limits.
func DecodeJSONFromReaderSafe(r io.Reader, v any, maxSize int64) error {
	limitedReader := io.LimitReader(r, maxSize+1)
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return fmt.Errorf("read JSON: %w", err)
	}

	if int64(len(data)) > maxSize {
		return fmt.Errorf("JSON payload exceeds maximum size of %d bytes", maxSize)
	}

	return json.Unmarshal(data, v)
}
