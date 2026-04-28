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

func TestRendererImplementsInterface(t *testing.T) {
	var _ demokit.Renderer = (*Renderer)(nil)
}

func TestRenderHeader(t *testing.T) {
	r := New()
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
	r := New()
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

func TestRenderResult(t *testing.T) {
	r := New()
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
	r := New()
	out := captureStdout(t, func() {
		r.RenderResult(1, "", nil)
	})
	// Empty result should not render a box
	if strings.Contains(out, "Result") {
		t.Error("empty result should not render a Result box")
	}
}

func TestRenderSection(t *testing.T) {
	r := New()
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
	r := New()
	out := captureStdout(t, func() {
		r.RenderDone()
	})
	if !strings.Contains(out, "Done") {
		t.Error("done output missing 'Done'")
	}
}

func TestCustomWidth(t *testing.T) {
	r := New()
	r.Width = 40

	out := captureStdout(t, func() {
		r.RenderHeader("Narrow", "", 1)
	})
	// Verify it renders without panic and contains the title.
	if !strings.Contains(out, "Narrow") {
		t.Error("narrow render missing title")
	}
}

func TestCustomPalette(t *testing.T) {
	r := New()
	// Just verify it doesn't panic with a custom palette.
	r.Palette = DefaultPalette()
	out := captureStdout(t, func() {
		r.RenderHeader("Palette Test", "", 1)
	})
	if !strings.Contains(out, "Palette Test") {
		t.Error("custom palette render failed")
	}
}