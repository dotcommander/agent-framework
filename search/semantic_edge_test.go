package search

import (
	"context"
	"sync"
	"testing"
)

// TestLRUEviction_UnlimitedCapacity tests maxEntries = 0 (unlimited).
func TestLRUEviction_UnlimitedCapacity(t *testing.T) {
	embedder := &mockEmbedder{dimension: 8}
	idx := NewSemanticIndex(embedder, nil, WithMaxEntries(0))

	ctx := context.Background()

	// Add many documents - no eviction should occur
	for i := range 100 {
		err := idx.Add(ctx, &Document{
			ID:      string(rune('a' + i%26)) + string(rune('0' + i/26)),
			Content: "content",
		})
		if err != nil {
			t.Fatalf("Add document %d: %v", i, err)
		}
	}

	// All 100 should be present
	if got := idx.Count(); got != 100 {
		t.Errorf("Count() = %d, want 100", got)
	}
}

// TestLRUEviction_MaxEntriesOne tests maxEntries = 1 (always evict oldest).
func TestLRUEviction_MaxEntriesOne(t *testing.T) {
	embedder := &mockEmbedder{dimension: 8}
	idx := NewSemanticIndex(embedder, nil, WithMaxEntries(1))

	ctx := context.Background()

	// Add 3 documents sequentially
	docs := []string{"doc1", "doc2", "doc3"}
	for _, id := range docs {
		err := idx.Add(ctx, &Document{
			ID:      id,
			Content: "content for " + id,
		})
		if err != nil {
			t.Fatalf("Add(%s): %v", id, err)
		}

		// Should only ever have 1 document
		if got := idx.Count(); got != 1 {
			t.Errorf("Count() after Add(%s) = %d, want 1", id, got)
		}
	}

	// Only doc3 should remain
	if doc := idx.Get("doc3"); doc == nil {
		t.Error("doc3 should exist")
	}

	// doc1 and doc2 should be evicted
	for _, id := range []string{"doc1", "doc2"} {
		if doc := idx.Get(id); doc != nil {
			t.Errorf("%s should have been evicted", id)
		}
	}
}

// TestLRUEviction_ChunksRemovedWithDocument tests that eviction removes associated chunks.
func TestLRUEviction_ChunksRemovedWithDocument(t *testing.T) {
	embedder := &mockEmbedder{dimension: 8}
	chunker := NewFixedSizeChunker(10, 2)
	idx := NewSemanticIndex(embedder, chunker, WithMaxEntries(2))

	ctx := context.Background()

	// Add 3 documents (with chunks)
	for i := 1; i <= 3; i++ {
		err := idx.Add(ctx, &Document{
			ID:      string(rune('a' + i - 1)),
			Content: "This is some content that will be chunked into multiple parts for testing purposes.",
		})
		if err != nil {
			t.Fatalf("Add document %d: %v", i, err)
		}
	}

	// Should have 2 documents (doc 'a' evicted)
	if got := idx.Count(); got != 2 {
		t.Errorf("Count() = %d, want 2", got)
	}

	// Verify chunks for 'a' are removed
	idx.mu.RLock()
	for chunkID := range idx.chunks {
		if len(chunkID) > 0 && chunkID[0] == 'a' {
			t.Errorf("chunk %s for evicted document 'a' still exists", chunkID)
		}
	}
	idx.mu.RUnlock()
}

// TestLRUEviction_GetUpdatesPosition tests that Get() moves document to front.
func TestLRUEviction_GetUpdatesPosition(t *testing.T) {
	embedder := &mockEmbedder{dimension: 8}
	idx := NewSemanticIndex(embedder, nil, WithMaxEntries(3))

	ctx := context.Background()

	// Add 3 documents (order: doc1, doc2, doc3)
	for _, id := range []string{"doc1", "doc2", "doc3"} {
		err := idx.Add(ctx, &Document{
			ID:      id,
			Content: "content for " + id,
		})
		if err != nil {
			t.Fatalf("Add(%s): %v", id, err)
		}
	}

	// Access doc1 twice to make it most recently used
	_ = idx.Get("doc1")
	_ = idx.Get("doc1")

	// Add doc4 - should evict doc2 (least recently used)
	err := idx.Add(ctx, &Document{
		ID:      "doc4",
		Content: "content for doc4",
	})
	if err != nil {
		t.Fatalf("Add(doc4): %v", err)
	}

	// doc2 should be evicted (was oldest after doc1 was accessed)
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

// TestConcurrent_AddWithEviction tests concurrent Add() calls with eviction.
func TestConcurrent_AddWithEviction(t *testing.T) {
	embedder := &mockEmbedder{dimension: 8}
	idx := NewSemanticIndex(embedder, nil, WithMaxEntries(50))

	ctx := context.Background()
	var wg sync.WaitGroup

	// Spawn 10 goroutines, each adding 20 documents
	for g := range 10 {
		wg.Add(1)
		go func(goroutine int) {
			defer wg.Done()
			for i := range 20 {
				id := string(rune('A'+goroutine)) + string(rune('0'+i%10)) + string(rune('0'+i/10))
				_ = idx.Add(ctx, &Document{
					ID:      id,
					Content: "concurrent content",
				})
			}
		}(g)
	}

	wg.Wait()

	// Should have exactly maxEntries (50)
	count := idx.Count()
	if count > 50 {
		t.Errorf("Count() = %d, exceeds maxEntries of 50", count)
	}
}

// TestConcurrent_SearchDuringAdd tests concurrent Search() during Add().
func TestConcurrent_SearchDuringAdd(t *testing.T) {
	embedder := &mockEmbedder{dimension: 8}
	idx := NewSemanticIndex(embedder, nil, WithMaxEntries(100))

	ctx := context.Background()
	var wg sync.WaitGroup

	// Writer goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := range 50 {
			_ = idx.Add(ctx, &Document{
				ID:      string(rune('W' + i%26)),
				Content: "writer content",
			})
		}
	}()

	// Multiple reader goroutines
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 20 {
				_, _ = idx.Search(ctx, "content", 5)
			}
		}()
	}

	wg.Wait()

	// Should complete without panics
}

