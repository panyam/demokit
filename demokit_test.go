package demokit

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/panyam/demokit/events"
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

func TestNoteVariadic(t *testing.T) {
	t.Run("single arg", func(t *testing.T) {
		s := (&StepDef{}).Note("foo")
		if s.note != "foo" {
			t.Errorf("note = %q, want %q", s.note, "foo")
		}
	})
	t.Run("multi arg joins with newline", func(t *testing.T) {
		s := (&StepDef{}).Note("a", "b")
		if s.note != "a\nb" {
			t.Errorf("note = %q, want %q", s.note, "a\nb")
		}
	})
	t.Run("no args leaves empty", func(t *testing.T) {
		s := (&StepDef{}).Note()
		if s.note != "" {
			t.Errorf("note = %q, want empty", s.note)
		}
	})
	t.Run("spread of slice", func(t *testing.T) {
		parts := []string{"Lead.", "", "- one", "- two"}
		s := (&StepDef{}).Note(parts...)
		want := "Lead.\n\n- one\n- two"
		if s.note != want {
			t.Errorf("note = %q, want %q", s.note, want)
		}
	})
}

// TestVerbatimAttachment verifies the three verbatim setters all
// append in declaration order and the read accessor projects them
// without mutation. Empty labels are permitted (markdown skips the
// heading; TUI omits the label line).
func TestVerbatimAttachment(t *testing.T) {
	s := (&StepDef{}).
		Verbatim("first", "alpha").
		VerbatimLang("second", "json", `{"k":"v"}`).
		Shell("echo hi").
		Verbatim("", "no-label content")

	got := s.VerbatimBlocks()
	if len(got) != 4 {
		t.Fatalf("VerbatimBlocks() len = %d, want 4", len(got))
	}

	want := []struct {
		Label, Lang, Content string
	}{
		{Label: "first", Lang: "", Content: "alpha"},
		{Label: "second", Lang: "json", Content: `{"k":"v"}`},
		{Label: "", Lang: "bash", Content: "echo hi"},
		{Label: "", Lang: "", Content: "no-label content"},
	}
	for i, w := range want {
		if len(got[i].Variants) != 1 {
			t.Fatalf("block[%d] Variants len = %d, want 1 (single-variant backcompat)", i, len(got[i].Variants))
		}
		va := got[i].Variants[0]
		if got[i].Label != w.Label || va.Lang != w.Lang || va.Content != w.Content {
			t.Errorf("block[%d] = {label=%q variant={lang=%q content=%q}}, want {label=%q variant={lang=%q content=%q}}",
				i, got[i].Label, va.Lang, va.Content, w.Label, w.Lang, w.Content)
		}
	}
}

