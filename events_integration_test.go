package demokit

import (
	"fmt"
	"testing"

	"github.com/panyam/demokit/events"
)

// TestExecuteEmitsCanonicalEventSequence runs Execute against a
// tiny synthetic demo (one Section + one Step with output) and
// asserts the event log has the expected shape. This is the
// minimum guard against regressions in the demokit→events bridge:
// when adding a new lifecycle hook, the test will fail loudly if
// the event emission isn't wired up.
func TestExecuteEmitsCanonicalEventSequence(t *testing.T) {
	d := New("Test demo").
		Description("Integration test for event emission")
	d.Section("Intro", "Background context.")
	d.Step("Greet").ID("greet").Note("Print a greeting.").
		Run(func(ctx StepContext) *StepResult {
			fmt.Println("hello, events!")
			return nil
		})

	// Force non-interactive mode so Execute doesn't try to read
	// stdin (which would block in a test). The flag has the same
	// effect as --non-interactive on the CLI.
	d.flagNonInteractive = true
	d.flagsRegistered = true // suppress the os.Args scan
	d.Execute()

	q := d.Events()
	if q == nil {
		t.Fatal("Demo.Events() returned nil after Execute")
	}
	all, _ := q.ReadFrom(0)

	// Expected sequence: Header, Section, StepStart,
	// StepReadyToRun, OutputChunk*, StepEnd, Done. No
	// WaitForAdvance — non-interactive skips the pause.
	want := []string{
		"events.Header",
		"events.Section",
		"events.StepStart",
		"events.StepReadyToRun",
		"events.OutputChunk", // "hello, events!\n"
		"events.StepEnd",
		"events.Done",
	}
	got := make([]string, 0, len(all))
	for _, e := range all {
		got = append(got, fmt.Sprintf("%T", e))
	}
	if len(got) != len(want) {
		t.Fatalf("event count = %d, want %d\n got:  %v\n want: %v", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("event[%d] = %s, want %s", i, got[i], want[i])
		}
	}

	// Spot-check the StepStart payload.
	start, ok := all[2].(events.StepStart)
	if !ok {
		t.Fatalf("event[2] not StepStart: %T", all[2])
	}
	if start.Visit != 1 || start.StepID != "greet" || start.Title != "Greet" {
		t.Errorf("StepStart payload mismatch: %+v", start)
	}

	// Spot-check the OutputChunk content.
	chunk := all[4].(events.OutputChunk)
	if string(chunk.Chunk) != "hello, events!\n" {
		t.Errorf("OutputChunk = %q, want %q", string(chunk.Chunk), "hello, events!\n")
	}

	// Spot-check StepEnd success status.
	end := all[5].(events.StepEnd)
	if end.Status != "ok" {
		t.Errorf("StepEnd.Status = %q, want %q", end.Status, "ok")
	}
}

// TestExecutePromptResolvesViaQueueForLegacy verifies the
// legacy-renderer Prompt path mirrors the resolution into the
// event queue so any future consumer (record, web) sees the
// answers.
func TestExecutePromptResolvesViaQueueForLegacy(t *testing.T) {
	d := New("Prompt demo")
	d.Step("Pick").ID("pick").
		Input(Choice("red", "blue").Named("color", "Color").WithDefault("red"))

	d.flagNonInteractive = true
	d.flagsRegistered = true
	d.Execute()

	all, _ := d.Events().ReadFrom(0)
	promptOffset := -1
	for i := range all {
		if _, ok := all[i].(events.PromptOpen); ok {
			promptOffset = i
			break
		}
	}
	if promptOffset < 0 {
		t.Fatal("no PromptOpen event emitted")
	}
	r, ok := d.Events().Resolution(promptOffset)
	if !ok {
		t.Fatal("PromptOpen not resolved after Execute returns")
	}
	pr, ok := r.(*events.PromptResolution)
	if !ok {
		t.Fatalf("resolution = %T, want *PromptResolution", r)
	}
	if pr.Answers["color"] != "red" {
		t.Errorf("answers[color] = %v, want %q", pr.Answers["color"], "red")
	}
	if pr.Source != "default" {
		t.Errorf("Source = %q, want %q", pr.Source, "default")
	}
}
