package demokit

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

func TestCopyOSC52SequenceShape(t *testing.T) {
	var buf bytes.Buffer
	SetClipboardWriter(&buf)
	EnableShellClipboardFallback(false)
	defer func() {
		SetClipboardWriter(nil)
		EnableShellClipboardFallback(true)
	}()

	strategy, ok := Copy("hello, clipboard")
	if !ok {
		t.Fatalf("Copy returned ok=false; OSC 52 should always succeed when writer is a *bytes.Buffer")
	}
	if strategy != "osc52" {
		t.Errorf("strategy = %q, want %q", strategy, "osc52")
	}

	got := buf.String()
	if !strings.HasPrefix(got, "\x1b]52;c;") {
		t.Errorf("missing OSC 52 prefix: got %q", got)
	}
	if !strings.HasSuffix(got, "\x07") {
		t.Errorf("missing BEL terminator: got %q", got)
	}

	wantPayload := base64.StdEncoding.EncodeToString([]byte("hello, clipboard"))
	if !strings.Contains(got, wantPayload) {
		t.Errorf("base64 payload missing or wrong: got %q, want substring %q", got, wantPayload)
	}
}

func TestCopyWithoutTerminalFallsBackOrFails(t *testing.T) {
	// With shell fallback disabled and the OSC 52 writer accepting any
	// write, Copy succeeds via OSC 52 — confirming the strategy order
	// (OSC 52 tried first).
	var buf bytes.Buffer
	SetClipboardWriter(&buf)
	EnableShellClipboardFallback(false)
	defer func() {
		SetClipboardWriter(nil)
		EnableShellClipboardFallback(true)
	}()

	strategy, ok := Copy("payload")
	if !ok || strategy != "osc52" {
		t.Errorf("expected OSC 52 success with shell fallback disabled, got (%q, %v)", strategy, ok)
	}
}

// failingWriter always returns an error from Write — simulates a
// terminal that doesn't accept the OSC 52 sequence (no stderr, etc.).
type failingWriter struct{}

func (failingWriter) Write(p []byte) (int, error) {
	return 0, errFailingWriter
}

var errFailingWriter = &errString{"forced write failure"}

type errString struct{ s string }

func (e *errString) Error() string { return e.s }

func TestCopyOSC52WriteFailFallsThrough(t *testing.T) {
	// When OSC 52 write fails AND shell fallbacks are disabled, Copy
	// must return ("", false) without panicking.
	SetClipboardWriter(failingWriter{})
	EnableShellClipboardFallback(false)
	defer func() {
		SetClipboardWriter(nil)
		EnableShellClipboardFallback(true)
	}()

	strategy, ok := Copy("payload")
	if ok {
		t.Errorf("expected Copy to fail when OSC 52 writer errors and shell fallback is disabled, got strategy=%q", strategy)
	}
	if strategy != "" {
		t.Errorf("expected empty strategy on full failure, got %q", strategy)
	}
}
