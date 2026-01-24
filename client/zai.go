package client

// ZAIClient is a mock client for Z.AI API compatibility testing.
// It returns configurable responses and tracks all calls for assertions.
type ZAIClient struct {
	*mockClientBase
}

// Compile-time interface check.
var _ MockClient = (*ZAIClient)(nil)

// NewZAIClient creates a new Z.AI mock client with a default response.
func NewZAIClient() *ZAIClient {
	return &ZAIClient{
		mockClientBase: newMockClientBase("zai"),
	}
}
