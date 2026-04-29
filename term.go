package demokit

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/charmbracelet/x/term"
)

// TermWidth returns the current terminal width, or 80 as fallback.
// Tries stdout, then stderr (which stays connected even when stdout is piped).
func TermWidth() int {
	for _, f := range []*os.File{os.Stdout, os.Stderr} {
		if w, _, err := term.GetSize(f.Fd()); err == nil && w > 0 {
			return w
		}
	}
	return 80
}

// isTerminal returns true if stdin appears to be an interactive terminal.
func isTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// captureOutput redirects stdout while fn runs and returns what was written.
// Panics are recovered and converted to a StepResult with StatusError.
func captureOutput(fn func(StepContext) *StepResult, ctx StepContext) (output string, result *StepResult) {
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		// If pipe fails, just run normally — don't block the demo.
		result = fn(ctx)
		return "", result
	}
	os.Stdout = w

	done := make(chan string)
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		done <- buf.String()
	}()

	func() {
		defer func() {
			if rec := recover(); rec != nil {
				result = &StepResult{
					Status:  StatusError,
					Message: fmt.Sprintf("panic: %v", rec),
				}
			}
		}()
		result = fn(ctx)
	}()

	w.Close()
	os.Stdout = old
	output = <-done
	return output, result
}
