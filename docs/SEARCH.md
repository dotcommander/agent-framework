# Semantic Search

Semantic search capabilities for finding relevant documents using embeddings.

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "github.com/dotcommander/agent/search"
)

func main() {
    ctx := context.Background()

    // Create index with embedder and chunker
    index := search.NewSemanticIndex(embedder, search.NewFixedSizeChunker(512, 64))

    // Add documents
    index.Add(ctx, &search.Document{
        ID:      "doc-1",
        Content: "Go is a statically typed, compiled language...",
        Metadata: map[string]any{
            "type": "tutorial",
            "lang": "go",
        },
    })

    // Search
    results, err := index.Search(ctx, "how to handle errors in Go", 5)
    if err != nil {
        panic(err)
    }

    for _, result := range results {
        fmt.Printf("%.2f: %s\n", result.Score, result.Document.ID)
    }
}
```

## Core Types

### Document

Represents a searchable document:

```go
type Document struct {
    ID        string           // Unique identifier
    Content   string           // Document text
    Metadata  map[string]any   // Custom metadata
    Embedding Embedding        // Vector representation
    Chunks    []*DocumentChunk // Split sections
}
```

### DocumentChunk

A section of a document:

```go
type DocumentChunk struct {
    ID         string    // Chunk identifier
    DocumentID string    // Parent document
    Content    string    // Chunk text
    StartPos   int       // Start position in original
    EndPos     int       // End position in original
    Embedding  Embedding // Vector representation
}
```

### SearchResult

Search results with relevance score:

```go
type SearchResult struct {
    Document   *Document      // Matched document
    Chunk      *DocumentChunk // Matched chunk (if chunked)
    Score      float64        // Relevance score (0.0-1.0)
    Highlights []string       // Matching excerpts
}
```

### Embedding

Vector representation:

```go
type Embedding []float64
```

## Embedding Provider

Implement this interface to generate embeddings:

```go
type EmbeddingProvider interface {
    Embed(ctx context.Context, text string) (Embedding, error)
    EmbedBatch(ctx context.Context, texts []string) ([]Embedding, error)
    Dimension() int
}
```

Example with OpenAI:

```go
type OpenAIEmbedder struct {
    client *openai.Client
}

func (e *OpenAIEmbedder) Embed(ctx context.Context, text string) (search.Embedding, error) {
    resp, err := e.client.CreateEmbeddings(ctx, openai.EmbeddingRequest{
        Model: openai.AdaEmbeddingV2,
        Input: []string{text},
    })
    if err != nil {
        return nil, err
    }
    return search.Embedding(resp.Data[0].Embedding), nil
}

func (e *OpenAIEmbedder) EmbedBatch(ctx context.Context, texts []string) ([]search.Embedding, error) {
    resp, err := e.client.CreateEmbeddings(ctx, openai.EmbeddingRequest{
        Model: openai.AdaEmbeddingV2,
        Input: texts,
    })
    if err != nil {
        return nil, err
    }

    embeddings := make([]search.Embedding, len(resp.Data))
    for i, d := range resp.Data {
        embeddings[i] = search.Embedding(d.Embedding)
    }
    return embeddings, nil
}

func (e *OpenAIEmbedder) Dimension() int {
    return 1536 // Ada v2 dimension
}
```

## SemanticIndex

### Creating an Index

```go
// With chunking
index := search.NewSemanticIndex(embedder, search.NewFixedSizeChunker(512, 64))

// Without chunking (embed full documents)
index := search.NewSemanticIndex(embedder, nil)
```

### Adding Documents

```go
// Add single document
err := index.Add(ctx, &search.Document{
    ID:      "doc-1",
    Content: "Document content here...",
})

// Add multiple documents
err := index.AddBatch(ctx, []*search.Document{doc1, doc2, doc3})
```

### Searching

```go
// Semantic search
results, err := index.Search(ctx, "query text", 10)

// Search by embedding vector
results := index.SearchByEmbedding(queryVector, 10)

// Keyword search (no embeddings needed)
results := index.KeywordSearch("error handling", 10)

// Hybrid search (combines semantic + keyword)
results, err := index.HybridSearch(ctx, "query", 10, 0.3)  // 30% keyword weight
```

### Managing Documents

```go
// Get document by ID
doc := index.Get("doc-1")

// Remove document
index.Remove("doc-1")

// Count documents
count := index.Count()

// Clear all
index.Clear()
```

## Chunking Strategies

### Fixed Size Chunker

Splits by character count with overlap:

```go
chunker := search.NewFixedSizeChunker(
    512,  // Chunk size (characters)
    64,   // Overlap between chunks
)
```

Example with 512 size, 64 overlap:
```
[-------- chunk 1 --------]
                    [-------- chunk 2 --------]
                                        [-------- chunk 3 --------]
```

### Sentence Chunker

Splits by sentences:

```go
chunker := search.NewSentenceChunker(
    10,  // Max sentences per chunk
    2,   // Overlap (sentences)
)
```

### Custom Chunker

Implement the interface:

```go
type Chunker interface {
    Chunk(doc *Document) []*DocumentChunk
}

