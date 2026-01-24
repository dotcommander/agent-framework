// Package search provides code search and retrieval capabilities.
package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const (
	openAIEmbeddingURL     = "https://api.openai.com/v1/embeddings"
	openAIDefaultModel     = "text-embedding-3-small"
	openAIDefaultDimension = 1536
	openAIMaxBatchSize     = 2048 // OpenAI allows up to 2048 inputs per request
)

// OpenAIEmbeddingProvider generates embeddings using OpenAI's API.
type OpenAIEmbeddingProvider struct {
	apiKey     string
	model      string
	dimension  int
	httpClient *http.Client
	batchSize  int
}

// OpenAIOption configures the OpenAI embedding provider.
type OpenAIOption func(*OpenAIEmbeddingProvider)

// WithOpenAIModel sets the embedding model.
// Default is "text-embedding-3-small".
func WithOpenAIModel(model string) OpenAIOption {
	return func(p *OpenAIEmbeddingProvider) {
		p.model = model
	}
}

// WithOpenAIDimension sets the embedding dimension.
// Only applicable to text-embedding-3-* models.
func WithOpenAIDimension(dim int) OpenAIOption {
	return func(p *OpenAIEmbeddingProvider) {
		p.dimension = dim
	}
}

// WithOpenAIHTTPClient sets a custom HTTP client.
func WithOpenAIHTTPClient(client *http.Client) OpenAIOption {
	return func(p *OpenAIEmbeddingProvider) {
		p.httpClient = client
	}
}

// WithOpenAIBatchSize sets the batch size for embedding requests.
// Default is 2048 (OpenAI max).
func WithOpenAIBatchSize(size int) OpenAIOption {
	return func(p *OpenAIEmbeddingProvider) {
		if size > 0 && size <= openAIMaxBatchSize {
			p.batchSize = size
		}
	}
}

// NewOpenAIEmbeddingProvider creates a new OpenAI embedding provider.
// Reads OPENAI_API_KEY from environment if apiKey is empty.
func NewOpenAIEmbeddingProvider(apiKey string, opts ...OpenAIOption) (*OpenAIEmbeddingProvider, error) {
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY not set")
	}

	p := &OpenAIEmbeddingProvider{
		apiKey:    apiKey,
		model:     openAIDefaultModel,
		dimension: openAIDefaultDimension,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		batchSize: openAIMaxBatchSize,
	}

	for _, opt := range opts {
		opt(p)
	}

	return p, nil
}

// openAIEmbeddingRequest represents the request to OpenAI's embedding API.
type openAIEmbeddingRequest struct {
	Input          any    `json:"input"` // string or []string
	Model          string `json:"model"`
	EncodingFormat string `json:"encoding_format,omitempty"`
	Dimensions     int    `json:"dimensions,omitempty"`
}

// openAIEmbeddingResponse represents the response from OpenAI's embedding API.
type openAIEmbeddingResponse struct {
	Object string `json:"object"`
	Data   []struct {
		Object    string    `json:"object"`
		Embedding []float64 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Model string `json:"model"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

// openAIErrorResponse represents an error response from OpenAI.
type openAIErrorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

// Embed generates an embedding for the given text.
func (p *OpenAIEmbeddingProvider) Embed(ctx context.Context, text string) (Embedding, error) {
	embeddings, err := p.doEmbedRequest(ctx, text)
	if err != nil {
		return nil, err
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("no embedding returned")
	}
	return embeddings[0], nil
}

// EmbedBatch generates embeddings for multiple texts.
// Automatically batches requests to respect OpenAI's limits.
func (p *OpenAIEmbeddingProvider) EmbedBatch(ctx context.Context, texts []string) ([]Embedding, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	// Process in batches
	var allEmbeddings []Embedding

	for i := 0; i < len(texts); i += p.batchSize {
		end := i + p.batchSize
		if end > len(texts) {
			end = len(texts)
		}
		batch := texts[i:end]

		embeddings, err := p.doEmbedRequest(ctx, batch)
		if err != nil {
			return nil, fmt.Errorf("batch %d-%d: %w", i, end, err)
		}

		allEmbeddings = append(allEmbeddings, embeddings...)
	}

	return allEmbeddings, nil
}

// Dimension returns the embedding dimension.
func (p *OpenAIEmbeddingProvider) Dimension() int {
	return p.dimension
}

// doEmbedRequest performs the actual embedding request.
// input can be a string or []string.
func (p *OpenAIEmbeddingProvider) doEmbedRequest(ctx context.Context, input any) ([]Embedding, error) {
	req := openAIEmbeddingRequest{
		Input: input,
		Model: p.model,
	}

	// Only set dimensions for text-embedding-3-* models
	if p.dimension > 0 && (p.model == "text-embedding-3-small" || p.model == "text-embedding-3-large") {
		req.Dimensions = p.dimension
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", openAIEmbeddingURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp openAIErrorResponse
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error.Message != "" {
			return nil, fmt.Errorf("openai error (%s): %s", errResp.Error.Type, errResp.Error.Message)
		}
		return nil, fmt.Errorf("openai error: status %d", resp.StatusCode)
	}

	var embResp openAIEmbeddingResponse
	if err := json.Unmarshal(respBody, &embResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	// Sort by index to maintain order
	embeddings := make([]Embedding, len(embResp.Data))
	for _, d := range embResp.Data {
		if d.Index < len(embeddings) {
			embeddings[d.Index] = d.Embedding
		}
	}

	return embeddings, nil
}
