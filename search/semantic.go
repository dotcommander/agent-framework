// Package search provides code search and retrieval capabilities.
package search

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
)

// Embedding represents a vector embedding.
type Embedding []float64

// Document represents a searchable document.
type Document struct {
	ID        string           `json:"id"`
	Content   string           `json:"content"`
	Metadata  map[string]any   `json:"metadata,omitempty"`
	Embedding Embedding        `json:"embedding,omitempty"`
	Chunks    []*DocumentChunk `json:"chunks,omitempty"`
}

// DocumentChunk represents a chunk of a document.
type DocumentChunk struct {
	ID         string    `json:"id"`
	DocumentID string    `json:"document_id"`
	Content    string    `json:"content"`
	StartPos   int       `json:"start_pos"`
	EndPos     int       `json:"end_pos"`
	Embedding  Embedding `json:"embedding,omitempty"`
}

// SearchResult represents a search result.
type SearchResult struct {
	Document   *Document      `json:"document"`
	Chunk      *DocumentChunk `json:"chunk,omitempty"`
	Score      float64        `json:"score"`
	Highlights []string       `json:"highlights,omitempty"`
}

// EmbeddingProvider generates embeddings for text.
type EmbeddingProvider interface {
	// Embed generates an embedding for the given text.
	Embed(ctx context.Context, text string) (Embedding, error)

	// EmbedBatch generates embeddings for multiple texts.
	EmbedBatch(ctx context.Context, texts []string) ([]Embedding, error)

	// Dimension returns the embedding dimension.
	Dimension() int
}

// SemanticIndex stores documents with embeddings for search.
type SemanticIndex struct {
	documents map[string]*Document
	chunks    map[string]*DocumentChunk
	embedder  EmbeddingProvider
	chunker   Chunker
	mu        sync.RWMutex
}

// IndexConfig configures the semantic index.
type IndexConfig struct {
	ChunkSize    int
	ChunkOverlap int
	MaxChunks    int
}

// DefaultIndexConfig returns sensible defaults.
func DefaultIndexConfig() *IndexConfig {
	return &IndexConfig{
		ChunkSize:    512,
		ChunkOverlap: 64,
		MaxChunks:    100,
	}
}

// NewSemanticIndex creates a new semantic index.
func NewSemanticIndex(embedder EmbeddingProvider, chunker Chunker) *SemanticIndex {
	return &SemanticIndex{
		documents: make(map[string]*Document),
		chunks:    make(map[string]*DocumentChunk),
		embedder:  embedder,
		chunker:   chunker,
	}
}

