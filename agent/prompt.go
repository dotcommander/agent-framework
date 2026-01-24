package agent

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"text/template"
)

// PromptRegistry manages prompt templates with embed + runtime override support.
// Priority: runtime (~/.config/<app>/prompts/) > embedded > error
//
// Example:
//
//	//go:embed prompts/*.prompt
//	var promptsFS embed.FS
//
//	prompts := agent.NewPromptRegistry("myapp", promptsFS)
//	text, err := prompts.Render("extraction", map[string]any{"input": data})
type PromptRegistry struct {
	appName   string
	embedded  embed.FS
	cache     map[string]*template.Template
	cacheMu   sync.RWMutex
	runtimeDir string
}

// NewPromptRegistry creates a prompt registry.
// appName is used for runtime config path: ~/.config/<appName>/prompts/
// embedded is the embed.FS containing *.prompt files.
func NewPromptRegistry(appName string, embedded embed.FS) *PromptRegistry {
	home, _ := os.UserHomeDir()
	return &PromptRegistry{
		appName:    appName,
		embedded:   embedded,
		cache:      make(map[string]*template.Template),
		runtimeDir: filepath.Join(home, ".config", appName, "prompts"),
	}
}

// Load retrieves a prompt template by name (without .prompt extension).
// Checks runtime directory first, then embedded.
func (r *PromptRegistry) Load(name string) (string, error) {
	// Try runtime first (allows user customization)
	if content, err := r.loadRuntime(name); err == nil {
		return content, nil
	}

	// Fall back to embedded
	return r.loadEmbedded(name)
}

// loadRuntime loads from ~/.config/<app>/prompts/<name>.prompt
func (r *PromptRegistry) loadRuntime(name string) (string, error) {
	path := filepath.Join(r.runtimeDir, name+".prompt")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// loadEmbedded loads from the embedded filesystem.
func (r *PromptRegistry) loadEmbedded(name string) (string, error) {
	// Try with prompts/ prefix (common embed pattern)
	paths := []string{
		name + ".prompt",
		"prompts/" + name + ".prompt",
	}

	for _, path := range paths {
		data, err := r.embedded.ReadFile(path)
		if err == nil {
			return strings.TrimSpace(string(data)), nil
		}
	}

	return "", fmt.Errorf("prompt not found: %s", name)
}

// MustLoad loads a prompt or panics. Use during init.
func (r *PromptRegistry) MustLoad(name string) string {
	content, err := r.Load(name)
	if err != nil {
		panic(fmt.Sprintf("MustLoad(%q): %v", name, err))
	}
	return content
}

// Render loads a prompt and executes it as a Go template.
//
// Example:
//
//	text, err := prompts.Render("extraction", map[string]any{
//	    "input": userInput,
//	    "maxItems": 10,
//	})
func (r *PromptRegistry) Render(name string, data any) (string, error) {
	tmpl, err := r.getTemplate(name)
	if err != nil {
		return "", err
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("render %q: %w", name, err)
	}

	return buf.String(), nil
}

// MustRender renders a prompt or panics.
func (r *PromptRegistry) MustRender(name string, data any) string {
	result, err := r.Render(name, data)
	if err != nil {
		panic(fmt.Sprintf("MustRender(%q): %v", name, err))
	}
	return result
}

// getTemplate loads and caches a parsed template.
func (r *PromptRegistry) getTemplate(name string) (*template.Template, error) {
	// Check cache
	r.cacheMu.RLock()
	if tmpl, ok := r.cache[name]; ok {
		r.cacheMu.RUnlock()
		return tmpl, nil
	}
	r.cacheMu.RUnlock()

	// Load and parse
	content, err := r.Load(name)
	if err != nil {
		return nil, err
	}

	tmpl, err := template.New(name).Parse(content)
	if err != nil {
		return nil, fmt.Errorf("parse template %q: %w", name, err)
	}

	// Cache
	r.cacheMu.Lock()
	r.cache[name] = tmpl
	r.cacheMu.Unlock()

	return tmpl, nil
}

// Format is a simpler alternative using fmt.Sprintf.
// Use when templates are overkill.
//
// Example:
//
//	text, err := prompts.Format("simple", userInput)
func (r *PromptRegistry) Format(name string, args ...any) (string, error) {
	content, err := r.Load(name)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(content, args...), nil
}

// MustFormat formats a prompt or panics.
func (r *PromptRegistry) MustFormat(name string, args ...any) string {
	result, err := r.Format(name, args...)
	if err != nil {
		panic(fmt.Sprintf("MustFormat(%q): %v", name, err))
	}
	return result
}

// List returns all available prompt names.
func (r *PromptRegistry) List() []string {
	names := make(map[string]struct{})

	// List embedded
	if entries, err := r.embedded.ReadDir("."); err == nil {
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".prompt") {
				names[strings.TrimSuffix(e.Name(), ".prompt")] = struct{}{}
			}
		}
	}
	// Also try prompts/ subdir
	if entries, err := r.embedded.ReadDir("prompts"); err == nil {
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".prompt") {
				names[strings.TrimSuffix(e.Name(), ".prompt")] = struct{}{}
			}
		}
	}

	// List runtime
	if entries, err := os.ReadDir(r.runtimeDir); err == nil {
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".prompt") {
				names[strings.TrimSuffix(e.Name(), ".prompt")] = struct{}{}
			}
		}
	}

	result := make([]string, 0, len(names))
	for name := range names {
		result = append(result, name)
	}
	return result
}

// SimplePrompt formats a prompt with a single input placeholder.
// Convenience for the common case of "template with one %s".
//
// Example:
//
//	prompt := agent.SimplePrompt("Summarize:\n\n%s", text)
func SimplePrompt(template string, input string) string {
	return fmt.Sprintf(template, input)
}