// TestConcurrent_GetAndAdd tests concurrent Get() and Add().
func TestConcurrent_GetAndAdd(t *testing.T) {
	embedder := &mockEmbedder{dimension: 8}
	idx := NewSemanticIndex(embedder, nil, WithMaxEntries(50))

	ctx := context.Background()

	// Pre-populate
	for i := range 20 {
		_ = idx.Add(ctx, &Document{
			ID:      string(rune('a' + i)),
			Content: "initial content",
		})
	}

	var wg sync.WaitGroup

	// Writer goroutines
	for w := range 3 {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := range 30 {
				id := string(rune('A'+worker)) + string(rune('0'+i%10))
				_ = idx.Add(ctx, &Document{
					ID:      id,
					Content: "added content",
				})
			}
		}(w)
	}

	// Reader goroutines
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 50 {
				id := string(rune('a' + i%20))
				_ = idx.Get(id)
			}
		}()
	}

	wg.Wait()

	// Should complete without panics and respect maxEntries
	count := idx.Count()
	if count > 50 {
		t.Errorf("Count() = %d, exceeds maxEntries of 50", count)
	}
}

// TestEdgeCase_EmptyQuerySearch tests searching with an empty query.
func TestEdgeCase_EmptyQuerySearch(t *testing.T) {
	embedder := &mockEmbedder{dimension: 8}
	idx := NewSemanticIndex(embedder, nil)

	ctx := context.Background()

	// Add documents
	for i := range 5 {
		_ = idx.Add(ctx, &Document{
			ID:      string(rune('a' + i)),
			Content: "content",
		})
	}

	// Search with empty query - should not panic
	results, err := idx.Search(ctx, "", 10)
	if err != nil {
		t.Fatalf("Search with empty query: %v", err)
	}

	// Should return results (embedder generates embedding even for empty string)
	if len(results) == 0 {
		t.Error("Expected results even with empty query")
	}
}

// TestEdgeCase_DocumentWithEmptyContent tests adding a document with empty content.
func TestEdgeCase_DocumentWithEmptyContent(t *testing.T) {
	embedder := &mockEmbedder{dimension: 8}
	chunker := NewFixedSizeChunker(10, 2)
	idx := NewSemanticIndex(embedder, chunker)

	ctx := context.Background()

	err := idx.Add(ctx, &Document{
		ID:      "empty-doc",
		Content: "",
	})
	if err != nil {
		t.Fatalf("Add document with empty content: %v", err)
	}

	// Should be added successfully
	doc := idx.Get("empty-doc")
	if doc == nil {
		t.Error("Document with empty content should be retrievable")
	}

	// Should have no chunks (chunker returns nil for empty content)
	if doc != nil && len(doc.Chunks) > 0 {
		t.Errorf("Empty document should have no chunks, got %d", len(doc.Chunks))
	}
}