// Add adds a document to the index.
func (idx *SemanticIndex) Add(ctx context.Context, doc *Document) error {
	if doc.ID == "" {
		return fmt.Errorf("document ID is required")
	}

	// Generate document embedding
	if len(doc.Embedding) == 0 && idx.embedder != nil {
		embedding, err := idx.embedder.Embed(ctx, doc.Content)
		if err != nil {
			return fmt.Errorf("embed document: %w", err)
		}
		doc.Embedding = embedding
	}

	// Chunk document
	if idx.chunker != nil && len(doc.Chunks) == 0 {
		chunks := idx.chunker.Chunk(doc)
		doc.Chunks = chunks

		// Embed chunks
		if idx.embedder != nil && len(chunks) > 0 {
			texts := make([]string, len(chunks))
			for i, chunk := range chunks {
				texts[i] = chunk.Content
			}

			embeddings, err := idx.embedder.EmbedBatch(ctx, texts)
			if err != nil {
				return fmt.Errorf("embed chunks: %w", err)
			}

			for i, chunk := range chunks {
				chunk.Embedding = embeddings[i]
			}
		}
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()

	idx.documents[doc.ID] = doc
	for _, chunk := range doc.Chunks {
		idx.chunks[chunk.ID] = chunk
	}

	return nil
}

// AddBatch adds multiple documents.
func (idx *SemanticIndex) AddBatch(ctx context.Context, docs []*Document) error {
	for _, doc := range docs {
		if err := idx.Add(ctx, doc); err != nil {
			return err
		}
	}
	return nil
}

// Remove removes a document from the index.
func (idx *SemanticIndex) Remove(id string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if doc, ok := idx.documents[id]; ok {
		for _, chunk := range doc.Chunks {
			delete(idx.chunks, chunk.ID)
		}
		delete(idx.documents, id)
	}
}

// Search performs semantic search.
func (idx *SemanticIndex) Search(ctx context.Context, query string, topK int) ([]*SearchResult, error) {
	if idx.embedder == nil {
		return nil, fmt.Errorf("no embedder configured")
	}

	// Embed query
	queryEmbedding, err := idx.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	return idx.SearchByEmbedding(queryEmbedding, topK), nil
}

// SearchByEmbedding searches by embedding vector.
func (idx *SemanticIndex) SearchByEmbedding(query Embedding, topK int) []*SearchResult {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var results []*SearchResult

	// Search chunks
	for _, chunk := range idx.chunks {
		if len(chunk.Embedding) == 0 {
			continue
		}

		score := cosineSimilarity(query, chunk.Embedding)
		doc := idx.documents[chunk.DocumentID]

		results = append(results, &SearchResult{
			Document: doc,
			Chunk:    chunk,
			Score:    score,
		})
	}

	// Search documents (if no chunks)
	for _, doc := range idx.documents {
		if len(doc.Chunks) > 0 {
			continue // Already searched via chunks
		}
		if len(doc.Embedding) == 0 {
			continue
		}

		score := cosineSimilarity(query, doc.Embedding)
		results = append(results, &SearchResult{
			Document: doc,
			Score:    score,
		})
	}

	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// Return top K
	if len(results) > topK {
		results = results[:topK]
	}

	return results
}

// HybridSearch combines semantic and keyword search.
func (idx *SemanticIndex) HybridSearch(ctx context.Context, query string, topK int, keywordWeight float64) ([]*SearchResult, error) {
	// Semantic search
	semanticResults, err := idx.Search(ctx, query, topK*2)
	if err != nil {
		return nil, err
	}

	// Keyword search
	keywordResults := idx.KeywordSearch(query, topK*2)

	// Merge and rerank
	scoreMap := make(map[string]float64)
	resultMap := make(map[string]*SearchResult)

	semanticWeight := 1.0 - keywordWeight

	for _, r := range semanticResults {
		key := r.Document.ID
		if r.Chunk != nil {
			key = r.Chunk.ID
		}
		scoreMap[key] += r.Score * semanticWeight
		resultMap[key] = r
	}

	for _, r := range keywordResults {
		key := r.Document.ID
		if r.Chunk != nil {
			key = r.Chunk.ID
		}
		scoreMap[key] += r.Score * keywordWeight
		if _, ok := resultMap[key]; !ok {
			resultMap[key] = r
		}
	}

	// Build final results
	var results []*SearchResult
	for key, score := range scoreMap {
		r := resultMap[key]
		r.Score = score
		results = append(results, r)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > topK {
		results = results[:topK]
	}

	return results, nil
}

// KeywordSearch performs basic keyword matching.
func (idx *SemanticIndex) KeywordSearch(query string, topK int) []*SearchResult {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	queryLower := strings.ToLower(query)
	queryTerms := strings.Fields(queryLower)

	var results []*SearchResult

	for _, doc := range idx.documents {
		contentLower := strings.ToLower(doc.Content)

		// Count term matches
		matches := 0
		for _, term := range queryTerms {
			matches += strings.Count(contentLower, term)
		}

		if matches > 0 {
			// Normalize score by content length
			score := float64(matches) / float64(len(doc.Content)+1) * 100

			results = append(results, &SearchResult{
				Document: doc,
				Score:    score,
			})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > topK {
		results = results[:topK]
	}

	return results
}

// Get retrieves a document by ID.
func (idx *SemanticIndex) Get(id string) *Document {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.documents[id]
}

// Count returns the number of indexed documents.
func (idx *SemanticIndex) Count() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.documents)
}

// Clear removes all documents.
func (idx *SemanticIndex) Clear() {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.documents = make(map[string]*Document)
	idx.chunks = make(map[string]*DocumentChunk)
}

// Chunker splits documents into chunks.
type Chunker interface {
	Chunk(doc *Document) []*DocumentChunk
}

// FixedSizeChunker splits by character count.
type FixedSizeChunker struct {
	Size    int
	Overlap int
}

// NewFixedSizeChunker creates a fixed size chunker.
func NewFixedSizeChunker(size, overlap int) *FixedSizeChunker {
	return &FixedSizeChunker{
		Size:    size,
		Overlap: overlap,
	}
}

// Chunk splits a document into fixed-size chunks.
func (c *FixedSizeChunker) Chunk(doc *Document) []*DocumentChunk {
	content := doc.Content
	if len(content) == 0 {
		return nil
	}

	var chunks []*DocumentChunk
	chunkNum := 0

	for start := 0; start < len(content); {
		end := min(start+c.Size, len(content))

		chunkNum++
		chunks = append(chunks, &DocumentChunk{
			ID:         fmt.Sprintf("%s-chunk-%d", doc.ID, chunkNum),
			DocumentID: doc.ID,
			Content:    content[start:end],
			StartPos:   start,
			EndPos:     end,
		})

		start = end - c.Overlap
		if start >= end {
			break
		}
	}

	return chunks
}

// SentenceChunker splits by sentences.
type SentenceChunker struct {
	MaxSentences int
	Overlap      int
}

// NewSentenceChunker creates a sentence-based chunker.
func NewSentenceChunker(maxSentences, overlap int) *SentenceChunker {
	return &SentenceChunker{
		MaxSentences: maxSentences,
		Overlap:      overlap,
	}
}

// Chunk splits a document by sentences.
func (c *SentenceChunker) Chunk(doc *Document) []*DocumentChunk {
	sentences := splitSentences(doc.Content)
	if len(sentences) == 0 {
		return nil
	}

	var chunks []*DocumentChunk
	chunkNum := 0

	for i := 0; i < len(sentences); {
		end := min(i+c.MaxSentences, len(sentences))

		chunkContent := strings.Join(sentences[i:end], " ")
		chunkNum++

		chunks = append(chunks, &DocumentChunk{
			ID:         fmt.Sprintf("%s-chunk-%d", doc.ID, chunkNum),
			DocumentID: doc.ID,
			Content:    chunkContent,
		})

		// Move forward, ensuring progress
		next := end - c.Overlap
		if next <= i {
			break // No forward progress possible
		}
		i = next
	}

	return chunks
}

// splitSentences splits text into sentences.
func splitSentences(text string) []string {
	// Simple sentence splitting
	var sentences []string
	var current strings.Builder

	for i, r := range text {
		current.WriteRune(r)

		if r == '.' || r == '!' || r == '?' {
			// Check if followed by space or end
			if i == len(text)-1 || text[i+1] == ' ' || text[i+1] == '\n' {
				s := strings.TrimSpace(current.String())
				if s != "" {
					sentences = append(sentences, s)
				}
				current.Reset()
			}
		}
	}

	// Handle remaining text
	if s := strings.TrimSpace(current.String()); s != "" {
		sentences = append(sentences, s)
	}

	return sentences
}

// cosineSimilarity calculates cosine similarity between two vectors.
func cosineSimilarity(a, b Embedding) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

// EuclideanDistance calculates Euclidean distance between two vectors.
func EuclideanDistance(a, b Embedding) float64 {
	if len(a) != len(b) {
		return math.Inf(1)
	}

	var sum float64
	for i := range a {
		diff := a[i] - b[i]
		sum += diff * diff
	}

	return math.Sqrt(sum)
}

// CodeDocument represents a code file for indexing.
type CodeDocument struct {
	*Document
	FilePath string       `json:"file_path"`
	Language string       `json:"language"`
	Symbols  []CodeSymbol `json:"symbols,omitempty"`
}

// CodeSymbol represents a code symbol (function, class, etc.)
type CodeSymbol struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"` // "function", "class", "method", etc.
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Signature string `json:"signature,omitempty"`
}

// CodeIndex indexes code files for semantic search.
type CodeIndex struct {
	*SemanticIndex
	codeDocuments map[string]*CodeDocument
}

// NewCodeIndex creates a new code index.
func NewCodeIndex(embedder EmbeddingProvider) *CodeIndex {
	return &CodeIndex{
		SemanticIndex: NewSemanticIndex(embedder, NewFixedSizeChunker(512, 64)),
		codeDocuments: make(map[string]*CodeDocument),
	}
}

// AddCode adds a code file to the index.
func (idx *CodeIndex) AddCode(ctx context.Context, filePath, language, content string) error {
	doc := &CodeDocument{
		Document: &Document{
			ID:      filePath,
			Content: content,
			Metadata: map[string]any{
				"file_path": filePath,
				"language":  language,
			},
		},
		FilePath: filePath,
		Language: language,
	}

	if err := idx.SemanticIndex.Add(ctx, doc.Document); err != nil {
		return err
	}

	idx.mu.Lock()
	idx.codeDocuments[filePath] = doc
	idx.mu.Unlock()

	return nil
}

// SearchCode searches code with optional language filter.
func (idx *CodeIndex) SearchCode(ctx context.Context, query string, topK int, language string) ([]*SearchResult, error) {
	results, err := idx.Search(ctx, query, topK*2)
	if err != nil {
		return nil, err
	}

	if language == "" {
		if len(results) > topK {
			results = results[:topK]
		}
		return results, nil
	}

	// Filter by language
	var filtered []*SearchResult
	for _, r := range results {
		if lang, ok := r.Document.Metadata["language"].(string); ok && lang == language {
			filtered = append(filtered, r)
		}
	}

	if len(filtered) > topK {
		filtered = filtered[:topK]
	}

	return filtered, nil
}
