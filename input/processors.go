package input

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Processor processes input and returns Content.
type Processor interface {
	// Process processes the input value and returns Content.
	Process(ctx context.Context, value string) (*Content, error)

	// CanProcess returns true if this processor can handle the given type.
	CanProcess(typ Type) bool
}

// URLProcessor processes URL inputs.
type URLProcessor struct {
	client *http.Client
}

// NewURLProcessor creates a new URL processor.
func NewURLProcessor() *URLProcessor {
	return &URLProcessor{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Process fetches the URL and returns its content.
func (p *URLProcessor) Process(ctx context.Context, value string) (*Content, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, value, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	content := NewContent(TypeURL, value)
	content.Data = data
	content.Metadata["status_code"] = resp.StatusCode
	content.Metadata["content_type"] = resp.Header.Get("Content-Type")

	return content, nil
}

// CanProcess returns true for URL types.
func (p *URLProcessor) CanProcess(typ Type) bool {
	return typ == TypeURL
}

// FileProcessor processes file inputs.
type FileProcessor struct{}

// NewFileProcessor creates a new file processor.
func NewFileProcessor() *FileProcessor {
	return &FileProcessor{}
}

// Process reads the file and returns its content.
func (p *FileProcessor) Process(ctx context.Context, value string) (*Content, error) {
	// Expand tilde
	if strings.HasPrefix(value, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("expand home dir: %w", err)
		}
		value = filepath.Join(home, value[1:])
	}

	data, err := os.ReadFile(value)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	content := NewContent(TypeFile, value)
	content.Data = data

	// Add file metadata
	info, err := os.Stat(value)
	if err == nil {
		content.Metadata["size"] = info.Size()
		content.Metadata["mod_time"] = info.ModTime()
	}

	return content, nil
}

// CanProcess returns true for file types.
func (p *FileProcessor) CanProcess(typ Type) bool {
	return typ == TypeFile
}

// GlobProcessor processes glob pattern inputs.
type GlobProcessor struct {
	fileProcessor *FileProcessor
}

// NewGlobProcessor creates a new glob processor.
func NewGlobProcessor() *GlobProcessor {
	return &GlobProcessor{
		fileProcessor: NewFileProcessor(),
	}
}

// Process expands the glob pattern and reads matching files.
func (p *GlobProcessor) Process(ctx context.Context, value string) (*Content, error) {
	matches, err := filepath.Glob(value)
	if err != nil {
		return nil, fmt.Errorf("glob pattern: %w", err)
	}

	if len(matches) == 0 {
		return nil, fmt.Errorf("no files matched pattern: %s", value)
	}

	content := NewContent(TypeGlob, value)
	content.Metadata["matches"] = matches
	content.Metadata["count"] = len(matches)

	// Read all matching files
	var combined strings.Builder
	for i, match := range matches {
		fileContent, err := p.fileProcessor.Process(ctx, match)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", match, err)
		}

		if i > 0 {
			combined.WriteString("\n---\n")
		}
		fmt.Fprintf(&combined, "# %s\n", match)
		combined.Write(fileContent.Data)
	}

	content.Data = []byte(combined.String())
	return content, nil
}

// CanProcess returns true for glob types.
func (p *GlobProcessor) CanProcess(typ Type) bool {
	return typ == TypeGlob
}

// TextProcessor processes plain text inputs.
type TextProcessor struct{}

// NewTextProcessor creates a new text processor.
func NewTextProcessor() *TextProcessor {
	return &TextProcessor{}
}

// Process returns the text as-is.
func (p *TextProcessor) Process(ctx context.Context, value string) (*Content, error) {
	content := NewContent(TypeText, value)
	content.Data = []byte(value)
	return content, nil
}

// CanProcess returns true for text types.
func (p *TextProcessor) CanProcess(typ Type) bool {
	return typ == TypeText
}

// Registry manages processors for different input types.
type Registry struct {
	processors []Processor
}

// NewRegistry creates a new processor registry with default processors.
func NewRegistry() *Registry {
	return &Registry{
		processors: []Processor{
			NewURLProcessor(),
			NewFileProcessor(),
			NewGlobProcessor(),
			NewTextProcessor(),
		},
	}
}

// Register adds a custom processor to the registry.
func (r *Registry) Register(p Processor) {
	r.processors = append(r.processors, p)
}

// Process detects the input type and processes it with the appropriate processor.
func (r *Registry) Process(ctx context.Context, value string) (*Content, error) {
	typ := Detect(value)

	for _, p := range r.processors {
		if p.CanProcess(typ) {
			return p.Process(ctx, value)
		}
	}

	return nil, fmt.Errorf("no processor for type: %s", typ)
}
