package tui

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/panyam/demokit"
)

// captureStdout runs fn and returns what it wrote to stdout.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w

	done := make(chan string)
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		done <- buf.String()
	}()

	fn()

	w.Close()
	os.Stdout = old
	return <-done
}

// newTestRenderer creates a renderer with smooth scroll disabled for fast tests.
func newTestRenderer() *Renderer {
	r := New()
	r.Delay = -1
	return r
}

func TestRendererImplementsInterface(t *testing.T) {
	var _ demokit.Renderer = (*Renderer)(nil)
}

func TestRenderHeader(t *testing.T) {
	r := newTestRenderer()
	out := captureStdout(t, func() {
		r.RenderHeader("My Demo", "A description", 5)
	})
	if !strings.Contains(out, "My Demo") {
		t.Error("header should contain title")
	}
	if !strings.Contains(out, "5 steps") {
		t.Error("header should contain step count")
	}
}

func TestRenderStep(t *testing.T) {
	r := newTestRenderer()
	demo := demokit.New("test").Actors(demokit.Actor("A", "A"), demokit.Actor("B", "B"))
	step := demo.Step("Test Step").
		Arrow("A", "B", "call").
		DashedArrow("B", "A", "reply").
		Ref(demokit.Ref{Name: "RFC 123", URL: "https://example.com"}).
		Note("This is a note")

	out := captureStdout(t, func() {
		r.RenderStep(1, 3, step)
	})

	checks := []string{"Step 1/3", "Test Step", "RFC 123", "A", "B", "call", "This is a note"}
	for _, c := range checks {
		if !strings.Contains(out, c) {
			t.Errorf("step output missing %q", c)
		}
	}
}

// TestRenderStepVerbatimNoBorderInjection is the load-bearing test for
// the copy-paste invariant: a content line longer than the renderer's
// width must NOT have box border characters injected mid-line. The bug
// being prevented: lipgloss soft-wrapping a 200-char curl recipe inside
// a 80-col bordered box, splitting the JSON body across two visual rows
// with a `│` between them.
func TestRenderStepVerbatimNoBorderInjection(t *testing.T) {
	r := newTestRenderer()
	r.MaxWidth = 80
	r.Fraction = 1.0
	r.Delay = -1

	// 200-char content marker — distinct from any glyph the box uses.
	content := strings.Repeat("X", 200)
	demo := demokit.New("v")
	step := demo.Step("Repro").VerbatimLang("the wire", "bash", content)

	out := captureStdout(t, func() {
		r.RenderStep(1, 1, step)
	})

	// Walk every output row. If a row contains the verbatim content
	// marker AND a box border char, the invariant is broken.
	for i, row := range strings.Split(out, "\n") {
		hasContent := strings.ContainsRune(row, 'X')
		if !hasContent {
			continue
		}
		for _, b := range []rune{'│', '─', '┌', '┐', '└', '┘', '╭', '╮', '╰', '╯'} {
			if strings.ContainsRune(row, b) {
				t.Errorf("row %d contains both verbatim content and border %q:\n%q",
					i, string(b), row)
			}
		}
	}

	// And the content must actually appear in full — clipping/ellipsis
	// would defeat the copy-paste use case.
	if !strings.Contains(out, content) {
		t.Errorf("full 200-char content missing from output (clipped?)\n%s", out)
	}

	// The label must render too.
	if !strings.Contains(out, "the wire") {
		t.Errorf("verbatim label missing from output\n%s", out)
	}
}

// TestRenderStepVerbatimMultilinePreserved verifies multi-line content
// is emitted line-by-line with each input line on its own output row.
func TestRenderStepVerbatimMultilinePreserved(t *testing.T) {
	r := newTestRenderer()
	r.MaxWidth = 80
	r.Fraction = 1.0
	r.Delay = -1

	demo := demokit.New("v")
	step := demo.Step("Repro").Verbatim("", "line1\nline2\nline3")

	out := captureStdout(t, func() {
		r.RenderStep(1, 1, step)
	})

	for _, want := range []string{"line1", "line2", "line3"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n%s", want, out)
		}
	}
}