// TestVerbatimContentByteExact verifies the raw bytes are preserved —
// no trimming, normalization, or tab→space — between Verbatim() and
// the read accessor. Wire-format demos rely on this.
func TestVerbatimContentByteExact(t *testing.T) {
	raw := "  leading-space\n\ttabbed\nline\twith\ttabs\ntrailing-newline\n"
	s := (&StepDef{}).Verbatim("", raw)
	got := s.VerbatimBlocks()[0].Variants[0].Content
	if got != raw {
		t.Errorf("Verbatim content not preserved byte-exact:\n got:  %q\n want: %q", got, raw)
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

func TestCaptureOutputSuccess(t *testing.T) {
	out, result := captureOutput(func(ctx StepContext) *StepResult {
		fmt.Print("hello world")
		return nil
	}, StepContext{}, nil)
	if result != nil {
		t.Fatalf("expected nil result, got %+v", result)
	}
	if out != "hello world" {
		t.Errorf("captureOutput = %q, want %q", out, "hello world")
	}
}

func TestCaptureOutputEmpty(t *testing.T) {
	out, result := captureOutput(func(ctx StepContext) *StepResult { return nil }, StepContext{}, nil)
	if result != nil {
		t.Fatalf("expected nil result, got %+v", result)
	}
	if out != "" {
		t.Errorf("captureOutput = %q, want empty", out)
	}
}

func TestCaptureOutputError(t *testing.T) {
	out, result := captureOutput(func(ctx StepContext) *StepResult {
		fmt.Print("partial output")
		return Errf("step failed")
	}, StepContext{}, nil)
	if result == nil || result.Status != StatusError {
		t.Fatalf("expected error result, got %+v", result)
	}
	if result.Message != "step failed" {
		t.Errorf("message = %q, want %q", result.Message, "step failed")
	}
	if out != "partial output" {
		t.Errorf("output = %q, want %q", out, "partial output")
	}
}

func TestCaptureOutputPanic(t *testing.T) {
	out, result := captureOutput(func(ctx StepContext) *StepResult {
		fmt.Print("before panic")
		panic("boom")
	}, StepContext{}, nil)
	if result == nil || result.Status != StatusError {
		t.Fatalf("expected error result from panic, got %+v", result)
	}
	if !strings.Contains(result.Message, "boom") {
		t.Errorf("message = %q, want to contain 'boom'", result.Message)
	}
	if out != "before panic" {
		t.Errorf("output = %q, want %q", out, "before panic")
	}
}

func TestCaptureOutputWarning(t *testing.T) {
	_, result := captureOutput(func(ctx StepContext) *StepResult {
		return Warn("heads up")
	}, StepContext{}, nil)
	if result == nil || result.Status != StatusWarning {
		t.Fatalf("expected warning result, got %+v", result)
	}
	if result.DisplayLabel() != "Warning" {
		t.Errorf("label = %q, want %q", result.DisplayLabel(), "Warning")
	}
}

func TestCaptureOutputInfo(t *testing.T) {
	_, result := captureOutput(func(ctx StepContext) *StepResult {
		return Info("FYI")
	}, StepContext{}, nil)
	if result == nil || result.Status != StatusInfo {
		t.Fatalf("expected info result, got %+v", result)
	}
	if result.DisplayLabel() != "Info" {
		t.Errorf("label = %q, want %q", result.DisplayLabel(), "Info")
	}
}

func TestStepResultCustomLabel(t *testing.T) {
	r := &StepResult{Status: StatusWarning, Label: "Heads Up", Message: "custom"}
	if r.DisplayLabel() != "Heads Up" {
		t.Errorf("DisplayLabel() = %q, want %q", r.DisplayLabel(), "Heads Up")
	}
}

func TestStepResultDefaultLabels(t *testing.T) {
	tests := []struct {
		status ResultStatus
		want   string
	}{
		{StatusSuccess, "Result"},
		{StatusError, "Error"},
		{StatusWarning, "Warning"},
		{StatusInfo, "Info"},
	}
	for _, tt := range tests {
		got := tt.status.DefaultLabel()
		if got != tt.want {
			t.Errorf("DefaultLabel(%d) = %q, want %q", tt.status, got, tt.want)
		}
	}
}

// recordingRenderer is an event-aware test spy. It drains the demo's
// event queue and records a string for each discrete event so tests
// can assert on the demo's emission sequence. AttachEventQueue +
// Finish satisfy the EventAwareRenderer / FinishableRenderer contract;
// PromptOpen events auto-resolve with declared defaults so tests
// that exercise input-bearing steps don't deadlock.
//
// Subclasses install onChunk / onStepEnd hooks to react to streamed
// output without re-implementing the drain loop — used by the
// streaming spies in term_test.go and cancel_test.go.
type recordingRenderer struct {
	mu      sync.Mutex
	calls   []string
	queue   *events.EventQueue
	drainWG sync.WaitGroup
	done    bool

	onChunk   func(visit int, chunk []byte)
	onStepEnd func(visit int, status, message, errorText string)
}

func (r *recordingRenderer) AttachEventQueue(q *events.EventQueue) {
	r.queue = q
	r.done = false
	r.drainWG.Add(1)
	go func() {
		defer r.drainWG.Done()
		sub := q.Subscribe()
		defer sub.Close()
		offset := r.drainFrom(0)
		for !r.done {
			<-sub.Notify()
			offset = r.drainFrom(offset)
		}
	}()
}

func (r *recordingRenderer) Finish() { r.drainWG.Wait() }

func (r *recordingRenderer) drainFrom(offset int) int {
	evs, newOff := r.queue.ReadFrom(offset)
	for i, ev := range evs {
		r.handleEvent(offset+i, ev)
	}
	return newOff
}

func (r *recordingRenderer) handleEvent(off int, ev events.Event) {
	switch e := ev.(type) {
	case events.Header:
		r.record("header:" + e.Title)
	case events.StepStart:
		r.record("step:" + e.Title)
	case events.StepEnd:
		r.record("result")
		if r.onStepEnd != nil {
			r.onStepEnd(e.Visit, e.Status, e.Message, e.ErrorText)
		}
	case events.OutputChunk:
		if r.onChunk != nil {
			r.onChunk(e.Visit, e.Chunk)
		}
	case events.Section:
		r.record("section:" + e.Title)
	case events.Done:
		r.record("done")
		r.done = true
	case events.PromptOpen:
		// Mirror the legacy spy's default-filling behavior so input
		// steps don't deadlock waiting on a stdin that tests don't
		// provide.
		if _, resolved := r.queue.Resolution(off); resolved {
			return
		}
		out := make(map[string]any, len(e.Inputs))
		for _, in := range e.Inputs {
			if d := in.InputDefault(); d != nil {
				out[in.InputName()] = d
			}
		}
		_ = r.queue.Resolve(off, &events.PromptResolution{
			Answers: out, Source: "default", Timestamp: time.Now(),
		})
	}
}

func (r *recordingRenderer) record(s string) {
	r.mu.Lock()
	r.calls = append(r.calls, s)
	r.mu.Unlock()
}

// TestExecuteJumpViaNext verifies that a step's StepResult.Next jumps to
// the named step ID, skipping items between current and target.
func TestExecuteJumpViaNext(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"test", "--non-interactive"}

	rec := &recordingRenderer{}
	demo := New("DAG").WithRenderer(rec).MaxSteps(10)
	demo.Step("first").ID("a").Run(func(ctx StepContext) *StepResult {
		return &StepResult{Next: "c"}
	})
	demo.Step("skipped").ID("b")
	demo.Step("third").ID("c")

	demo.Execute()

	got := strings.Join(rec.calls, ", ")
	want := "header:DAG, step:first, result, step:third, result, done"
	if got != want {
		t.Errorf("calls:\n  got:  %s\n  want: %s", got, want)
	}
}

// TestExecuteFallThrough verifies steps run in declaration order when no
// Next is set, and sections in between are visited.
func TestExecuteFallThrough(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"test", "--non-interactive"}

	rec := &recordingRenderer{}
	demo := New("Linear").WithRenderer(rec)
	demo.Step("one")
	demo.Section("Mid", "between")
	demo.Step("two")

	demo.Execute()

	got := strings.Join(rec.calls, ", ")
	want := "header:Linear, step:one, result, section:Mid, step:two, result, done"
	if got != want {
		t.Errorf("calls:\n  got:  %s\n  want: %s", got, want)
	}
}

