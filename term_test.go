package demokit

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
)

// TestCaptureOutputCapturesStderr verifies that bytes written to
// os.Stderr inside a step's Run are captured the same way bytes
// written to os.Stdout are. The pre-streaming version of
// captureOutput only redirected stdout, so an `fmt.Fprintln(os.Stderr, ...)`
// or a default `log.Println` (which uses stderr) leaked straight to
// the terminal — different placement than `fmt.Println`. Now both
// flow through the same merged pipe.
func TestCaptureOutputCapturesStderr(t *testing.T) {
	out, _ := captureOutput(func(ctx StepContext) *StepResult {
		fmt.Fprintln(os.Stderr, "from stderr")
		return nil
	}, StepContext{}, nil)

	if !strings.Contains(out, "from stderr") {
		t.Errorf("captured output missing stderr text: %q", out)
	}
}

// TestCaptureOutputInterleavesStdoutStderr verifies the merged-pipe
// approach preserves the order of writes across stdout and stderr.
// A two-pass concat (read stdout fully, then stderr) would lose this;
// the single shared writer end keeps things in source order.
func TestCaptureOutputInterleavesStdoutStderr(t *testing.T) {
	out, _ := captureOutput(func(ctx StepContext) *StepResult {
		fmt.Fprint(os.Stdout, "a\n")
		fmt.Fprint(os.Stderr, "b\n")
		fmt.Fprint(os.Stdout, "c\n")
		return nil
	}, StepContext{}, nil)

	idxA := strings.Index(out, "a")
	idxB := strings.Index(out, "b")
	idxC := strings.Index(out, "c")
	if idxA < 0 || idxB < 0 || idxC < 0 {
		t.Fatalf("missing one of a/b/c in output: %q", out)
	}
	if !(idxA < idxB && idxB < idxC) {
		t.Errorf("interleaving lost — got order indexes a=%d b=%d c=%d in %q", idxA, idxB, idxC, out)
	}
}

// TestCaptureOutputOnChunkCallbackFires verifies onChunk is invoked
// with the bytes a step's Run produces, in roughly real time. Verifies
// the streaming primitive at the captureOutput level — independent of
// any renderer integration.
func TestCaptureOutputOnChunkCallbackFires(t *testing.T) {
	var (
		mu     sync.Mutex
		chunks []string
	)
	onChunk := func(chunk []byte) {
		mu.Lock()
		chunks = append(chunks, string(chunk))
		mu.Unlock()
	}

	out, _ := captureOutput(func(ctx StepContext) *StepResult {
		fmt.Print("hello ")
		fmt.Print("world")
		return nil
	}, StepContext{}, onChunk)

	mu.Lock()
	defer mu.Unlock()
	if len(chunks) == 0 {
		t.Fatalf("onChunk never fired; full output was %q", out)
	}
	joined := strings.Join(chunks, "")
	if joined != "hello world" {
		t.Errorf("chunks reassembled to %q, want %q", joined, "hello world")
	}
	if out != "hello world" {
		t.Errorf("captured buffer %q diverges from streamed chunks %q", out, joined)
	}
}

// streamingResultRenderer is the event-aware test spy: it captures
// every OutputChunk into the chunks slice so tests can assert what a
// step streamed during Run. lastOutput stays empty by design — the
// post-cleanup architecture delivers output exclusively as chunks
// (StepEnd carries status/message but no output text), so any test
// asserting on lastOutput is asserting on the no-double-print
// invariant.
type streamingResultRenderer struct {
	recordingRenderer
	mu         sync.Mutex
	chunks     []string
	lastOutput string
}

func newStreamingResultRenderer() *streamingResultRenderer {
	r := &streamingResultRenderer{}
	r.onChunk = func(_ int, chunk []byte) {
		r.mu.Lock()
		r.chunks = append(r.chunks, string(chunk))
		r.mu.Unlock()
	}
	return r
}

// TestExecuteStreamingChunksAreDelivered verifies a step's Run output
// flows to the renderer as OutputChunk events in roughly real time.
// The drain accumulates them; the assertion checks the full payload
// arrived. lastOutput stays empty because StepEnd carries no output
// (chunks are the only delivery path).
func TestExecuteStreamingChunksAreDelivered(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"test", "--non-interactive"}

	r := newStreamingResultRenderer()
	demo := New("Stream").WithRenderer(r)
	demo.Step("emit").ID("emit").Run(func(ctx StepContext) *StepResult {
		fmt.Print("live output")
		return nil
	})
	demo.Execute()

	r.mu.Lock()
	gotChunks := strings.Join(r.chunks, "")
	r.mu.Unlock()

	if gotChunks != "live output" {
		t.Errorf("streamed chunks = %q, want %q", gotChunks, "live output")
	}
	if r.lastOutput != "" {
		t.Errorf("StepEnd should carry no output text (chunks are the only delivery), got %q", r.lastOutput)
	}
}

// TestExecuteStreamingTraceCapturesFullOutput verifies that even when
// a step streams live, the trace recorder still gets the complete
// captured text — streaming tees into the renderer; it doesn't replace
// the buffer feeding the trace.
func TestExecuteStreamingTraceCapturesFullOutput(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"test", "--non-interactive"}

	rec := &MemoryRecorder{}
	r := &streamingResultRenderer{}
	demo := New("Tee").WithRenderer(r).WithRecorder(rec)
	demo.Step("emit").ID("emit").Run(func(ctx StepContext) *StepResult {
		fmt.Print("archive me")
		return nil
	})
	demo.Execute()

	if len(rec.Entries) != 1 {
		t.Fatalf("expected 1 trace entry, got %d", len(rec.Entries))
	}
	if rec.Entries[0].Output != "archive me" {
		t.Errorf("trace[0].Output = %q, want %q", rec.Entries[0].Output, "archive me")
	}
}

// Regression for issue 23: captureOutput's mutation of global
// os.Stdout / os.Stderr raced with concurrent readers (TermWidth,
// the originalStdout snapshot in Execute) — race-flagged by
// `go test -race ./web/`. Drive both sides in parallel under
// -race; the test passes when both are gated by stdoutMu, and
// fails (race detector) when they aren't.
func TestCaptureOutputDoesNotRaceWithTermWidth(t *testing.T) {
	const goroutines = 6
	const iters = 25
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for j := 0; j < iters; j++ {
				_, _ = captureOutput(func(ctx StepContext) *StepResult {
					fmt.Print("x")
					return nil
				}, StepContext{}, nil)
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < iters*4; j++ {
				_ = TermWidth()
			}
		}()
	}
	wg.Wait()
}
