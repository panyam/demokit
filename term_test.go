package demokit

import (
	"fmt"
	"io"
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

// streamingTestRenderer extends recordingRenderer with StreamOutput
// support. Used to verify the streaming dispatch path.
type streamingTestRenderer struct {
	recordingRenderer
	mu     sync.Mutex
	chunks []string
}

func (r *streamingTestRenderer) StreamOutput(_ int, chunk []byte, _ io.Writer) {
	r.mu.Lock()
	r.chunks = append(r.chunks, string(chunk))
	r.mu.Unlock()
}

// resultRecordingRenderer captures the output value RenderResult sees,
// so tests can assert the streaming path passes "" while the buffered
// path passes the full text.
type resultRecordingRenderer struct {
	recordingRenderer
	lastOutput string
}

func (r *resultRecordingRenderer) RenderResult(num int, out string, res *StepResult) {
	r.recordingRenderer.RenderResult(num, out, res)
	r.lastOutput = out
}

type streamingResultRenderer struct {
	resultRecordingRenderer
	mu     sync.Mutex
	chunks []string
}

func (r *streamingResultRenderer) StreamOutput(_ int, chunk []byte, _ io.Writer) {
	r.mu.Lock()
	r.chunks = append(r.chunks, string(chunk))
	r.mu.Unlock()
}

// TestExecuteStreamingRendererReceivesChunks verifies that when a
// renderer implements StreamingRenderer, a step's Run output is
// teed into StreamOutput in roughly real time AND RenderResult is
// invoked with output == "" so the body isn't double-printed.
func TestExecuteStreamingRendererReceivesChunks(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"test", "--non-interactive"}

	r := &streamingResultRenderer{}
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
		t.Errorf("RenderResult should receive empty output when streaming, got %q", r.lastOutput)
	}
}

// TestExecuteNonStreamingRendererBuffers verifies the legacy path
// for renderers that don't implement StreamingRenderer: the captured
// output flows in full to RenderResult, no streaming.
func TestExecuteNonStreamingRendererBuffers(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"test", "--non-interactive"}

	r := &resultRecordingRenderer{}
	demo := New("Buffer").WithRenderer(r)
	demo.Step("emit").ID("emit").Run(func(ctx StepContext) *StepResult {
		fmt.Print("buffered output")
		return nil
	})
	demo.Execute()

	if r.lastOutput != "buffered output" {
		t.Errorf("buffered RenderResult output = %q, want %q", r.lastOutput, "buffered output")
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