// TestExecuteMaxSteps verifies the runaway-loop guard fires when a cycle
// exceeds Demo.MaxSteps.
func TestExecuteMaxSteps(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"test", "--non-interactive"}

	rec := &recordingRenderer{}
	demo := New("Loop").WithRenderer(rec).MaxSteps(5)
	demo.Step("loop").ID("loop").Run(func(ctx StepContext) *StepResult {
		return &StepResult{Next: "loop"}
	})

	demo.Execute()

	stepCount := strings.Count(strings.Join(rec.calls, ","), "step:loop")
	if stepCount != 5 {
		t.Errorf("expected 5 step visits before max-steps guard, got %d", stepCount)
	}
}

// TestExecuteMaxVisitsPerStep verifies the per-step revisit cap.
func TestExecuteMaxVisitsPerStep(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"test", "--non-interactive"}

	rec := &recordingRenderer{}
	demo := New("Loop").WithRenderer(rec).MaxSteps(100).MaxVisits(3)
	demo.Step("loop").ID("loop").Run(func(ctx StepContext) *StepResult {
		return &StepResult{Next: "loop"}
	})

	demo.Execute()

	stepCount := strings.Count(strings.Join(rec.calls, ","), "step:loop")
	if stepCount != 3 {
		t.Errorf("expected 3 step visits before max-visits guard, got %d", stepCount)
	}
}

// TestMaxVisitsRecordsErrorEntry verifies that when the per-step
// MaxVisits guard fires, the recorder receives a final TraceEntry
// flagged StatusError. Without this, doc renders / embed players
// silently truncate mid-loop with no indication that the demo
// aborted — what looked like a normal completion was actually a
// safety-guard trip.
func TestMaxVisitsRecordsErrorEntry(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"test", "--non-interactive"}

	rec := &MemoryRecorder{}
	demo := New("Loop").
		WithRenderer(&recordingRenderer{}).
		WithRecorder(rec).
		MaxSteps(100).
		MaxVisits(2)
	demo.Step("loop").ID("loop").Run(func(ctx StepContext) *StepResult {
		return &StepResult{Next: "loop"}
	})
	demo.Execute()

	if len(rec.Entries) == 0 {
		t.Fatal("expected trace entries, got none")
	}
	last := rec.Entries[len(rec.Entries)-1]
	if last.Status != StatusError {
		t.Errorf("trailing trace entry status = %v, want StatusError", last.Status)
	}
	if last.StepID != "loop" {
		t.Errorf("trailing entry step_id = %q, want %q", last.StepID, "loop")
	}
	if !strings.Contains(last.Message, "max visits") {
		t.Errorf("trailing message = %q, want to mention max visits", last.Message)
	}
}

