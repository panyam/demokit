package notebook

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/panyam/demokit/events"
)

// apply pushes events through applyEvent with sequential offsets
// and returns the mutated model. The starting offset is the
// model's current offset (lets us thread multi-call sequences).
func apply(m Model, evs ...events.Event) Model {
	base := m.offset
	for i, e := range evs {
		m.applyEvent(base+i, e)
	}
	m.offset = base + len(evs)
	return m
}

func TestApplyHeader(t *testing.T) {
	m := New(nil)
	m = apply(m, events.Header{Title: "My Demo", Description: "hi", StepCount: 5})
	if m.header != "My Demo" {
		t.Errorf("header = %q, want %q", m.header, "My Demo")
	}
}

func TestApplySectionAppendsCell(t *testing.T) {
	m := New(nil)
	m = apply(m, events.Section{Title: "Heads up", Body: "note"})
	if len(m.cells) != 1 {
		t.Fatalf("cells len = %d, want 1", len(m.cells))
	}
	if _, ok := m.cells[0].(*SectionCell); !ok {
		t.Errorf("cells[0] = %T, want *SectionCell", m.cells[0])
	}
}

func TestApplyStepStartBuildsCellsAndSnapsCursor(t *testing.T) {
	prior := []Cell{NewMetaCell("p", "Prior", "")}
	m := New(prior)
	m.cursor = 0
	m.mode = ViewMode

	m = apply(m, events.StepStart{
		Visit: 1, StepID: "step.a", Title: "Step A", Note: "body",
		Verbatims: []events.Verbatim{{Label: "code", Variants: []events.Variant{{Content: "x"}}}},
	})
	if got := len(m.cells); got != 3 {
		t.Fatalf("cells = %d, want 3 (1 prior + meta + verbatim)", got)
	}
	if _, ok := m.cells[1].(*MetaCell); !ok {
		t.Errorf("cells[1] = %T, want *MetaCell", m.cells[1])
	}
	if _, ok := m.cells[2].(*VerbatimCell); !ok {
		t.Errorf("cells[2] = %T, want *VerbatimCell", m.cells[2])
	}
	if m.cursor != 1 {
		t.Errorf("cursor = %d, want 1 (first newly-appended)", m.cursor)
	}
	if m.mode != SelectMode {
		t.Errorf("mode = %v, want SelectMode", m.mode)
	}
}

func TestApplyStepReadyToRunInstallsOutputCell(t *testing.T) {
	m := New(nil)
	m = apply(m,
		events.StepStart{Visit: 1, StepID: "s1", Title: "S1"},
		events.StepReadyToRun{Visit: 1},
	)
	if got := m.outputCellByVisit[1]; got == nil {
		t.Fatal("outputCellByVisit[1] is nil after StepReadyToRun")
	}
	tail := m.cells[len(m.cells)-1]
	if _, ok := tail.(*OutputCell); !ok {
		t.Errorf("tail cell = %T, want *OutputCell", tail)
	}
}

func TestApplyOutputChunkRoutesByVisit(t *testing.T) {
	m := New(nil)
	m = apply(m,
		events.StepStart{Visit: 1, StepID: "s1", Title: "S1"},
		events.StepReadyToRun{Visit: 1},
		events.StepStart{Visit: 2, StepID: "s2", Title: "S2"},
		events.StepReadyToRun{Visit: 2},
		events.OutputChunk{Visit: 1, Chunk: []byte("for one\n")},
		events.OutputChunk{Visit: 2, Chunk: []byte("for two\n")},
	)
	if got := m.outputCellByVisit[1].buf.LineCount(); got != 1 {
		t.Errorf("visit-1 lines = %d, want 1", got)
	}
	if got := m.outputCellByVisit[2].buf.LineCount(); got != 1 {
		t.Errorf("visit-2 lines = %d, want 1", got)
	}
}

