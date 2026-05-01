package demokit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// TestStepContextCtxAlwaysNonNil verifies every Run sees a non-nil,
// not-yet-cancelled context, even on steps without Timeout or
// Cancellable. Run code that reads ctx.Ctx.Done() should never panic
// on a nil dereference; the background context only fires after Run
// returns (when demokit cancels it during cleanup).
func TestStepContextCtxAlwaysNonNil(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"test", "--non-interactive"}

	var ctxSawNonNil bool
	var ctxNotCancelledDuringRun bool
	demo := New("plain").WithRenderer(&recordingRenderer{})
	demo.Step("a").Run(func(ctx StepContext) *StepResult {
		ctxSawNonNil = ctx.Ctx != nil
		select {
		case <-ctx.Ctx.Done():
			ctxNotCancelledDuringRun = false
		default:
			ctxNotCancelledDuringRun = true
		}
		return nil
	})
	demo.Execute()

	if !ctxSawNonNil {
		t.Fatal("StepContext.Ctx was nil during Run for a step with no Timeout/Cancellable")
	}
	if !ctxNotCancelledDuringRun {
		t.Error("background ctx should not be cancelled during Run for a plain step")
	}
}

// TestStepTimeoutFiresContextDone verifies that a step with Timeout(d)
// causes ctx.Ctx to fire within d. A Run that selects on ctx.Done()
// returns when the timer fires; demokit then sees the captured result
// (or synthesizes an Info if Run returned nil).
func TestStepTimeoutFiresContextDone(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"test", "--non-interactive"}

	var ctxErr error
	demo := New("timeout").WithRenderer(&recordingRenderer{})
	demo.Step("watch").Timeout(50 * time.Millisecond).Run(func(ctx StepContext) *StepResult {
		<-ctx.Ctx.Done()
		ctxErr = ctx.Ctx.Err()
		return nil
	})

	start := time.Now()
	demo.Execute()
	elapsed := time.Since(start)

	if !errors.Is(ctxErr, context.DeadlineExceeded) {
		t.Errorf("ctx.Ctx.Err() = %v, want context.DeadlineExceeded", ctxErr)
	}
	if elapsed < 50*time.Millisecond {
		t.Errorf("Execute returned in %v, expected at least 50ms (the timeout)", elapsed)
	}
	if elapsed > 2*time.Second {
		t.Errorf("Execute took %v — timeout firing should be near-instant", elapsed)
	}
}

// TestStepTimeoutInfoResultWhenRunReturnsNil verifies that when a step
// times out and Run returns nil (no own result), demokit surfaces an
// Info result so the user sees why the step ended. A user-supplied
// result wins over the synthesized one.
func TestStepTimeoutInfoResultWhenRunReturnsNil(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"test", "--non-interactive"}

	rec := &MemoryRecorder{}
	demo := New("timeout-info").WithRenderer(&recordingRenderer{}).WithRecorder(rec)
	demo.Step("watch").ID("w").Timeout(20 * time.Millisecond).Run(func(ctx StepContext) *StepResult {
		<-ctx.Ctx.Done()
		return nil // no user result; demokit should synthesize an Info
	})
	demo.Execute()

	if len(rec.Entries) != 1 {
		t.Fatalf("expected 1 trace entry, got %d", len(rec.Entries))
	}
	e := rec.Entries[0]
	if e.Status != StatusInfo {
		t.Errorf("trace status = %v, want StatusInfo", e.Status)
	}
	if e.Message != "step timed out" {
		t.Errorf("trace message = %q, want %q", e.Message, "step timed out")
	}
}

// TestStepTimeoutUserResultWins verifies that a Run which returns its
// own *StepResult after noticing the cancel keeps that result —
// demokit does not overwrite a user-supplied result with the
// synthesized timeout Info.
func TestStepTimeoutUserResultWins(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"test", "--non-interactive"}

	rec := &MemoryRecorder{}
	demo := New("user-result").WithRenderer(&recordingRenderer{}).WithRecorder(rec)
	demo.Step("watch").ID("w").Timeout(20 * time.Millisecond).Run(func(ctx StepContext) *StepResult {
		<-ctx.Ctx.Done()
		return Info("custom finish")
	})
	demo.Execute()

	if rec.Entries[0].Message != "custom finish" {
		t.Errorf("user-supplied message overwritten: %q", rec.Entries[0].Message)
	}
}

// TestRunHonorsCancelDuringStreaming verifies that streaming output
// continues to flow through the chunk callback during the cancel
// branch — Run's cleanup-phase prints (e.g. "flushing...", "saved") are
// streamed live, not buffered into the next step's render.
func TestRunHonorsCancelDuringStreaming(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"test", "--non-interactive"}

	r := &streamingResultRenderer{}
	demo := New("flush").WithRenderer(r)
	demo.Step("watch").Timeout(20 * time.Millisecond).Run(func(ctx StepContext) *StepResult {
		fmt.Print("starting...\n")
		<-ctx.Ctx.Done()
		fmt.Print("flushing...\n")
		fmt.Print("done.\n")
		return nil
	})
	demo.Execute()

	r.mu.Lock()
	defer r.mu.Unlock()
	joined := ""
	for _, c := range r.chunks {
		joined += c
	}
	for _, want := range []string{"starting...", "flushing...", "done."} {
		if !strings.Contains(joined, want) {
			t.Errorf("streamed chunks missing %q; got: %q", want, joined)
		}
	}
}