// TestEdgeCase_DuplicateDocumentIDs tests adding documents with duplicate IDs.
func TestEdgeCase_DuplicateDocumentIDs(t *testing.T) {
	embedder := &mockEmbedder{dimension: 8}
	chunker := NewFixedSizeChunker(10, 2)
	idx := NewSemanticIndex(embedder, chunker, WithMaxEntries(10))

	ctx := context.Background()

	// Add initial document with chunks
	err := idx.Add(ctx, &Document{
		ID:      "duplicate-id",
		Content: "This is the original content with enough text to create multiple chunks.",
	})
	if err != nil {
		t.Fatalf("Add initial document: %v", err)
	}

	initialDoc := idx.Get("duplicate-id")
	if initialDoc == nil {
		t.Fatal("Initial document not found")
	}
	initialChunkCount := len(initialDoc.Chunks)

	// Add document with same ID but different content
	err = idx.Add(ctx, &Document{
		ID:      "duplicate-id",
		Content: "This is new content that should replace the old document.",
	})
	if err != nil {
		t.Fatalf("Add duplicate document: %v", err)
	}

	// Should still have only 1 document
	if got := idx.Count(); got != 1 {
		t.Errorf("Count() = %d, want 1", got)
	}

	// Should have updated content
	updatedDoc := idx.Get("duplicate-id")
	if updatedDoc == nil {
		t.Fatal("Updated document not found")
	}

	if updatedDoc.Content != "This is new content that should replace the old document." {
		t.Error("Document content should be updated")
	}

	// Old chunks should be removed, new chunks added
	newChunkCount := len(updatedDoc.Chunks)
	if newChunkCount == initialChunkCount {
		t.Logf("Chunk counts match (%d), but this is acceptable if content length is similar", newChunkCount)
	}

	// Verify old chunks are not in the index
	idx.mu.RLock()
	chunkCount := len(idx.chunks)
	idx.mu.RUnlock()

	if chunkCount != newChunkCount {
		t.Errorf("Total chunk count = %d, want %d (only new chunks)", chunkCount, newChunkCount)
	}
}

// TestEdgeCase_DuplicateWithLRU tests duplicate ID handling maintains LRU position.
func TestEdgeCase_DuplicateWithLRU(t *testing.T) {
	embedder := &mockEmbedder{dimension: 8}
	idx := NewSemanticIndex(embedder, nil, WithMaxEntries(3))

	ctx := context.Background()

	// Add 3 documents (LRU order: doc1 -> doc2 -> doc3)
	for _, id := range []string{"doc1", "doc2", "doc3"} {
		_ = idx.Add(ctx, &Document{
			ID:      id,
			Content: "original " + id,
		})
	}

	// Re-add doc1 (should move to front of LRU)
	_ = idx.Add(ctx, &Document{
		ID:      "doc1",
		Content: "updated doc1",
	})

	// Add doc4 - should evict doc2 (oldest, since doc1 moved to front)
	_ = idx.Add(ctx, &Document{
		ID:      "doc4",
		Content: "new doc4",
	})

	// doc2 should be evicted
	if doc := idx.Get("doc2"); doc != nil {
		t.Error("doc2 should have been evicted")
	}

	// doc1 should exist and be updated
	doc1 := idx.Get("doc1")
	if doc1 == nil {
		t.Error("doc1 should exist")
	} else if doc1.Content != "updated doc1" {
		t.Errorf("doc1.Content = %q, want %q", doc1.Content, "updated doc1")
	}

	// doc3 and doc4 should exist
	for _, id := range []string{"doc3", "doc4"} {
		if doc := idx.Get(id); doc == nil {
			t.Errorf("%s should exist", id)
		}
	}
}

// TestEdgeCase_NoEmbedderProvided tests behavior when embedder is nil.
func TestEdgeCase_NoEmbedderProvided(t *testing.T) {
	idx := NewSemanticIndex(nil, nil)

	ctx := context.Background()

	// Add document with pre-computed embedding
	err := idx.Add(ctx, &Document{
		ID:      "doc1",
		Content: "content",
		Embedding: Embedding{0.1, 0.2, 0.3, 0.4},
	})
	if err != nil {
		t.Fatalf("Add with pre-computed embedding: %v", err)
	}

	// Search should fail (no embedder to embed query)
	_, err = idx.Search(ctx, "query", 5)
	if err == nil {
		t.Error("Search without embedder should return error")
	}

	// SearchByEmbedding should work
	results := idx.SearchByEmbedding(Embedding{0.1, 0.2, 0.3, 0.4}, 5)
	if len(results) != 1 {
		t.Errorf("SearchByEmbedding returned %d results, want 1", len(results))
	}
}

// TestEdgeCase_SearchEmptyIndex tests searching an empty index.
func TestEdgeCase_SearchEmptyIndex(t *testing.T) {
	embedder := &mockEmbedder{dimension: 8}
	idx := NewSemanticIndex(embedder, nil)

	ctx := context.Background()

	results, err := idx.Search(ctx, "query", 10)
	if err != nil {
		t.Fatalf("Search on empty index: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Search on empty index returned %d results, want 0", len(results))
	}
}

// TestEdgeCase_MissingDocumentID tests adding a document without an ID.
func TestEdgeCase_MissingDocumentID(t *testing.T) {
	embedder := &mockEmbedder{dimension: 8}
	idx := NewSemanticIndex(embedder, nil)

	ctx := context.Background()

	err := idx.Add(ctx, &Document{
		ID:      "",
		Content: "content without ID",
	})
	if err == nil {
		t.Error("Add without ID should return error")
	}
}
