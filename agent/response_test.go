package agent

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestResponse_String(t *testing.T) {
	tests := []struct {
		name     string
		response *Response
		want     string
	}{
		{
			name:     "normal content",
			response: &Response{Content: "hello world"},
			want:     "hello world",
		},
		{
			name:     "empty content",
			response: &Response{Content: ""},
			want:     "",
		},
		{
			name:     "nil response",
			response: nil,
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.response.String()
			if got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResponse_JSON(t *testing.T) {
	type testStruct struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	tests := []struct {
		name      string
		response  *Response
		wantName  string
		wantValue int
		wantErr   bool
	}{
		{
			name:      "valid JSON",
			response:  &Response{Content: `{"name":"test","value":42}`},
			wantName:  "test",
			wantValue: 42,
			wantErr:   false,
		},
		{
			name:     "invalid JSON",
			response: &Response{Content: "not json"},
			wantErr:  true,
		},
		{
			name:     "empty content",
			response: &Response{Content: ""},
			wantErr:  true,
		},
		{
			name:     "nil response",
			response: nil,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result testStruct
			err := tt.response.JSON(&result)
			if (err != nil) != tt.wantErr {
				t.Errorf("JSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if result.Name != tt.wantName {
					t.Errorf("JSON() name = %q, want %q", result.Name, tt.wantName)
				}
				if result.Value != tt.wantValue {
					t.Errorf("JSON() value = %d, want %d", result.Value, tt.wantValue)
				}
			}
		})
	}
}

func TestResponse_SaveTo(t *testing.T) {
	tests := []struct {
		name     string
		response *Response
		want     string
	}{
		{
			name:     "normal content",
			response: &Response{Content: "file content here"},
			want:     "file content here",
		},
		{
			name:     "empty content",
			response: &Response{Content: ""},
			want:     "",
		},
		{
			name:     "nil response",
			response: nil,
			want:     "",
		},
		{
			name:     "multiline content",
			response: &Response{Content: "line1\nline2\nline3"},
			want:     "line1\nline2\nline3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "output.txt")

			err := tt.response.SaveTo(path)
			if err != nil {
				t.Fatalf("SaveTo() error = %v", err)
			}

			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("ReadFile() error = %v", err)
			}

			if string(content) != tt.want {
				t.Errorf("SaveTo() wrote %q, want %q", string(content), tt.want)
			}
		})
	}
}

