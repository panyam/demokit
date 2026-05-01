package demokit

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/charmbracelet/x/term"
	"github.com/muesli/cancelreader"
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

// watchCancelKey starts a stdin reader in a goroutine; when the user
// presses Enter, it calls cancel. Returns a stop function the caller
// must invoke when the run ends (whether the watcher fired or not).
//
// Uses muesli/cancelreader so the goroutine doesn't leak across
// successive steps — the same pattern WaitForEnterOrTimeout uses.
// On platforms where cancelreader fails (rare), the watcher falls
// back to a no-op so the demo still progresses.
func watchCancelKey(cancel context.CancelFunc) (stop func()) {
	cr, err := cancelreader.NewReader(os.Stdin)
	if err != nil {
		return func() {}
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = bufio.NewReader(cr).ReadString('\n')
		cancel()
	}()
	return func() {
		cr.Cancel()
		<-done
		_ = cr.Close()
	}
}

// cancelReason maps a context.Err() to a user-visible message.
func cancelReason(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "step timed out"
	case errors.Is(err, context.Canceled):
		return "step cancelled"
	default:
		return err.Error()
	}
}

// captureOutput runs fn with both os.Stdout AND os.Stderr redirected
// through a single pipe so a step's prints — wherever they go —
// land in the rendered demo body. Returns the captured output and
// the StepResult; panics in fn become a StatusError result.
//
// If onChunk is non-nil, it's invoked for each byte chunk drained
// from the pipe in roughly real time. Renderers that implement
// StreamingRenderer hook in here so the user sees output as it's
// produced rather than waiting for fn to return. The captured
// output (returned string) still contains everything regardless of
// onChunk — the chunk callback tees, it doesn't replace the buffer.
func captureOutput(fn func(StepContext) *StepResult, ctx StepContext, onChunk func([]byte)) (output string, result *StepResult) {
	oldOut, oldErr := os.Stdout, os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		// If pipe fails, just run normally — don't block the demo.
		result = fn(ctx)
		return "", result
	}
	os.Stdout = w
	os.Stderr = w

	done := make(chan string)
	go func() {
		var buf bytes.Buffer
		// Read in chunks so onChunk fires in something like real time.
		// io.Copy alone would also work, but it gives us no hook to
		// observe each block as it arrives.
		raw := make([]byte, 4096)
		for {
			n, err := r.Read(raw)
			if n > 0 {
				buf.Write(raw[:n])
				if onChunk != nil {
					// Copy the slice — the callback may outlive this
					// loop iteration and `raw` is reused on the next read.
					cp := make([]byte, n)
					copy(cp, raw[:n])
					onChunk(cp)
				}
			}
			if err != nil {
				if err != io.EOF {
					_ = err // pipe closed; surface nothing
				}
				break
			}
		}
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
	os.Stdout = oldOut
	os.Stderr = oldErr
	output = <-done
	return output, result
}