// TestMaxStepsRecordsErrorEntry verifies the same shape for the
// total-steps guard. The synthesized entry uses a sentinel step id
// so it doesn't collide with author-defined steps in the trace.
func TestMaxStepsRecordsErrorEntry(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"test", "--non-interactive"}

	rec := &MemoryRecorder{}
	demo := New("OverallLoop").
		WithRenderer(&recordingRenderer{}).
		WithRecorder(rec).
		MaxSteps(3) // tiny cap; the loop runs forever otherwise
	demo.Step("loop").ID("loop").Run(func(ctx StepContext) *StepResult {
		return &StepResult{Next: "loop"}
	})
	demo.Execute()

	if len(rec.Entries) == 0 {
		t.Fatal("expected trace entries, got none")
	}
	last := rec.Entries[len(rec.Entries)-1]
	if last.Status != StatusError {
		t.Errorf("trailing trace entry status = %v, want StatusError", last.Status)
	}
	if last.StepID != "__demokit_aborted__" {
		t.Errorf("trailing entry step_id = %q, want sentinel \"__demokit_aborted__\"", last.StepID)
	}
	if !strings.Contains(last.Message, "max steps") {
		t.Errorf("trailing message = %q, want to mention max steps", last.Message)
	}
}

// TestExecuteUnknownNext verifies an unknown jump target produces an
// error result and aborts the demo cleanly.
func TestExecuteUnknownNext(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"test", "--non-interactive"}

	rec := &recordingRenderer{}
	demo := New("BadJump").WithRenderer(rec)
	demo.Step("first").Run(func(ctx StepContext) *StepResult {
		return &StepResult{Next: "does-not-exist"}
	})
	demo.Step("never").ID("never")

	demo.Execute()

	joined := strings.Join(rec.calls, ", ")
	if !strings.Contains(joined, "step:first") {
		t.Errorf("expected first step to render, got: %s", joined)
	}
	if strings.Contains(joined, "step:never") {
		t.Errorf("did not expect 'never' step to render: %s", joined)
	}
	if !strings.Contains(joined, "done") {
		t.Errorf("expected done at end despite error: %s", joined)
	}
}

// TestStepContextVisitsIncrement verifies the per-step visit counter is
// incremented and visible to the run function.
func TestStepContextVisitsIncrement(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"test", "--non-interactive"}

	rec := &recordingRenderer{}
	var seenVisits []int
	demo := New("Visits").WithRenderer(rec).MaxSteps(10)
	demo.Step("a").ID("a").Run(func(ctx StepContext) *StepResult {
		seenVisits = append(seenVisits, ctx.Visits)
		if ctx.Visits < 3 {
			return &StepResult{Next: "a"}
		}
		return nil
	})

	demo.Execute()

	want := []int{1, 2, 3}
	if len(seenVisits) != len(want) {
		t.Fatalf("expected visits %v, got %v", want, seenVisits)
	}
	for i, v := range want {
		if seenVisits[i] != v {
			t.Errorf("visit[%d] = %d, want %d", i, seenVisits[i], v)
		}
	}
}

// TestAutoAssignedIDs verifies steps without explicit IDs receive
// auto-generated step-N identifiers.
func TestAutoAssignedIDs(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"test", "--non-interactive"}

	demo := New("auto").WithRenderer(&recordingRenderer{})
	s1 := demo.Step("one")
	s2 := demo.Step("two").ID("explicit")
	s3 := demo.Step("three")

	demo.Execute()

	if s1.StepID() == "" || s3.StepID() == "" {
		t.Errorf("auto IDs not assigned: s1=%q s3=%q", s1.StepID(), s3.StepID())
	}
	if s2.StepID() != "explicit" {
		t.Errorf("explicit ID overridden: %q", s2.StepID())
	}
	if s1.StepID() == s3.StepID() {
		t.Errorf("auto IDs collided: %q", s1.StepID())
	}
}

// TestNonInteractiveUsesDefaults verifies that without a TTY (or with
// --non-interactive), declared inputs fall back to their Default value.
func TestNonInteractiveUsesDefaults(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"test", "--non-interactive"}

	rec := &recordingRenderer{}
	var seenUser any
	var seenPort any
	demo := New("Inputs").WithRenderer(rec)
	demo.Step("login").
		Input(String().Named("user", "Username").WithDefault("alice")).
		Input(Int().Named("port", "Port").WithDefault(8080)).
		Run(func(ctx StepContext) *StepResult {
			seenUser = ctx.Inputs["user"]
			seenPort = ctx.Inputs["port"]
			return nil
		})

	demo.Execute()

	if seenUser != "alice" {
		t.Errorf("user = %v, want alice", seenUser)
	}
	if seenPort != 8080 {
		t.Errorf("port = %v, want 8080", seenPort)
	}
}

// TestCoalesce verifies the Coalesce hook builds a typed struct from
// the inputs map and exposes it as ctx.Input.
func TestCoalesce(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"test", "--non-interactive"}

	type cfg struct {
		User string
		Port int
	}

	var got cfg
	demo := New("Coalesce").WithRenderer(&recordingRenderer{})
	demo.Step("s").
		Input(String().Named("user", "Username").WithDefault("bob")).
		Input(Int().Named("port", "Port").WithDefault(9000)).
		Coalesce(func(m map[string]any) any {
			return cfg{User: m["user"].(string), Port: m["port"].(int)}
		}).
		Run(func(ctx StepContext) *StepResult {
			got = ctx.Input.(cfg)
			return nil
		})

	demo.Execute()

	want := cfg{User: "bob", Port: 9000}
	if got != want {
		t.Errorf("coalesced = %+v, want %+v", got, want)
	}
}

