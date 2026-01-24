package search

import (
	"context"
	"testing"
)

// mockEmbedder implements EmbeddingProvider for testing.
type mockEmbedder struct {
	dimension int
}

func (m *mockEmbedder) Embed(_ context.Context, _ string) (Embedding, error) {
	// Return a simple embedding based on dimension
	emb := make(Embedding, m.dimension)
	for i := range emb {
		emb[i] = 0.1 * float64(i+1)
	}
	return emb, nil
}

func (m *mockEmbedder) EmbedBatch(_ context.Context, texts []string) ([]Embedding, error) {
	embeddings := make([]Embedding, len(texts))
	for i := range texts {
		emb := make(Embedding, m.dimension)
		for j := range emb {
			emb[j] = 0.1 * float64(j+1) * float64(i+1)
		}
		embeddings[i] = emb
	}
	return embeddings, nil
}

func (m *mockEmbedder) Dimension() int {
	return m.dimension
}

func TestSemanticIndex_LRUEviction(t *testing.T) {
	embedder := &mockEmbedder{dimension: 8}
	idx := NewSemanticIndex(embedder, nil, WithMaxEntries(3))

	ctx := context.Background()

	// Add 4 documents (limit is 3)
	docs := []string{"doc1", "doc2", "doc3", "doc4"}
	for _, id := range docs {
		err := idx.Add(ctx, &Document{
			ID:      id,
			Content: "content for " + id,
		})
		if err != nil {
			t.Fatalf("Add(%s): %v", id, err)
		}
	}

	// Should have 3 documents (doc1 evicted)
	if got := idx.Count(); got != 3 {
		t.Errorf("Count() = %d, want 3", got)
	}

	// doc1 should be gone
	if doc := idx.Get("doc1"); doc != nil {
		t.Error("doc1 should have been evicted")
	}

	// doc2, doc3, doc4 should exist
	for _, id := range []string{"doc2", "doc3", "doc4"} {
		if doc := idx.Get(id); doc == nil {
			t.Errorf("%s should exist", id)
		}
	}
}

func TestSemanticIndex_LRUAccessUpdates(t *testing.T) {
	embedder := &mockEmbedder{dimension: 8}
	idx := NewSemanticIndex(embedder, nil, WithMaxEntries(3))

	ctx := context.Background()

	// Add 3 documents
	for _, id := range []string{"doc1", "doc2", "doc3"} {
		err := idx.Add(ctx, &Document{
			ID:      id,
			Content: "content for " + id,
		})
		if err != nil {
			t.Fatalf("Add(%s): %v", id, err)
		}
	}

	// Access doc1 to make it most recently used
	_ = idx.Get("doc1")

	// Add doc4 - should evict doc2 (least recently used)
	err := idx.Add(ctx, &Document{
		ID:      "doc4",
		Content: "content for doc4",
	})
	if err != nil {
		t.Fatalf("Add(doc4): %v", err)
	}

	// doc2 should be evicted
	if doc := idx.Get("doc2"); doc != nil {
		t.Error("doc2 should have been evicted")
	}

	// doc1, doc3, doc4 should exist
	for _, id := range []string{"doc1", "doc3", "doc4"} {
		if doc := idx.Get(id); doc == nil {
			t.Errorf("%s should exist", id)
		}
	}
}

func TestSemanticIndex_Pagination(t *testing.T) {
	embedder := &mockEmbedder{dimension: 8}
	idx := NewSemanticIndex(embedder, nil)

	ctx := context.Background()

	// Add 10 documents
	for i := range 10 {
		err := idx.Add(ctx, &Document{
			ID:      string(rune('a' + i)),
			Content: "content",
		})
		if err != nil {
			t.Fatalf("Add: %v", err)
		}
	}

	// Search with pagination
	resp, err := idx.SearchWithOptions(ctx, "content", &SearchOptions{
		Offset: 2,
		Limit:  3,
	})
	if err != nil {
		t.Fatalf("SearchWithOptions: %v", err)
	}

	if resp.TotalCount != 10 {
		t.Errorf("TotalCount = %d, want 10", resp.TotalCount)
	}

	if len(resp.Results) != 3 {
		t.Errorf("len(Results) = %d, want 3", len(resp.Results))
	}

	if resp.Offset != 2 {
		t.Errorf("Offset = %d, want 2", resp.Offset)
	}

	if resp.Limit != 3 {
		t.Errorf("Limit = %d, want 3", resp.Limit)
	}
}

