package demokit

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestStepAccessors(t *testing.T) {
	s := &StepDef{}
	s.title = "test step"
	s.note = "a note"
	s.refs = []Ref{{Name: "RFC 1", URL: "https://example.com"}}
	s.arrows = []arrowDef{
		{from: "A", to: "B", label: "request"},
		{from: "B", to: "A", label: "response", dashed: true},
	}

	if s.Title() != "test step" {
		t.Errorf("Title() = %q, want %q", s.Title(), "test step")
	}
	if s.NoteText() != "a note" {
		t.Errorf("NoteText() = %q, want %q", s.NoteText(), "a note")
	}
	if len(s.Refs()) != 1 || s.Refs()[0].Name != "RFC 1" {
		t.Errorf("Refs() unexpected: %v", s.Refs())
	}

	arrows := s.Arrows()
	if len(arrows) != 2 {
		t.Fatalf("Arrows() len = %d, want 2", len(arrows))
	}
	if arrows[0].From != "A" || arrows[0].To != "B" || arrows[0].Label != "request" || arrows[0].Dashed {
		t.Errorf("Arrows()[0] = %+v", arrows[0])
	}
	if !arrows[1].Dashed {
		t.Error("Arrows()[1].Dashed should be true")
	}
}

func TestSectionAccessors(t *testing.T) {
	s := &SectionDef{title: "sec", body: "body text"}
	if s.Title() != "sec" {
		t.Errorf("Title() = %q", s.Title())
	}
	if s.Body() != "body text" {
		t.Errorf("Body() = %q", s.Body())
	}
}

func TestCaptureOutput(t *testing.T) {
	out, err := captureOutput(func() {
		fmt.Print("hello world")
	})
	if err != nil {
		t.Fatalf("captureOutput error: %v", err)
	}
	if out != "hello world" {
		t.Errorf("captureOutput = %q, want %q", out, "hello world")
	}
}

func TestCaptureOutputEmpty(t *testing.T) {
	out, err := captureOutput(func() {})
	if err != nil {
		t.Fatalf("captureOutput error: %v", err)
	}
	if out != "" {
		t.Errorf("captureOutput = %q, want empty", out)
	}
}

// recordingRenderer captures all renderer calls for testing.
type recordingRenderer struct {
	calls []string
}

func (r *recordingRenderer) RenderHeader(title, desc string, n int)      { r.calls = append(r.calls, "header:"+title) }
func (r *recordingRenderer) RenderStep(num, total int, s *StepDef)       { r.calls = append(r.calls, "step:"+s.title) }
func (r *recordingRenderer) RenderResult(num int, out string, err error) { r.calls = append(r.calls, "result") }
func (r *recordingRenderer) RenderSection(s *SectionDef)                 { r.calls = append(r.calls, "section:"+s.title) }
func (r *recordingRenderer) RenderDone()                                 { r.calls = append(r.calls, "done") }
func (r *recordingRenderer) WaitForStep()                                {} // no-op

func TestExecuteCallsRenderer(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"test", "--non-interactive"}

	rec := &recordingRenderer{}
	demo := New("Test Demo").WithRenderer(rec)
	demo.Section("Intro", "hello")
	demo.Step("Do thing").Run(func() { fmt.Print("output") })
	demo.Step("Another")

	demo.Execute()

	got := strings.Join(rec.calls, ", ")
	want := "header:Test Demo, section:Intro, step:Do thing, result, step:Another, result, done"
	if got != want {
		t.Errorf("renderer calls:\n  got:  %s\n  want: %s", got, want)
	}
}

func TestMarkdownGeneration(t *testing.T) {
	demo := New("MD Test").
		Description("desc").
		Dir("test-dir").
		Actors(Actor("A", "Alpha"), Actor("B", "Beta"))
	demo.Step("First").
		Arrow("A", "B", "call").
		Note("a note")

	md := demo.Markdown()

	checks := []string{
		"# MD Test",
		"desc",
		"sequenceDiagram",
		"participant A as Alpha",
		"A->>B: call",
		"Step 1: First",
		"a note",
		"go run ./examples/test-dir/",
	}
	for _, c := range checks {
		if !strings.Contains(md, c) {
			t.Errorf("Markdown missing %q", c)
		}
	}
}

func TestBuilderChaining(t *testing.T) {
	demo := New("Chain").Description("d").Dir("dir").RunPrefix("ex")
	if demo.title != "Chain" || demo.description != "d" || demo.dir != "dir" || demo.runPrefix != "ex" {
		t.Errorf("chaining failed: %+v", demo)
	}

	s := demo.Step("s").
		Arrow("A", "B", "req").
		DashedArrow("B", "A", "res").
		Ref(Ref{Name: "R", URL: "http://r"}).
		Note("n").
		Run(func() {})

	if s.title != "s" || len(s.arrows) != 2 || len(s.refs) != 1 || s.note != "n" || s.runFn == nil {
		t.Errorf("step chaining failed: %+v", s)
	}
}
