package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// InputSource represents where input comes from.
type InputSource int

const (
	// InputSourceArgs indicates input from command-line arguments.
	InputSourceArgs InputSource = iota

	// InputSourceStdin indicates input from stdin.
	InputSourceStdin

	// InputSourceFile indicates input from a file.
	InputSourceFile
)

// ResolveInput resolves input from args, stdin, or file.
// Priority: args > stdin > file
func ResolveInput(args []string, stdinFile string) (string, InputSource, error) {
	// Check args first
	if len(args) > 0 {
		return strings.Join(args, " "), InputSourceArgs, nil
	}

	// Check stdin
	stat, err := os.Stdin.Stat()
	if err == nil && (stat.Mode()&os.ModeCharDevice) == 0 {
		// stdin is piped
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", InputSourceStdin, fmt.Errorf("read stdin: %w", err)
		}
		content := strings.TrimSpace(string(data))
		if content != "" {
			return content, InputSourceStdin, nil
		}
	}

	// Check file
	if stdinFile != "" {
		data, err := os.ReadFile(stdinFile)
		if err != nil {
			return "", InputSourceFile, fmt.Errorf("read file %s: %w", stdinFile, err)
		}
		return string(data), InputSourceFile, nil
	}

	return "", InputSourceArgs, fmt.Errorf("no input provided")
}
