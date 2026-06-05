package tui

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/panyam/demokit"
	"github.com/panyam/demokit/events"
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

// stepStartFromDef builds the events.StepStart projection for a
// *demokit.StepDef the same way demokit's event emitter does. Tests
// use this to drive printStepBlock directly without standing up a
// full Execute + event queue.
func stepStartFromDef(visit, declared int, step *demokit.StepDef) events.StepStart {
	return events.StepStart{
		Visit:     visit,
		StepID:    step.StepID(),
		Title:     step.Title(),
		Note:      step.NoteText(),
		Declared:  declared,
		Refs:      refsToEvents(step.Refs()),
		Arrows:    arrowsToEvents(step.Arrows()),
		Verbatims: verbatimsToEventsTUI(step.VerbatimBlocks()),
	}
}

func TestRendererImplementsInterface(t *testing.T) {
	var _ demokit.Renderer = (*Renderer)(nil)
}

func TestRenderHeader(t *testing.T) {
	r := newTestRenderer()
	out := captureStdout(t, func() {
		r.printHeaderBlock("My Demo", "A description", 5)
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
		r.printStepBlock(1, 3, stepStartFromDef(1, 3, step), false)
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
		r.printStepBlock(1, 1, stepStartFromDef(1, 1, step), false)
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

// TestVerbatimBorderHorizontalOnlyNoSideChars verifies a multi-
// variant verbatim block under BorderHorizontalOnly produces top +
// bottom border lines but no `│` chars on inner content rows. This
// is the load-bearing assertion for the copy-paste use case from
// mcpkit walkthroughs (issue 55): mouse-select on a content row
// must not pick up vertical box characters.
func TestVerbatimBorderHorizontalOnlyNoSideChars(t *testing.T) {
	r := newTestRenderer()
	r.MaxWidth = 80
	r.Fraction = 1.0
	r.Delay = -1
	r.WithBorderStyle(demokit.BorderHorizontalOnly)

	demo := demokit.New("v").BoxedVerbatim()
	step := demo.Step("Repro").VerbatimVariants("Fetch",
		demokit.MakeVariant("curl", "bash", "curl -X GET https://example.com").Default(),
		demokit.MakeVariant("python", "python", "requests.get('https://example.com')"),
	)

	out := captureStdout(t, func() {
		r.printStepBlock(1, 1, stepStartFromDef(1, 1, step), true)
	})

	// Inner content rows must not contain `│`.
	for i, row := range strings.Split(out, "\n") {
		if strings.Contains(row, "curl -X GET") || strings.Contains(row, "requests.get") {
			if strings.ContainsRune(row, '│') {
				t.Errorf("row %d contains a side-border char on a content row:\n%q", i, row)
			}
		}
	}

	// And the horizontal border must appear at the top + bottom — the
	// frame is still visually present, just without side chars.
	if !strings.ContainsRune(out, '─') {
		t.Errorf("expected horizontal border char `─` in output, got:\n%s", out)
	}
}

// TestVerbatimBorderNoneNoBoxChars verifies BorderNone strips the
// box entirely from verbatim blocks. The verbatim region starts at
// the "Fetch" label; from there, no border chars should appear.
// The step header box (out of scope) retains its rounded chars.
func TestVerbatimBorderNoneNoBoxChars(t *testing.T) {
	r := newTestRenderer()
	r.MaxWidth = 80
	r.Fraction = 1.0
	r.Delay = -1
	r.WithBorderStyle(demokit.BorderNone)

	demo := demokit.New("v").BoxedVerbatim()
	step := demo.Step("Repro").VerbatimVariants("Fetch",
		demokit.MakeVariant("curl", "bash", "curl -X GET https://example.com").Default(),
		demokit.MakeVariant("python", "python", "requests.get('https://example.com')"),
	)

	out := captureStdout(t, func() {
		r.printStepBlock(1, 1, stepStartFromDef(1, 1, step), true)
	})

	if !strings.Contains(out, "curl -X GET") {
		t.Fatalf("expected variant content in output, got:\n%s", out)
	}

	// Snip off everything before the "Fetch" label so the step
	// header box (out of scope) doesn't muddy the assertion.
	verbatimStart := strings.Index(out, "Fetch")
	if verbatimStart < 0 {
		t.Fatalf("verbatim label `Fetch` missing from output:\n%s", out)
	}
	verbatimRegion := out[verbatimStart:]

	for _, b := range []rune{'│', '─', '╭', '╮', '╰', '╯'} {
		if strings.ContainsRune(verbatimRegion, b) {
			t.Errorf("BorderNone should suppress all border chars in the verbatim region, found %q in:\n%s",
				string(b), verbatimRegion)
		}
	}
}

// TestVerbatimBorderCharsCustom verifies a custom BorderChars value
// reaches the rendered output of the verbatim block: a struct
// literal with `#` on top/bottom, `*` on sides, and `+` corners
// produces those chars on the verbatim frame. Acceptance for the
// "custom chars via struct literal" path of the issue 55 design.
// The step header box (out of scope per WithBorderStyle/Chars) is
// unaffected.
func TestVerbatimBorderCharsCustom(t *testing.T) {
	r := newTestRenderer()
	r.MaxWidth = 80
	r.Fraction = 1.0
	r.Delay = -1
	r.WithBorderStyle(demokit.BorderFull).
		WithBorderChars(demokit.BorderChars{
			Top: "#", Bottom: "#", Left: "*", Right: "*",
			TopLeft: "+", TopRight: "+", BottomLeft: "+", BottomRight: "+",
		})

	demo := demokit.New("v").BoxedVerbatim()
	step := demo.Step("Repro").VerbatimVariants("Fetch",
		demokit.MakeVariant("curl", "bash", "curl -X GET https://example.com").Default(),
	)

	out := captureStdout(t, func() {
		r.printStepBlock(1, 1, stepStartFromDef(1, 1, step), true)
	})

	// Find the verbatim content row, then assert its surrounding
	// frame uses the custom chars.
	rows := strings.Split(out, "\n")
	contentRowIdx := -1
	for i, row := range rows {
		if strings.Contains(row, "curl -X GET") {
			contentRowIdx = i
			break
		}
	}
	if contentRowIdx < 0 {
		t.Fatalf("verbatim content row not found in output:\n%s", out)
	}
	contentRow := rows[contentRowIdx]
	if !strings.Contains(contentRow, "*") {
		t.Errorf("content row should have `*` side chars (Left/Right custom), got:\n%q", contentRow)
	}
	// Inner content rows must NOT contain rounded side-chars `│`.
	if strings.ContainsRune(contentRow, '│') {
		t.Errorf("content row should not have default `│`, custom Left=`*` should win:\n%q", contentRow)
	}
	// Top/bottom rows of the verbatim frame use `#` and `+`.
	if contentRowIdx == 0 || contentRowIdx >= len(rows)-1 {
		t.Fatalf("content row at edge of output, can't inspect frame; output:\n%s", out)
	}
	topRow := rows[contentRowIdx-1]
	bottomRow := rows[contentRowIdx+1]
	for _, want := range []string{"#", "+"} {
		if !strings.Contains(topRow, want) {
			t.Errorf("top frame row should contain %q, got:\n%q", want, topRow)
		}
		if !strings.Contains(bottomRow, want) {
			t.Errorf("bottom frame row should contain %q, got:\n%q", want, bottomRow)
		}
	}
}

// TestVerbatimBorderCharsRoundedHInfersHorizontalSides verifies the
// one-call ergonomic shortcut: passing BorderCharsRoundedH (Left/
// Right empty) without an explicit WithBorderStyle still produces
// a horizontal-only box. The empty side fields are the signal.
func TestVerbatimBorderCharsRoundedHInfersHorizontalSides(t *testing.T) {
	r := newTestRenderer()
	r.MaxWidth = 80
	r.Fraction = 1.0
	r.Delay = -1
	r.WithBorderChars(demokit.BorderCharsRoundedH)

	demo := demokit.New("v").BoxedVerbatim()
	step := demo.Step("Repro").VerbatimVariants("Fetch",
		demokit.MakeVariant("curl", "bash", "curl -X GET https://example.com").Default(),
	)

	out := captureStdout(t, func() {
		r.printStepBlock(1, 1, stepStartFromDef(1, 1, step), true)
	})

	// Content rows must not have side chars.
	for i, row := range strings.Split(out, "\n") {
		if strings.Contains(row, "curl -X GET") {
			if strings.ContainsRune(row, '│') {
				t.Errorf("row %d unexpectedly contains side char `│`:\n%q", i, row)
			}
		}
	}
	// Top/bottom horizontal chars must appear.
	if !strings.ContainsRune(out, '─') {
		t.Errorf("expected horizontal char `─` from BorderCharsRoundedH, got:\n%s", out)
	}
}

// TestResultBoxBorderHorizontalOnly verifies the result/output box
// (printResultBlock) also honors BorderHorizontalOnly. Result box
// contains captured stdout, so mouse-select on output rows must
// not pick up `│`.
func TestResultBoxBorderHorizontalOnly(t *testing.T) {
	r := newTestRenderer()
	r.MaxWidth = 80
	r.Fraction = 1.0
	r.Delay = -1
	r.WithBorderStyle(demokit.BorderHorizontalOnly)

	out := captureStdout(t, func() {
		r.printResultBlock(1, "some captured output line", nil)
	})

	for i, row := range strings.Split(out, "\n") {
		if strings.Contains(row, "some captured output") {
			if strings.ContainsRune(row, '│') {
				t.Errorf("result output row %d has side-border char:\n%q", i, row)
			}
		}
	}
}

// TestVerbatimBorderDefaultUnchanged is the regression guard for
// callers who never opt in: the existing rounded all-sides border
// must still be drawn when WithBorderStyle / WithBorderChars are
// not called.
func TestVerbatimBorderDefaultUnchanged(t *testing.T) {
	r := newTestRenderer()
	r.MaxWidth = 80
	r.Fraction = 1.0
	r.Delay = -1

	demo := demokit.New("v").BoxedVerbatim()
	step := demo.Step("Repro").VerbatimVariants("Fetch",
		demokit.MakeVariant("curl", "bash", "curl -X GET https://example.com").Default(),
	)

	out := captureStdout(t, func() {
		r.printStepBlock(1, 1, stepStartFromDef(1, 1, step), true)
	})

	// Rounded corners must appear (today's default).
	for _, want := range []rune{'╭', '╮', '╰', '╯', '│', '─'} {
		if !strings.ContainsRune(out, want) {
			t.Errorf("default border should contain rounded char %q, missing in:\n%s",
				string(want), out)
		}
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
		r.printStepBlock(1, 1, stepStartFromDef(1, 1, step), false)
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
		r.printResultBlock(1, "some output", nil)
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
		r.printResultBlock(1, "", nil)
	})
	if strings.Contains(out, "Result") {
		t.Error("empty result should not render a Result box")
	}
}

func TestRenderResultError(t *testing.T) {
	r := newTestRenderer()
	out := captureStdout(t, func() {
		r.printResultBlock(1, "partial output", demokit.Errf("something broke"))
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
		r.printResultBlock(1, "", demokit.Warn("watch out"))
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
		r.printResultBlock(1, "", demokit.Info("FYI"))
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
		r.printResultBlock(1, "", &demokit.StepResult{
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
		r.printDoneBlock()
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
		r.printHeaderBlock("Narrow", "", 1)
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
		r.printHeaderBlock("Palette Test", "", 1)
	})
	if !strings.Contains(out, "Palette Test") {
		t.Error("custom palette render failed")
	}
}
