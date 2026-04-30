package demokit

import (
	"io"
	"os"
	"strings"
	"testing"
)

// captureStdoutStderr swaps os.Stdout and os.Stderr for pipes during fn,
// then returns whatever was written. Used to assert CLI flag dispatch
// produces the right output and warnings.
func captureStdoutStderr(t *testing.T, fn func()) (stdout, stderr string) {
	t.Helper()
	origOut, origErr := os.Stdout, os.Stderr

	rOut, wOut, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe stdout: %v", err)
	}
	rErr, wErr, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe stderr: %v", err)
	}
	os.Stdout = wOut
	os.Stderr = wErr

	defer func() {
		os.Stdout = origOut
		os.Stderr = origErr
	}()

	fn()
	wOut.Close()
	wErr.Close()

	outBytes, _ := io.ReadAll(rOut)
	errBytes, _ := io.ReadAll(rErr)
	return string(outBytes), string(errBytes)
}

// TestEmitDocMDStaticDispatchesToMarkdown verifies that --doc md without
// --from routes to the rich Demo.Markdown() static visitor (which emits
// a sequenceDiagram block) rather than to the per-entry trace renderer
// (which would emit "## Walkthrough"). Pins the static-vs-trace split.
func TestEmitDocMDStaticDispatchesToMarkdown(t *testing.T) {
	d := New("Static MD").Description("desc").
		Actors(Actor("A", "A"), Actor("B", "B"))
	d.Step("step").Arrow("A", "B", "msg").Note("note")

	out, _ := captureStdoutStderr(t, func() { d.emitDoc("md", "") })

	if !strings.Contains(out, "sequenceDiagram") {
		t.Errorf("--doc md (static) should call Demo.Markdown() — expected sequenceDiagram block:\n%s", out)
	}
	if strings.Contains(out, "## Walkthrough") {
		t.Errorf("--doc md (static) should not produce trace-renderer output (## Walkthrough):\n%s", out)
	}
}

// TestEmitDocMDTraceDispatchesToTraceRenderer verifies --doc md --from
// routes to RenderDocumentMD (per-entry layered renderer with a
// "## Walkthrough" header), not to the static visitor.
func TestEmitDocMDTraceDispatchesToTraceRenderer(t *testing.T) {
	tmp, err := os.CreateTemp("", "demokit-trace-*.json")
	if err != nil {
		t.Fatal(err)
	}
	tmp.Close()
	defer os.Remove(tmp.Name())

	rec := NewJSONFileRecorder(tmp.Name())
	rec.Record(TraceEntry{Kind: KindStep, Title: "first", StepID: "x", Visit: 1})
	if err := rec.Close(); err != nil {
		t.Fatal(err)
	}

	d := New("Trace MD").Description("desc")
	d.Step("first").ID("x")

	out, _ := captureStdoutStderr(t, func() { d.emitDoc("md", tmp.Name()) })

	if !strings.Contains(out, "## Walkthrough") {
		t.Errorf("--doc md --from should produce trace-renderer output (## Walkthrough):\n%s", out)
	}
	if strings.Contains(out, "sequenceDiagram") {
		t.Errorf("--doc md --from should not produce static-visitor output (sequenceDiagram):\n%s", out)
	}
}

// TestEmitDocFormatsProduceNonEmpty exercises the format×mode matrix —
// every (md, html, json) × (static, trace) combination must produce
// non-empty, format-recognizable output. Catches a wholesale dispatch
// regression.
func TestEmitDocFormatsProduceNonEmpty(t *testing.T) {
	tmp, err := os.CreateTemp("", "demokit-trace-*.json")
	if err != nil {
		t.Fatal(err)
	}
	tmp.Close()
	defer os.Remove(tmp.Name())
	rec := NewJSONFileRecorder(tmp.Name())
	rec.Record(TraceEntry{Kind: KindStep, Title: "x", StepID: "x", Visit: 1})
	if err := rec.Close(); err != nil {
		t.Fatal(err)
	}

	// No actors declared on purpose — exercises the static md visitor's
	// no-actors path (issue 6 regression).
	d := New("Matrix").Description("desc")
	d.Step("x").ID("x").Note("a note")

	cases := []struct {
		name   string
		format string
		from   string
		marker string // substring that must appear in output
	}{
		{"md-static", "md", "", "# Matrix"},
		{"md-trace", "md", tmp.Name(), "## Walkthrough"},
		{"html-static", "html", "", "<!doctype html>"},
		{"html-trace", "html", tmp.Name(), "<!doctype html>"},
		{"json-static", "json", "", "\"demo\""},
		{"json-trace", "json", tmp.Name(), "\"trace\""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := captureStdoutStderr(t, func() { d.emitDoc(tc.format, tc.from) })
			if out == "" {
				t.Fatalf("empty output for %s", tc.name)
			}
			if !strings.Contains(out, tc.marker) {
				t.Errorf("%s output missing %q:\n%s", tc.name, tc.marker, out)
			}
		})
	}
}

