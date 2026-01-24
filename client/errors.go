package client

import "errors"

var (
	// ErrNotConnected indicates the client is not connected.
	ErrNotConnected = errors.New("client not connected")

	// ErrInvalidProvider indicates an unsupported provider was specified.
	ErrInvalidProvider = errors.New("invalid provider")
)
