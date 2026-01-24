// Package main demonstrates semantic search capabilities.
//
// The search package provides document indexing and search functionality.
// This example shows keyword search (no embedder required) and demonstrates
// the semantic search API structure.
package main

import (
	"context"
	"fmt"

	"github.com/dotcommander/agent/search"
)

func main() {
	fmt.Println("=== Semantic Search Demo ===")
	fmt.Println()

	// Create a semantic index without an embedder (for keyword search demo)
	// In production, you would provide an EmbeddingProvider for semantic search
	idx := search.NewSemanticIndex(nil, nil)

	// Add sample documents
	documents := []*search.Document{
		{
			ID:      "doc-1",
			Content: "Go is a statically typed, compiled programming language designed at Google. It is syntactically similar to C, but with memory safety, garbage collection, and structural typing.",
			Metadata: map[string]any{
				"category": "programming",
				"language": "en",
				"source":   "documentation",
			},
		},
		{
			ID:      "doc-2",
			Content: "Concurrency in Go is achieved through goroutines and channels. Goroutines are lightweight threads managed by the Go runtime. Channels provide a way for goroutines to communicate with each other.",
			Metadata: map[string]any{
				"category": "concurrency",
				"language": "en",
				"topic":    "goroutines",
			},
		},
		{
			ID:      "doc-3",
			Content: "Error handling in Go uses explicit return values rather than exceptions. Functions return an error as the last return value, which the caller must check and handle.",
			Metadata: map[string]any{
				"category": "best-practices",
				"language": "en",
				"topic":    "errors",
			},
		},
		{
			ID:      "doc-4",
			Content: "The context package provides a way to carry deadlines, cancellation signals, and request-scoped values across API boundaries and between processes.",
			Metadata: map[string]any{
				"category": "stdlib",
				"package":  "context",
			},
		},
		{
			ID:      "doc-5",
			Content: "Interfaces in Go are satisfied implicitly. A type implements an interface by implementing its methods. There is no explicit declaration of intent.",
			Metadata: map[string]any{
				"category": "types",
				"topic":    "interfaces",
			},
		},
		{
			ID:      "doc-6",
			Content: "Testing in Go is built into the language. The testing package provides support for automated testing of Go packages. Test files are named with _test.go suffix.",
			Metadata: map[string]any{
				"category": "testing",
				"package":  "testing",
			},
		},
		{
			ID:      "doc-7",
			Content: "Go modules are the dependency management system in Go. The go.mod file defines the module path and tracks dependencies with their versions.",
			Metadata: map[string]any{
				"category": "modules",
				"files":    []string{"go.mod", "go.sum"},
			},
		},
	}

	// Add documents to index
	fmt.Println("Indexing documents...")
	ctx := context.Background()
	for _, doc := range documents {
		// Note: Add() without embedder will skip embedding generation
		if err := idx.Add(ctx, doc); err != nil {
			fmt.Printf("Error adding %s: %v\n", doc.ID, err)
		} else {
			fmt.Printf("  Added: %s\n", doc.ID)
		}
	}
	fmt.Printf("\nTotal documents indexed: %d\n\n", idx.Count())

	// Perform keyword searches
	fmt.Println("=== Keyword Search ===")
	fmt.Println()

	queries := []string{
		"goroutines channels",
		"error handling",
		"testing Go",
		"modules dependencies",
		"interface methods",
	}

	for _, query := range queries {
		fmt.Printf("Query: \"%s\"\n", query)

		results := idx.KeywordSearch(query, 3)

		if len(results) == 0 {
			fmt.Println("  No results found")
		} else {
			for i, r := range results {
				preview := r.Document.Content
				if len(preview) > 80 {
					preview = preview[:80] + "..."
				}
				fmt.Printf("  %d. [%.3f] %s: %s\n", i+1, r.Score, r.Document.ID, preview)
			}
		}
		fmt.Println()
	}

	// Demonstrate chunking
	fmt.Println("=== Document Chunking ===")
	fmt.Println()

	// Create a fixed-size chunker
	chunker := search.NewFixedSizeChunker(100, 20)

	longDoc := &search.Document{
		ID:      "long-doc",
		Content: "This is a longer document that will be split into multiple chunks. Each chunk overlaps with the next to preserve context. This helps ensure that important information at chunk boundaries is not lost during processing. The chunking process is essential for handling large documents in embedding-based search systems.",
	}

	chunks := chunker.Chunk(ctx, longDoc)
	fmt.Printf("Original document: %d characters\n", len(longDoc.Content))
	fmt.Printf("Chunked into: %d chunks\n\n", len(chunks))

	for i, chunk := range chunks {
		fmt.Printf("Chunk %d (pos %d-%d):\n", i+1, chunk.StartPos, chunk.EndPos)
		fmt.Printf("  \"%s\"\n\n", chunk.Content)
	}

	// Demonstrate sentence chunker
	fmt.Println("=== Sentence Chunking ===")
	fmt.Println()

	sentenceChunker := search.NewSentenceChunker(2, 1) // 2 sentences per chunk, 1 overlap

	sentenceDoc := &search.Document{
		ID:      "sentence-doc",
		Content: "First sentence here. Second sentence follows. Third sentence now. Fourth sentence appears. Fifth and final sentence.",
	}

	sentenceChunks := sentenceChunker.Chunk(ctx, sentenceDoc)
	fmt.Printf("Sentence chunks (%d):\n", len(sentenceChunks))
	for i, chunk := range sentenceChunks {
		fmt.Printf("  %d: %s\n", i+1, chunk.Content)
	}
	fmt.Println()

	// Demonstrate document retrieval
	fmt.Println("=== Document Retrieval ===")
	fmt.Println()

	doc := idx.Get("doc-2")
	if doc != nil {
		fmt.Printf("Retrieved: %s\n", doc.ID)
		fmt.Printf("  Content: %s\n", doc.Content[:50]+"...")
		fmt.Printf("  Metadata: %v\n", doc.Metadata)
	}
	fmt.Println()

	// Demonstrate removal
	fmt.Println("=== Document Removal ===")
	fmt.Println()
	fmt.Printf("Count before removal: %d\n", idx.Count())
	idx.Remove("doc-7")
	fmt.Printf("Count after removing doc-7: %d\n", idx.Count())

	// Demonstrate semantic search API (would require embedder)
	fmt.Println()
	fmt.Println("=== Semantic Search API (requires EmbeddingProvider) ===")
	fmt.Println()
	fmt.Println("To enable semantic search, implement EmbeddingProvider interface:")
	fmt.Println()
	fmt.Println("  type MyEmbedder struct { ... }")
	fmt.Println()
	fmt.Println("  func (e *MyEmbedder) Embed(ctx context.Context, text string) (search.Embedding, error) {")
	fmt.Println("      // Call your embedding API (OpenAI, Cohere, local model, etc.)")
	fmt.Println("      return embedding, nil")
	fmt.Println("  }")
	fmt.Println()
	fmt.Println("  func (e *MyEmbedder) EmbedBatch(ctx context.Context, texts []string) ([]search.Embedding, error) {")
	fmt.Println("      // Batch embedding for efficiency")
	fmt.Println("      return embeddings, nil")
	fmt.Println("  }")
	fmt.Println()
	fmt.Println("  func (e *MyEmbedder) Dimension() int {")
	fmt.Println("      return 1536 // e.g., OpenAI ada-002")
	fmt.Println("  }")
	fmt.Println()
	fmt.Println("Then create index with embedder:")
	fmt.Println("  idx := search.NewSemanticIndex(myEmbedder, chunker)")
	fmt.Println()
	fmt.Println("And perform semantic search:")
	fmt.Println("  results, err := idx.Search(ctx, \"query\", 10)")
	fmt.Println("  // or hybrid search:")
	fmt.Println("  results, err := idx.HybridSearch(ctx, \"query\", 10, 0.3) // 30% keyword weight")

	// Demonstrate code index
	fmt.Println()
	fmt.Println("=== Code Index ===")
	fmt.Println()
	fmt.Println("For code-specific search, use CodeIndex:")
	fmt.Println()
	fmt.Println("  codeIdx := search.NewCodeIndex(embedder)")
	fmt.Println("  codeIdx.AddCode(ctx, \"main.go\", \"go\", content)")
	fmt.Println("  results, _ := codeIdx.SearchCode(ctx, \"concurrency\", 10, \"go\")")
}