// TestInputDefHelpers verifies parser helpers reject invalid input and
// accept valid input.
func TestInputDefHelpers(t *testing.T) {
	v, err := Int().Parse("42")
	if err != nil || v.(int) != 42 {
		t.Errorf("Int().Parse(\"42\") = (%v, %v)", v, err)
	}
	if _, err := Int().Parse("notanint"); err == nil {
		t.Error("Int() should reject non-numeric input")
	}

	v, err = Choice("yes", "no").Parse(" YES ")
	if err != nil || v.(string) != "yes" {
		t.Errorf("Choice case-insensitive match failed: (%v, %v)", v, err)
	}
	if _, err := Choice("yes", "no").Parse("maybe"); err == nil {
		t.Error("Choice should reject value outside list")
	}
}

// TestRecorderCapturesStepsAndSections verifies the recorder receives
// one TraceEntry per visited step (with inputs, output, resolved Next)
// and one per visited section, in execution order.
func TestRecorderCapturesStepsAndSections(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"test", "--non-interactive"}

	rec := &MemoryRecorder{}
	demo := New("Trace").WithRenderer(&recordingRenderer{}).WithRecorder(rec)
	demo.Step("first").ID("a").
		Input(String().Named("name", "Name").WithDefault("alice")).
		Run(func(ctx StepContext) *StepResult {
			fmt.Print("hi alice")
			return &StepResult{Next: "c"}
		})
	demo.Section("skipped", "this is between a and c but jumped over")
	demo.Step("third").ID("c").
		Run(func(ctx StepContext) *StepResult {
			return Info("done")
		})

	demo.Execute()

	if len(rec.Entries) != 2 {
		t.Fatalf("expected 2 trace entries, got %d", len(rec.Entries))
	}

	e0 := rec.Entries[0]
	if e0.Kind != KindStep || e0.StepID != "a" || e0.Next != "c" {
		t.Errorf("entry 0 wrong: %+v", e0)
	}
	if e0.Inputs["name"] != "alice" {
		t.Errorf("entry 0 inputs = %v", e0.Inputs)
	}
	if e0.Output != "hi alice" {
		t.Errorf("entry 0 output = %q", e0.Output)
	}

	e1 := rec.Entries[1]
	if e1.Kind != KindStep || e1.StepID != "c" || e1.Status != StatusInfo {
		t.Errorf("entry 1 wrong: %+v", e1)
	}
}

// TestJSONFileRecorderRoundTrip verifies a recorded trace can be loaded
// back into the same TraceEntry slice.
func TestJSONFileRecorderRoundTrip(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"test", "--non-interactive"}

	tmp, err := os.CreateTemp("", "demokit-trace-*.json")
	if err != nil {
		t.Fatal(err)
	}
	tmp.Close()
	defer os.Remove(tmp.Name())

	demo := New("RT").WithRenderer(&recordingRenderer{}).
		WithRecorder(NewJSONFileRecorder(tmp.Name()))
	demo.Step("only").ID("only").
		Input(Int().Named("n", "N").WithDefault(7)).
		Run(func(ctx StepContext) *StepResult { return nil })

	demo.Execute()

	loaded, err := LoadTrace(tmp.Name())
	if err != nil {
		t.Fatalf("LoadTrace: %v", err)
	}
	if len(loaded) != 1 || loaded[0].StepID != "only" {
		t.Fatalf("unexpected trace: %+v", loaded)
	}
	// JSON unmarshals numbers as float64 by default — verify the value
	// survives round-trip even if the type widened.
	got := loaded[0].Inputs["n"]
	if got != float64(7) && got != 7 {
		t.Errorf("input n = %v (%T), want 7", got, got)
	}
}

// TestRecordFlag verifies --record path.json constructs a JSONFileRecorder
// when none is set programmatically.
func TestRecordFlag(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	tmp, err := os.CreateTemp("", "demokit-recflag-*.json")
	if err != nil {
		t.Fatal(err)
	}
	tmp.Close()
	defer os.Remove(tmp.Name())

	os.Args = []string{"test", "--non-interactive", "--record", tmp.Name()}
	demo := New("Flag").WithRenderer(&recordingRenderer{})
	demo.Step("x").ID("x")
	demo.Execute()

	loaded, err := LoadTrace(tmp.Name())
	if err != nil {
		t.Fatalf("LoadTrace: %v", err)
	}
	if len(loaded) != 1 || loaded[0].StepID != "x" {
		t.Errorf("trace not written via --record: %+v", loaded)
	}
}