// TestEmitDocUnknownFormatErrors verifies an unknown --doc value writes
// a descriptive error to stderr and produces no stdout. Silent empty
// output here would be a bad UX trap.
func TestEmitDocUnknownFormatErrors(t *testing.T) {
	d := New("X")

	out, errOut := captureStdoutStderr(t, func() { d.emitDoc("yaml", "") })

	if out != "" {
		t.Errorf("unknown format should produce no stdout, got: %s", out)
	}
	if !strings.Contains(errOut, "unknown --doc format") {
		t.Errorf("expected stderr to describe unknown format, got: %s", errOut)
	}
}

// TestEmitDocFromMissingFileErrors verifies a non-existent --from path
// surfaces a stderr error and produces no stdout.
func TestEmitDocFromMissingFileErrors(t *testing.T) {
	d := New("X")

	out, errOut := captureStdoutStderr(t, func() { d.emitDoc("md", "/no/such/path.json") })

	if out != "" {
		t.Errorf("missing trace file should produce no stdout, got: %s", out)
	}
	if !strings.Contains(errOut, "--from") {
		t.Errorf("expected stderr to mention --from, got: %s", errOut)
	}
}

// TestExecuteDeprecatedReadmeWarns verifies invoking the legacy --readme
// flag prints a deprecation warning to stderr while still producing
// the expected static markdown on stdout. Same shape applies to
// --readme-from and --readme-html-from.
func TestExecuteDeprecatedReadmeWarns(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"test", "--readme"}

	d := New("Legacy").Description("d").Actors(Actor("A", "A"))
	d.Step("only").Arrow("A", "A", "self")

	out, errOut := captureStdoutStderr(t, func() { d.Execute() })

	if !strings.Contains(out, "# Legacy") {
		t.Errorf("--readme should still produce static markdown:\n%s", out)
	}
	if !strings.Contains(errOut, "deprecated") || !strings.Contains(errOut, "--doc md") {
		t.Errorf("expected deprecation warning pointing at --doc md, got stderr:\n%s", errOut)
	}
}

// TestExecuteDeprecatedReadmeFromWarns is the trace-md variant of the
// deprecation-warning test.
func TestExecuteDeprecatedReadmeFromWarns(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()

	tmp, err := os.CreateTemp("", "demokit-trace-*.json")
	if err != nil {
		t.Fatal(err)
	}
	tmp.Close()
	defer os.Remove(tmp.Name())
	rec := NewJSONFileRecorder(tmp.Name())
	rec.Record(TraceEntry{Kind: KindStep, Title: "x", StepID: "x", Visit: 1})
	if err := rec.Close(); err != nil {
		t.Fatal(err)
	}

	os.Args = []string{"test", "--readme-from", tmp.Name()}
	d := New("Legacy")
	d.Step("x").ID("x")

	out, errOut := captureStdoutStderr(t, func() { d.Execute() })

	if !strings.Contains(out, "## Walkthrough") {
		t.Errorf("--readme-from should still emit trace markdown:\n%s", out)
	}
	if !strings.Contains(errOut, "deprecated") || !strings.Contains(errOut, "--doc md --from") {
		t.Errorf("expected deprecation warning pointing at --doc md --from, got stderr:\n%s", errOut)
	}
}

// TestExecuteDocFlagDispatch verifies the new --doc md flag plumbing
// works end-to-end through Execute (covers scanOwnArgs + Execute
// dispatch glue, not just emitDoc).
func TestExecuteDocFlagDispatch(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"test", "--doc", "json"}

	d := New("DocFlag").Description("d")
	d.Step("a").ID("a")

	out, errOut := captureStdoutStderr(t, func() { d.Execute() })

	if errOut != "" {
		t.Errorf("--doc json should not warn or error, got stderr: %s", errOut)
	}
	if !strings.Contains(out, "\"demo\"") || !strings.Contains(out, "\"DocFlag\"") {
		t.Errorf("--doc json output missing expected fields:\n%s", out)
	}
}