func TestApplyOutputChunkAfterStepEndStillRoutes(t *testing.T) {
	m := New(nil)
	m = apply(m,
		events.StepStart{Visit: 1, StepID: "s1"},
		events.StepReadyToRun{Visit: 1},
		events.StepEnd{Visit: 1, Status: "ok"},
		events.OutputChunk{Visit: 1, Chunk: []byte("late line\n")},
	)
	oc := m.outputCellByVisit[1]
	if !oc.done {
		t.Error("StepEnd should have flipped MarkDone")
	}
	if got := oc.buf.LineCount(); got != 1 {
		t.Errorf("post-end chunk should still apply; lines = %d, want 1", got)
	}
}

func TestApplyStepEndAppendsErrorLine(t *testing.T) {
	m := New(nil)
	m = apply(m,
		events.StepStart{Visit: 1, StepID: "s1"},
		events.StepReadyToRun{Visit: 1},
		events.StepEnd{Visit: 1, Status: "error", ErrorText: "boom"},
	)
	oc := m.outputCellByVisit[1]
	all := oc.buf.AllLines()
	found := false
	for _, l := range all {
		if l == "[error] boom" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected [error] boom in output buffer; got %v", all)
	}
}

func TestApplyDoneFlipsBanner(t *testing.T) {
	m := New(nil)
	m = apply(m, events.Done{})
	if !m.done {
		t.Error("Done event did not flip done flag")
	}
}

func TestApplyPromptOpenAppendsCellAndFocuses(t *testing.T) {
	m := New(nil).WithQueue(events.NewQueue())
	m = apply(m, events.PromptOpen{
		Visit:  1,
		Inputs: []events.Input{events.NewStringInput("x", "X?", nil)},
	})
	if len(m.cells) != 1 {
		t.Fatalf("cells = %d, want 1", len(m.cells))
	}
	if _, ok := m.cells[0].(*PromptCell); !ok {
		t.Errorf("cells[0] = %T, want *PromptCell", m.cells[0])
	}
	if m.cursor != 0 || m.mode != ViewMode {
		t.Errorf("after prompt open: cursor=%d mode=%v, want 0 + ViewMode", m.cursor, m.mode)
	}
}

func TestApplyWaitForAdvanceTracksOffset(t *testing.T) {
	m := New(nil)
	m = apply(m, events.WaitForAdvance{Visit: 1})
	if m.pendingWaitOffset != 0 {
		t.Errorf("pendingWaitOffset = %d, want 0", m.pendingWaitOffset)
	}
}

func TestEnterResolvesPendingWait(t *testing.T) {
	q := events.NewQueue()
	m := New([]Cell{NewMetaCell("m", "T", "")}).WithQueue(q)

	off := q.Append(events.WaitForAdvance{Visit: 1})
	m.applyEvent(off, events.WaitForAdvance{Visit: 1})

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	got := next.(Model)
	if got.pendingWaitOffset != -1 {
		t.Errorf("pendingWaitOffset = %d, want -1 after resolve", got.pendingWaitOffset)
	}
	r, ok := q.Resolution(off)
	if !ok {
		t.Fatal("WaitForAdvance not resolved after Enter")
	}
	ar, ok := r.(*events.AdvanceResolution)
	if !ok {
		t.Fatalf("resolution = %T, want *AdvanceResolution", r)
	}
	if ar.Source != "user-enter" {
		t.Errorf("Resolution.Source = %q, want %q", ar.Source, "user-enter")
	}
}

func TestEventsAvailableDrainsAndApplies(t *testing.T) {
	q := events.NewQueue()
	m := New(nil).WithQueue(q)

	q.Append(events.Header{Title: "Demo"})
	q.Append(events.StepStart{Visit: 1, StepID: "s1", Title: "S1"})

	next, _ := m.Update(eventsAvailableMsg{})
	m = next.(Model)
	if m.header != "Demo" {
		t.Errorf("header = %q, want Demo", m.header)
	}
	if len(m.cells) != 1 {
		t.Errorf("cells = %d, want 1 (Meta from StepStart)", len(m.cells))
	}
	if m.offset != 2 {
		t.Errorf("offset = %d, want 2", m.offset)
	}
}