// TestReplayRoundTrip records a run, then replays the trace into a
// fresh demo and verifies the same path is taken with the same inputs,
// even when the user's Run logic chooses a different Next at replay
// time.
func TestReplayRoundTrip(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"test", "--non-interactive"}

	// Record run: choice "left" → step "left-target".
	recRec := &MemoryRecorder{}
	demo1 := New("rec").WithRenderer(&recordingRenderer{}).WithRecorder(recRec)
	demo1.Step("choose").ID("choose").
		Input(Choice("left", "right").Named("dir", "Direction").WithDefault("left")).
		Run(func(ctx StepContext) *StepResult {
			if ctx.Inputs["dir"] == "left" {
				return &StepResult{Next: "left-target"}
			}
			return &StepResult{Next: "right-target"}
		})
	demo1.Step("right").ID("right-target").Run(func(ctx StepContext) *StepResult { return Info("right") })
	demo1.Step("left").ID("left-target").Run(func(ctx StepContext) *StepResult { return Info("left") })
	demo1.Execute()

	if len(recRec.Entries) < 2 {
		t.Fatalf("expected ≥2 trace entries from record run, got %d", len(recRec.Entries))
	}
	if recRec.Entries[0].Next != "left-target" {
		t.Fatalf("record run did not jump to left-target: %+v", recRec.Entries[0])
	}

	// Replay run: same demo, but the Run logic deliberately chooses the
	// wrong branch — replay should still force "left-target".
	visited := []string{}
	demo2 := New("replay").WithRenderer(&recordingRenderer{}).WithReplay(recRec.Entries)
	demo2.Step("choose").ID("choose").
		Input(Choice("left", "right").Named("dir", "Direction").WithDefault("left")).
		Run(func(ctx StepContext) *StepResult {
			visited = append(visited, "choose:"+ctx.Inputs["dir"].(string))
			return &StepResult{Next: "right-target"} // deliberately wrong
		})
	demo2.Step("right").ID("right-target").Run(func(ctx StepContext) *StepResult {
		visited = append(visited, "right")
		return nil
	})
	demo2.Step("left").ID("left-target").Run(func(ctx StepContext) *StepResult {
		visited = append(visited, "left")
		return nil
	})
	demo2.Execute()

	if len(visited) != 2 || visited[0] != "choose:left" || visited[1] != "left" {
		t.Errorf("replay did not follow recorded path: %v", visited)
	}
}

// TestReplayFlag verifies --replay path.json loads a trace.
func TestReplayFlag(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()

	tmp, err := os.CreateTemp("", "demokit-replay-*.json")
	if err != nil {
		t.Fatal(err)
	}
	tmp.Close()
	defer os.Remove(tmp.Name())

	// Build a trace by hand (no record run needed).
	rec := NewJSONFileRecorder(tmp.Name())
	rec.Record(TraceEntry{Kind: KindStep, StepID: "a", Visit: 1, Next: "c"})
	rec.Record(TraceEntry{Kind: KindStep, StepID: "c", Visit: 1})
	if err := rec.Close(); err != nil {
		t.Fatal(err)
	}

	os.Args = []string{"test", "--replay", tmp.Name()}
	visited := []string{}
	demo := New("rf").WithRenderer(&recordingRenderer{})
	demo.Step("a").ID("a").Run(func(ctx StepContext) *StepResult {
		visited = append(visited, "a")
		return nil
	})
	demo.Step("b").ID("b").Run(func(ctx StepContext) *StepResult {
		visited = append(visited, "b")
		return nil
	})
	demo.Step("c").ID("c").Run(func(ctx StepContext) *StepResult {
		visited = append(visited, "c")
		return nil
	})
	demo.Execute()

	if strings.Join(visited, ",") != "a,c" {
		t.Errorf("--replay flag did not force trace path: %v", visited)
	}
}

// TestRenderDocumentMDSubstrings verifies trace-driven markdown
// captures the visited step path (including jumps) and reaches into
// the demo for notes and refs. The assertions are substring-based
// because exact byte equality is covered by render_test.go's
// composition tests; this test pins user-visible content survival.
func TestRenderDocumentMDSubstrings(t *testing.T) {
	demo := New("Branchy").Description("test trace doc gen")
	demo.Step("first").ID("a").
		Note("explanation of A").
		Ref(Ref{Name: "RFC X", URL: "https://rfc.example/X"})
	demo.Step("second").ID("b")

	trace := []TraceEntry{
		{Kind: KindStep, Title: "first", StepID: "a", Visit: 1,
			Inputs: map[string]any{"name": "alice"},
			Output: "hello\nworld",
			Next:   "b"},
		{Kind: KindStep, Title: "second", StepID: "b", Visit: 1,
			Status: StatusInfo, Message: "fyi"},
	}

	md := RenderDocumentMD(RenderContext{Demo: demo, Trace: trace})
	checks := []string{
		"# Branchy",
		"test trace doc gen",
		"### 1. first",
		"explanation of A",
		"RFC X",
		"https://rfc.example/X",
		"`name` = `alice`",
		"hello\nworld",
		"jumped to `b`",
		"### 2. second",
		"**Info:** fyi",
		"## References",
	}
	for _, c := range checks {
		if !strings.Contains(md, c) {
			t.Errorf("RenderDocumentMD missing %q", c)
		}
	}
}