// Example: Paragraph chunker
type ParagraphChunker struct{}

func (c *ParagraphChunker) Chunk(doc *search.Document) []*search.DocumentChunk {
    paragraphs := strings.Split(doc.Content, "\n\n")
    chunks := make([]*search.DocumentChunk, 0, len(paragraphs))

    for i, para := range paragraphs {
        if strings.TrimSpace(para) == "" {
            continue
        }
        chunks = append(chunks, &search.DocumentChunk{
            ID:         fmt.Sprintf("%s-para-%d", doc.ID, i),
            DocumentID: doc.ID,
            Content:    para,
        })
    }

    return chunks
}
```

## Hybrid Search

Combines semantic similarity with keyword matching:

```go
// 30% weight to keyword matching, 70% to semantic
results, err := index.HybridSearch(ctx, "Go error handling", 10, 0.3)
```

How it works:
1. Run semantic search (get top N*2 results)
2. Run keyword search (get top N*2 results)
3. Merge and rerank by weighted score
4. Return top N

## Code Search

Specialized index for code files:

```go
// Create code index
codeIndex := search.NewCodeIndex(embedder)

// Add code files
err := codeIndex.AddCode(ctx, "main.go", "go", `
package main

func main() {
    fmt.Println("Hello")
}
`)

// Search with language filter
results, err := codeIndex.SearchCode(ctx, "main function", 5, "go")
```

### Code Document

```go
type CodeDocument struct {
    *Document
    FilePath string       // File path
    Language string       // Programming language
    Symbols  []CodeSymbol // Functions, classes, etc.
}

type CodeSymbol struct {
    Name      string // Symbol name
    Kind      string // "function", "class", "method"
    StartLine int    // Start line
    EndLine   int    // End line
    Signature string // Function signature
}
```

## Distance Functions

### Cosine Similarity

Used by default for semantic search:

```go
// Built into SearchByEmbedding
score := cosineSimilarity(query, document)  // 0.0 to 1.0
```

### Euclidean Distance

For distance-based comparisons:

```go
distance := search.EuclideanDistance(embedding1, embedding2)
```

## Example: Document RAG System

```go
func buildRAGSystem(docs []string) (*search.SemanticIndex, error) {
    ctx := context.Background()

    // Create index
    index := search.NewSemanticIndex(
        openAIEmbedder,
        search.NewSentenceChunker(5, 1),
    )

    // Add all documents
    for i, content := range docs {
        doc := &search.Document{
            ID:      fmt.Sprintf("doc-%d", i),
            Content: content,
            Metadata: map[string]any{
                "index": i,
            },
        }

        if err := index.Add(ctx, doc); err != nil {
            return nil, fmt.Errorf("add doc %d: %w", i, err)
        }
    }

    return index, nil
}

func query(ctx context.Context, index *search.SemanticIndex, question string) (string, error) {
    // Find relevant chunks
    results, err := index.Search(ctx, question, 3)
    if err != nil {
        return "", err
    }

    // Build context from results
    var context strings.Builder
    for _, r := range results {
        if r.Chunk != nil {
            context.WriteString(r.Chunk.Content)
        } else {
            context.WriteString(r.Document.Content)
        }
        context.WriteString("\n\n")
    }

    // Use context to answer (call your LLM here)
    return answerWithContext(ctx, question, context.String())
}
```

## Example: Code Search Tool

```go
func createCodeSearchTool(embedder search.EmbeddingProvider) *tools.Tool {
    index := search.NewCodeIndex(embedder)

    return tools.NewTool(
        "search_code",
        "Search for code across the codebase",
        map[string]any{
            "type": "object",
            "properties": map[string]any{
                "query":    map[string]any{"type": "string"},
                "language": map[string]any{"type": "string"},
            },
            "required": []string{"query"},
        },
        func(ctx context.Context, input map[string]any) (any, error) {
            query := input["query"].(string)
            lang, _ := input["language"].(string)

            results, err := index.SearchCode(ctx, query, 5, lang)
            if err != nil {
                return nil, err
            }

            // Format results
            output := make([]map[string]any, len(results))
            for i, r := range results {
                output[i] = map[string]any{
                    "file":    r.Document.Metadata["file_path"],
                    "score":   r.Score,
                    "content": r.Chunk.Content,
                }
            }

            return output, nil
        },
    )
}
```

## Configuration

```go
config := &search.IndexConfig{
    ChunkSize:    512,  // Characters per chunk
    ChunkOverlap: 64,   // Overlap between chunks
    MaxChunks:    100,  // Max chunks per document
}
```

## Thread Safety

`SemanticIndex` and `CodeIndex` are thread-safe for concurrent reads and writes.

## Related Documentation

- [ARCHITECTURE.md](ARCHITECTURE.md) - Package overview
- [MCP.md](MCP.md) - Exposing search as a tool
- [AGENT-LOOP.md](AGENT-LOOP.md) - Using search in agent workflows