func TestRenderResultSuccess(t *testing.T) {
	r := newTestRenderer()
	out := captureStdout(t, func() {
		r.RenderResult(1, "some output", nil)
	})
	if !strings.Contains(out, "Result") {
		t.Error("result should contain 'Result' label")
	}
	if !strings.Contains(out, "some output") {
		t.Error("result should contain the output text")
	}
}

func TestRenderResultEmpty(t *testing.T) {
	r := newTestRenderer()
	out := captureStdout(t, func() {
		r.RenderResult(1, "", nil)
	})
	if strings.Contains(out, "Result") {
		t.Error("empty result should not render a Result box")
	}
}

func TestRenderResultError(t *testing.T) {
	r := newTestRenderer()
	out := captureStdout(t, func() {
		r.RenderResult(1, "partial output", demokit.Errf("something broke"))
	})
	if !strings.Contains(out, "Error") {
		t.Error("error result should contain 'Error' label")
	}
	if !strings.Contains(out, "something broke") {
		t.Error("error result should contain error message")
	}
	if !strings.Contains(out, "partial output") {
		t.Error("error result should still show captured output")
	}
}

func TestRenderResultWarning(t *testing.T) {
	r := newTestRenderer()
	out := captureStdout(t, func() {
		r.RenderResult(1, "", demokit.Warn("watch out"))
	})
	if !strings.Contains(out, "Warning") {
		t.Error("warning result should contain 'Warning' label")
	}
	if !strings.Contains(out, "watch out") {
		t.Error("warning result should contain message")
	}
}

func TestRenderResultInfo(t *testing.T) {
	r := newTestRenderer()
	out := captureStdout(t, func() {
		r.RenderResult(1, "", demokit.Info("FYI"))
	})
	if !strings.Contains(out, "Info") {
		t.Error("info result should contain 'Info' label")
	}
	if !strings.Contains(out, "FYI") {
		t.Error("info result should contain message")
	}
}

func TestRenderResultCustomLabel(t *testing.T) {
	r := newTestRenderer()
	out := captureStdout(t, func() {
		r.RenderResult(1, "", &demokit.StepResult{
			Status: demokit.StatusWarning, Label: "Heads Up", Message: "custom label",
		})
	})
	if !strings.Contains(out, "Heads Up") {
		t.Error("custom label result should contain 'Heads Up'")
	}
}

func TestRenderSection(t *testing.T) {
	r := newTestRenderer()
	demo := demokit.New("test")
	demo.Section("My Section", "line 1", "line 2")

	// Access the section through the demo's items (we need the SectionDef)
	// Since Section returns *Demo not *SectionDef, build one directly via the demo.
	// The TUI renderer takes *SectionDef, so we test via the full Execute path instead.
	// For a unit test, we can check the output contains expected text.
	out := captureStdout(t, func() {
		// We can't easily get a *SectionDef from outside the package,
		// so test via a full non-interactive Execute with the TUI renderer.
		orig := os.Args
		defer func() { os.Args = orig }()
		os.Args = []string{"test", "--non-interactive"}

		d := demokit.New("sec test").WithRenderer(r)
		d.Section("My Section", "line 1", "line 2")
		d.Execute()
	})

	if !strings.Contains(out, "My Section") {
		t.Error("section output missing title")
	}
	if !strings.Contains(out, "line 1") {
		t.Error("section output missing body")
	}
}

func TestRenderDone(t *testing.T) {
	r := newTestRenderer()
	out := captureStdout(t, func() {
		r.RenderDone()
	})
	if !strings.Contains(out, "Done") {
		t.Error("done output missing 'Done'")
	}
}

func TestCustomWidth(t *testing.T) {
	r := newTestRenderer()
	r.MaxWidth = 40
	r.Fraction = 1.0 // use full terminal width, capped by MaxWidth
	r.Delay = -1     // disable smooth scroll for test speed

	out := captureStdout(t, func() {
		r.RenderHeader("Narrow", "", 1)
	})
	if !strings.Contains(out, "Narrow") {
		t.Error("narrow render missing title")
	}
}

func TestCustomPalette(t *testing.T) {
	r := newTestRenderer()
	// Just verify it doesn't panic with a custom palette.
	r.Palette = DefaultPalette()
	out := captureStdout(t, func() {
		r.RenderHeader("Palette Test", "", 1)
	})
	if !strings.Contains(out, "Palette Test") {
		t.Error("custom palette render failed")
	}
}