// TestRenderDocumentHTMLEscaping verifies HTML output structure and
// that user-supplied content (description, output) is escaped before
// being written into the document body. A regression here is a
// genuine XSS-shaped vulnerability, so this stays as a focused test.
func TestRenderDocumentHTMLEscaping(t *testing.T) {
	demo := New("HTML Demo").Description("a <script>alert(1)</script>")
	demo.Step("step").ID("s")

	trace := []TraceEntry{
		{Kind: KindStep, Title: "step", StepID: "s", Visit: 1,
			Inputs: map[string]any{"x": "<b>"}, Output: "<not html>"},
	}
	out := RenderDocumentHTML(RenderContext{Demo: demo, Trace: trace})

	if !strings.Contains(out, "<!doctype html>") {
		t.Error("missing doctype")
	}
	if strings.Contains(out, "<script>alert(1)</script>") {
		t.Error("description not escaped")
	}
	if !strings.Contains(out, "&lt;not html&gt;") {
		t.Error("output not escaped")
	}
}

// TestRegisterFlags verifies that registering demokit's flags onto a
// user-owned FlagSet routes values into the demo and disables the
// internal os.Args scan.
func TestRegisterFlags(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()

	tmp, err := os.CreateTemp("", "demokit-rf-*.json")
	if err != nil {
		t.Fatal(err)
	}
	tmp.Close()
	defer os.Remove(tmp.Name())

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	demo := New("RF").WithRenderer(&recordingRenderer{})
	demo.RegisterFlags(fs)
	demo.Step("only").ID("only")

	if err := fs.Parse([]string{"--non-interactive", "--record", tmp.Name()}); err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// Sanity: even though os.Args has nothing useful for us, the demo
	// should honour the FlagSet-supplied values.
	os.Args = []string{"test"}

	demo.Execute()

	loaded, err := LoadTrace(tmp.Name())
	if err != nil {
		t.Fatalf("LoadTrace: %v", err)
	}
	if len(loaded) != 1 || loaded[0].StepID != "only" {
		t.Errorf("--record via RegisterFlags did not write trace: %+v", loaded)
	}
}

// TestWaitForEnterOrTimeoutRespectsBudget verifies the function returns
// promptly. In production this exercises the cancelreader-based timeout
// path; in `go test` (where stdin is typically EOF) it returns from the
// stdin-read goroutine itself. Either way the budget should be honored.
//
// The original bug — leaking the stdin goroutine on countdown expiry —
// requires a real terminal stdin to reproduce, so we don't test the
// goroutine count here. The manual repro in examples/graph/ remains
// the fixture for that.
func TestWaitForEnterOrTimeoutRespectsBudget(t *testing.T) {
	_ = runtime.NumGoroutine() // imported only here; keep the import live
	start := time.Now()
	WaitForEnterOrTimeout(50*time.Millisecond, nil)
	elapsed := time.Since(start)
	if elapsed > 500*time.Millisecond {
		t.Errorf("WaitForEnterOrTimeout(50ms) blocked for %v — should be near-instant", elapsed)
	}
}

func TestExecuteCallsRenderer(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"test", "--non-interactive"}

	rec := &recordingRenderer{}
	demo := New("Test Demo").WithRenderer(rec)
	demo.Section("Intro", "hello")
	demo.Step("Do thing").Run(func(ctx StepContext) (result *StepResult) {
		fmt.Print("output")
		return
	})
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

// TestMarkdownGenerationWithoutActors verifies Demo.Markdown() produces
// useful output for a demo with no actors — no panic, and crucially no
// empty mermaid sequenceDiagram block (which doesn't render meaningfully
// without participants). The "## Flow" section is skipped entirely;
// title, notes, steps detail, and run-it footer all still appear.
// Regression for issue 6.
func TestMarkdownGenerationWithoutActors(t *testing.T) {
	d := New("NoActors").Description("d")
	d.Step("only").Note("a note")

	md := d.Markdown()

	if md == "" {
		t.Fatalf("Markdown() returned empty for demo with no actors")
	}
	for _, want := range []string{"# NoActors", "a note", "## Steps"} {
		if !strings.Contains(md, want) {
			t.Errorf("Markdown() missing %q for actor-less demo:\n%s", want, md)
		}
	}
	// No participants ⇒ no sequenceDiagram block. Mermaid renders an
	// empty sequenceDiagram as a broken/unhelpful figure, so we'd
	// rather omit it than emit a placeholder.
	if strings.Contains(md, "sequenceDiagram") {
		t.Errorf("Markdown() should omit the Flow section when no actors are declared:\n%s", md)
	}
	if strings.Contains(md, "## Flow") {
		t.Errorf("Markdown() should omit the Flow header when no actors are declared:\n%s", md)
	}
}

