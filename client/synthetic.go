package client

// SyntheticClient is a mock client for testing that doesn't make real API calls.
// It returns configurable responses and tracks all calls for assertions.
type SyntheticClient struct {
	*mockClientBase
}

// Compile-time interface check.
var _ MockClient = (*SyntheticClient)(nil)

// NewSyntheticClient creates a new synthetic client with a default response.
func NewSyntheticClient() *SyntheticClient {
	return &SyntheticClient{
		mockClientBase: newMockClientBase("synthetic"),
	}
}