func TestSemanticIndex_PaginationBeyondResults(t *testing.T) {
	embedder := &mockEmbedder{dimension: 8}
	idx := NewSemanticIndex(embedder, nil)

	ctx := context.Background()

	// Add 5 documents
	for i := range 5 {
		_ = idx.Add(ctx, &Document{
			ID:      string(rune('a' + i)),
			Content: "content",
		})
	}

	// Search with offset beyond results
	resp, err := idx.SearchWithOptions(ctx, "content", &SearchOptions{
		Offset: 100,
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("SearchWithOptions: %v", err)
	}

	if resp.TotalCount != 5 {
		t.Errorf("TotalCount = %d, want 5", resp.TotalCount)
	}

	if len(resp.Results) != 0 {
		t.Errorf("len(Results) = %d, want 0", len(resp.Results))
	}
}

func TestSemanticIndex_RemoveUpdatesLRU(t *testing.T) {
	embedder := &mockEmbedder{dimension: 8}
	idx := NewSemanticIndex(embedder, nil, WithMaxEntries(3))

	ctx := context.Background()

	// Add 3 documents
	for _, id := range []string{"doc1", "doc2", "doc3"} {
		_ = idx.Add(ctx, &Document{
			ID:      id,
			Content: "content for " + id,
		})
	}

	// Remove doc1
	idx.Remove("doc1")

	// Add 2 more documents - neither should trigger eviction
	for _, id := range []string{"doc4", "doc5"} {
		_ = idx.Add(ctx, &Document{
			ID:      id,
			Content: "content for " + id,
		})
	}

	// Should have 3 documents (doc2 evicted when doc5 added)
	if got := idx.Count(); got != 3 {
		t.Errorf("Count() = %d, want 3", got)
	}
}

func TestSemanticIndex_Clear(t *testing.T) {
	embedder := &mockEmbedder{dimension: 8}
	idx := NewSemanticIndex(embedder, nil, WithMaxEntries(10))

	ctx := context.Background()

	// Add documents
	for i := range 5 {
		_ = idx.Add(ctx, &Document{
			ID:      string(rune('a' + i)),
			Content: "content",
		})
	}

	idx.Clear()

	if got := idx.Count(); got != 0 {
		t.Errorf("Count() after Clear = %d, want 0", got)
	}

	// Should be able to add more after clear
	err := idx.Add(ctx, &Document{
		ID:      "new",
		Content: "content",
	})
	if err != nil {
		t.Fatalf("Add after Clear: %v", err)
	}

	if got := idx.Count(); got != 1 {
		t.Errorf("Count() after Add = %d, want 1", got)
	}
}

func TestSearchByEmbeddingWithOptions(t *testing.T) {
	embedder := &mockEmbedder{dimension: 8}
	idx := NewSemanticIndex(embedder, nil)

	ctx := context.Background()

	// Add documents with pre-computed embeddings
	for i := range 5 {
		emb := make(Embedding, 8)
		for j := range emb {
			emb[j] = float64(i+1) * 0.1 * float64(j+1)
		}
		_ = idx.Add(ctx, &Document{
			ID:        string(rune('a' + i)),
			Content:   "content",
			Embedding: emb,
		})
	}

	// Create query embedding
	query := make(Embedding, 8)
	for i := range query {
		query[i] = 0.5 * float64(i+1)
	}

	resp := idx.SearchByEmbeddingWithOptions(query, &SearchOptions{
		Offset: 1,
		Limit:  2,
	})

	if resp.TotalCount != 5 {
		t.Errorf("TotalCount = %d, want 5", resp.TotalCount)
	}

	if len(resp.Results) != 2 {
		t.Errorf("len(Results) = %d, want 2", len(resp.Results))
	}
}