// TestMarkdownVerbatimEmission verifies the static markdown visitor
// emits verbatim blocks as fenced code with optional language hint,
// optional preceding heading, and content preserved byte-exact between
// fences.
func TestMarkdownVerbatimEmission(t *testing.T) {
	d := New("V").Description("d")
	d.Step("only").
		VerbatimLang("Repro on the wire", "bash", "curl -s http://x\n echo $?").
		Verbatim("", "no-label, no-lang") // empty label skips heading; empty lang ⇒ ```\n…\n```

	md := d.Markdown()

	want := []string{
		"#### Repro on the wire",
		"```bash\ncurl -s http://x\n echo $?\n```",
		"```\nno-label, no-lang\n```",
	}
	for _, w := range want {
		if !strings.Contains(md, w) {
			t.Errorf("Markdown missing %q\n--- got ---\n%s", w, md)
		}
	}

	// Empty-label form must NOT emit a #### heading for that block.
	idx := strings.Index(md, "no-label, no-lang")
	if idx < 0 {
		t.Fatalf("verbatim content missing entirely")
	}
	prefix := md[:idx]
	// The last "####" before the empty-label content must be the
	// labeled block's heading — there should be no second one between
	// the labeled fence and the unlabeled fence.
	headingsBefore := strings.Count(prefix, "#### ")
	if headingsBefore != 1 {
		t.Errorf("expected exactly 1 #### heading before unlabeled block, got %d\n%s",
			headingsBefore, md)
	}
}

// TestRenderEntryMDVerbatim verifies the per-entry trace markdown
// renderer also emits verbatim blocks (looked up off the static demo,
// not stored on the TraceEntry — same contract as note/refs).
func TestRenderEntryMDVerbatim(t *testing.T) {
	d := New("V").Description("d")
	d.Step("first").ID("a").
		Note("explainer").
		VerbatimLang("the wire", "json", `{"x":1}`)

	trace := []TraceEntry{
		{Kind: KindStep, Title: "first", StepID: "a", Visit: 1},
	}

	out := RenderEntryMD(RenderContext{Demo: d, Trace: trace}, trace[0], EntryOpts{StepNumber: 1})

	for _, w := range []string{"#### the wire", "```json\n{\"x\":1}\n```"} {
		if !strings.Contains(out, w) {
			t.Errorf("per-entry MD missing %q\n--- got ---\n%s", w, out)
		}
	}
}

// TestVerbatimTraceRoundTrip locks the contract that verbatim blocks,
// living on the static *Demo, survive a JSON-trace serialize/load
// cycle: byte-equal markdown render before and after, since the demo
// definition (where verbatim lives) is unchanged and the trace
// (TraceEntry slice) round-trips through JSON.
func TestVerbatimTraceRoundTrip(t *testing.T) {
	d := New("RT").Description("d")
	d.Step("only").ID("only").
		Note("n").
		VerbatimLang("first", "bash", "echo hi").
		Verbatim("", "second block\nline2")

	trace := []TraceEntry{
		{Kind: KindStep, Title: "only", StepID: "only", Visit: 1, Output: "ok"},
	}

	tmp, err := os.CreateTemp("", "demokit-verbatim-rt-*.json")
	if err != nil {
		t.Fatal(err)
	}
	tmp.Close()
	defer os.Remove(tmp.Name())

	rec := NewJSONFileRecorder(tmp.Name())
	for _, e := range trace {
		rec.Record(e)
	}
	if err := rec.Close(); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadTrace(tmp.Name())
	if err != nil {
		t.Fatal(err)
	}

	before := RenderDocumentMD(RenderContext{Demo: d, Trace: trace})
	after := RenderDocumentMD(RenderContext{Demo: d, Trace: loaded})

	if before != after {
		t.Errorf("markdown render diverges across trace round-trip\n--- before ---\n%s\n--- after ---\n%s",
			before, after)
	}
	// And the verbatim content must actually be present in the rendered
	// markdown (otherwise byte-equality is trivially satisfied by both
	// sides being broken).
	for _, w := range []string{"```bash\necho hi\n```", "second block\nline2"} {
		if !strings.Contains(before, w) {
			t.Errorf("rendered markdown missing %q\n%s", w, before)
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
		Run(func(ctx StepContext) (result *StepResult) { return })

	if s.title != "s" || len(s.arrows) != 2 || len(s.refs) != 1 || s.note != "n" || s.runFn == nil {
		t.Errorf("step chaining failed: %+v", s)
	}
}