func TestResponse_Lines(t *testing.T) {
	tests := []struct {
		name     string
		response *Response
		want     []string
	}{
		{
			name:     "multiple lines",
			response: &Response{Content: "line1\nline2\nline3"},
			want:     []string{"line1", "line2", "line3"},
		},
		{
			name:     "single line",
			response: &Response{Content: "single"},
			want:     []string{"single"},
		},
		{
			name:     "empty content",
			response: &Response{Content: ""},
			want:     nil,
		},
		{
			name:     "nil response",
			response: nil,
			want:     nil,
		},
		{
			name:     "trailing newline",
			response: &Response{Content: "line1\nline2\n"},
			want:     []string{"line1", "line2", ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.response.Lines()
			if len(got) != len(tt.want) {
				t.Errorf("Lines() = %v (len %d), want %v (len %d)", got, len(got), tt.want, len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("Lines()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestResponse_Contains(t *testing.T) {
	tests := []struct {
		name     string
		response *Response
		substr   string
		want     bool
	}{
		{
			name:     "contains substring",
			response: &Response{Content: "hello world"},
			substr:   "world",
			want:     true,
		},
		{
			name:     "does not contain",
			response: &Response{Content: "hello world"},
			substr:   "foo",
			want:     false,
		},
		{
			name:     "empty substring",
			response: &Response{Content: "hello"},
			substr:   "",
			want:     true,
		},
		{
			name:     "nil response",
			response: nil,
			substr:   "test",
			want:     false,
		},
		{
			name:     "case sensitive",
			response: &Response{Content: "Hello"},
			substr:   "hello",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.response.Contains(tt.substr)
			if got != tt.want {
				t.Errorf("Contains(%q) = %v, want %v", tt.substr, got, tt.want)
			}
		})
	}
}

func TestResponse_IsEmpty(t *testing.T) {
	tests := []struct {
		name     string
		response *Response
		want     bool
	}{
		{
			name:     "empty string",
			response: &Response{Content: ""},
			want:     true,
		},
		{
			name:     "whitespace only",
			response: &Response{Content: "   \t\n  "},
			want:     true,
		},
		{
			name:     "has content",
			response: &Response{Content: "hello"},
			want:     false,
		},
		{
			name:     "nil response",
			response: nil,
			want:     true,
		},
		{
			name:     "whitespace with text",
			response: &Response{Content: "  hello  "},
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.response.IsEmpty()
			if got != tt.want {
				t.Errorf("IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResponse_Bytes(t *testing.T) {
	tests := []struct {
		name     string
		response *Response
		want     []byte
	}{
		{
			name:     "normal content",
			response: &Response{Content: "hello"},
			want:     []byte("hello"),
		},
		{
			name:     "empty content",
			response: &Response{Content: ""},
			want:     []byte(""),
		},
		{
			name:     "nil response",
			response: nil,
			want:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.response.Bytes()
			if tt.want == nil {
				if got != nil {
					t.Errorf("Bytes() = %v, want nil", got)
				}
				return
			}
			if string(got) != string(tt.want) {
				t.Errorf("Bytes() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResponse_HasToolCalls(t *testing.T) {
	tests := []struct {
		name     string
		response *Response
		want     bool
	}{
		{
			name: "has tool calls",
			response: &Response{
				ToolCalls: []ToolCall{
					{Name: "test_tool"},
				},
			},
			want: true,
		},
		{
			name: "empty tool calls",
			response: &Response{
				ToolCalls: []ToolCall{},
			},
			want: false,
		},
		{
			name:     "nil tool calls",
			response: &Response{},
			want:     false,
		},
		{
			name:     "nil response",
			response: nil,
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.response.HasToolCalls()
			if got != tt.want {
				t.Errorf("HasToolCalls() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResponse_ToolCallsByName(t *testing.T) {
	sampleErr := errors.New("tool error")

	tests := []struct {
		name      string
		response  *Response
		toolName  string
		wantCount int
		wantNames []string
	}{
		{
			name: "single match",
			response: &Response{
				ToolCalls: []ToolCall{
					{Name: "read_file", Input: map[string]any{"path": "/tmp/a"}},
					{Name: "write_file", Input: map[string]any{"path": "/tmp/b"}},
				},
			},
			toolName:  "read_file",
			wantCount: 1,
			wantNames: []string{"read_file"},
		},
		{
			name: "multiple matches",
			response: &Response{
				ToolCalls: []ToolCall{
					{Name: "read_file", Input: map[string]any{"path": "/tmp/a"}},
					{Name: "read_file", Input: map[string]any{"path": "/tmp/b"}},
					{Name: "write_file", Input: map[string]any{"path": "/tmp/c"}},
				},
			},
			toolName:  "read_file",
			wantCount: 2,
			wantNames: []string{"read_file", "read_file"},
		},
		{
			name: "no match",
			response: &Response{
				ToolCalls: []ToolCall{
					{Name: "write_file"},
				},
			},
			toolName:  "read_file",
			wantCount: 0,
			wantNames: nil,
		},
		{
			name:      "nil response",
			response:  nil,
			toolName:  "read_file",
			wantCount: 0,
			wantNames: nil,
		},
		{
			name: "with error",
			response: &Response{
				ToolCalls: []ToolCall{
					{Name: "failing_tool", Error: sampleErr},
				},
			},
			toolName:  "failing_tool",
			wantCount: 1,
			wantNames: []string{"failing_tool"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.response.ToolCallsByName(tt.toolName)
			if len(got) != tt.wantCount {
				t.Errorf("ToolCallsByName(%q) returned %d results, want %d", tt.toolName, len(got), tt.wantCount)
				return
			}
			for i, tc := range got {
				if tc.Name != tt.wantNames[i] {
					t.Errorf("ToolCallsByName(%q)[%d].Name = %q, want %q", tt.toolName, i, tc.Name, tt.wantNames[i])
				}
			}
		})
	}
}
