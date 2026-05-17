package notebook

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/panyam/demokit"
)

// apply is a tiny helper: pass an event through applyEvent and
// return the mutated model. Pointer receiver under the hood, so
// we re-fetch the value.
func apply(m Model, events ...Event) Model {
	for _, e := range events {
		m.applyEvent(e)
	}
	return m
}

func TestApplyEventHeader(t *testing.T) {
	m := New(nil)
	m = apply(m, eventHeader{Title: "My Demo", Description: "hi", StepCount: 5})
	if m.header != "My Demo" {
		t.Errorf("header = %q, want %q", m.header, "My Demo")
	}
}

func TestApplyEventSectionAppendsCell(t *testing.T) {
	m := New(nil)
	m = apply(m, eventSection{Title: "Heads up", Body: "note"})
	if len(m.cells) != 1 {
		t.Fatalf("cells len = %d, want 1", len(m.cells))
	}
	if _, ok := m.cells[0].(*SectionCell); !ok {
		t.Errorf("cells[0] = %T, want *SectionCell", m.cells[0])
	}
}

func TestApplyEventStepStartAppendsBodyAndSnapsCursor(t *testing.T) {
	prior := []Cell{NewMetaCell("p", "Prior", "")}
	m := New(prior)
	m.cursor = 0
	m.mode = ViewMode

	newCells := []Cell{
		NewMetaCell("s1#0.meta", "Step One", "body"),
		NewVerbatimCell("s1#0.v0", "L", []demokit.Variant{{Content: "x"}}),
	}
	m = apply(m, eventStepStart{Visit: 1, StepID: "s1", BodyCells: newCells})

	if got := len(m.cells); got != 3 {
		t.Fatalf("cells len = %d, want 3 (1 prior + 2 new)", got)
	}
	if m.cursor != 1 {
		t.Errorf("cursor = %d, want 1 (first newly-appended)", m.cursor)
	}
	if m.mode != SelectMode {
		t.Errorf("mode = %v, want SelectMode after step start", m.mode)
	}
}

func TestApplyEventStepReadyToRunInstallsOutputCell(t *testing.T) {
	m := New(nil)
	buf := NewOutputBuffer()
	oc := NewOutputCell("s1#0.output", buf, 6)
	m = apply(m, eventStepReadyToRun{Visit: 1, Output: oc, OutputBuf: buf})

	if len(m.cells) != 1 {
		t.Fatalf("cells len = %d, want 1", len(m.cells))
	}
	if got := m.outputCellByVisit[1]; got != oc {
		t.Errorf("outputCellByVisit[1] = %v, want %v", got, oc)
	}
}

func TestApplyEventOutputChunkRoutesByVisit(t *testing.T) {
	m := New(nil)
	buf1 := NewOutputBuffer()
	oc1 := NewOutputCell("s1#0.output", buf1, 6)
	buf2 := NewOutputBuffer()
	oc2 := NewOutputCell("s2#0.output", buf2, 6)
	m = apply(m,
		eventStepReadyToRun{Visit: 1, Output: oc1, OutputBuf: buf1},
		eventStepReadyToRun{Visit: 2, Output: oc2, OutputBuf: buf2},
		eventOutputChunk{Visit: 1, Chunk: []byte("for one\n")},
		eventOutputChunk{Visit: 2, Chunk: []byte("for two\n")},
	)
	if got := buf1.LineCount(); got != 1 {
		t.Errorf("buf1 lines = %d, want 1", got)
	}
	if got := buf2.LineCount(); got != 1 {
		t.Errorf("buf2 lines = %d, want 1", got)
	}
	if l := buf1.Lines(0, 1); len(l) != 1 || l[0] != "for one" {
		t.Errorf("buf1 contents = %v, want [for one]", l)
	}
}

func TestApplyEventOutputChunkAfterStepEndStillRoutes(t *testing.T) {
	// A step's Run may spawn a background goroutine that keeps
	// emitting chunks after the step ends — live graph use case.
	// MarkDone is a label, not a seal.
	m := New(nil)
	buf := NewOutputBuffer()
	oc := NewOutputCell("s1#0.output", buf, 6)
	m = apply(m,
		eventStepReadyToRun{Visit: 1, Output: oc, OutputBuf: buf},
		eventStepEnd{Visit: 1},
		eventOutputChunk{Visit: 1, Chunk: []byte("late line\n")},
	)
	if !oc.done {
		t.Error("eventStepEnd should have flipped MarkDone")
	}
	if got := buf.LineCount(); got != 1 {
		t.Errorf("post-end chunk should still apply; lines = %d, want 1", got)
	}
}

func TestApplyEventStepEndAppendsErrorLine(t *testing.T) {
	m := New(nil)
	buf := NewOutputBuffer()
	oc := NewOutputCell("s1#0.output", buf, 6)
	m = apply(m,
		eventStepReadyToRun{Visit: 1, Output: oc, OutputBuf: buf},
		eventStepEnd{Visit: 1, Result: demokit.Errf("boom")},
	)
	lines := buf.AllLines()
	found := false
	for _, l := range lines {
		if l == "[error] boom" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected [error] boom in output buffer; got %v", lines)
	}
}

func TestApplyEventDoneFlipsBanner(t *testing.T) {
	m := New(nil)
	m = apply(m, eventDone{})
	if !m.done {
		t.Error("eventDone did not flip done flag")
	}
}

func TestApplyEventPromptOpenAppendsAndFocuses(t *testing.T) {
	m := New(nil)
	reply := make(chan map[string]any, 1)
	m = apply(m, eventPromptOpen{Visit: 1, Inputs: nil, Reply: reply})
	if len(m.cells) != 1 {
		t.Fatalf("cells len = %d, want 1", len(m.cells))
	}
	if _, ok := m.cells[0].(*PromptCell); !ok {
		t.Errorf("cells[0] = %T, want *PromptCell", m.cells[0])
	}
	if m.cursor != 0 || m.mode != ViewMode {
		t.Errorf("after prompt open: cursor=%d mode=%v, want 0 + ViewMode", m.cursor, m.mode)
	}
}

func TestApplyEventWaitForAdvanceStoresChannel(t *testing.T) {
	m := New(nil)
	ch := make(chan struct{})
	m = apply(m, eventWaitForAdvance{Visit: 1, Done: ch})
	if m.waitCh == nil {
		t.Error("waitCh should be set after eventWaitForAdvance")
	}
}

// --- End-to-end queue test: events appended → drained → applied ---

func TestEventsAvailableMsgDrainsAndApplies(t *testing.T) {
	q := newEventQueue()
	m := New(nil).WithQueue(q)

	q.Append(eventHeader{Title: "Demo"})
	q.Append(eventStepStart{Visit: 1, StepID: "s1", BodyCells: []Cell{NewMetaCell("s1#0.meta", "Step One", "")}})

	next, _ := m.Update(eventsAvailableMsg{})
	m = next.(Model)

	if m.header != "Demo" {
		t.Errorf("after drain: header = %q, want %q", m.header, "Demo")
	}
	if len(m.cells) != 1 {
		t.Errorf("after drain: cells = %d, want 1", len(m.cells))
	}
	if m.offset != 2 {
		t.Errorf("after drain: offset = %d, want 2", m.offset)
	}
}

func TestEventsAvailableMsgPreservesPriorOffset(t *testing.T) {
	// Confirm incremental drain — second eventsAvailableMsg only
	// applies events appended since the first drain.
	q := newEventQueue()
	m := New(nil).WithQueue(q)

	q.Append(eventHeader{Title: "A"})
	next, _ := m.Update(eventsAvailableMsg{})
	m = next.(Model)
	if m.offset != 1 {
		t.Fatalf("after first drain: offset = %d, want 1", m.offset)
	}

	q.Append(eventDone{})
	next, _ = m.Update(eventsAvailableMsg{})
	m = next.(Model)
	if m.offset != 2 {
		t.Errorf("after second drain: offset = %d, want 2", m.offset)
	}
	if !m.done {
		t.Error("after second drain: done flag should be set")
	}
}

// keypress helper that re-asserts to Model after Update.
func updateAndAssert(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	next, _ := m.Update(msg)
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want Model", next)
	}
	return got
}

func TestEnterReleasesWaitChannel(t *testing.T) {
	m := New([]Cell{NewMetaCell("only", "T", "")})
	ch := make(chan struct{})
	m.applyEvent(eventWaitForAdvance{Visit: 1, Done: ch})

	m = updateAndAssert(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	select {
	case <-ch:
		// expected — channel closed
	default:
		t.Error("Enter did not close the wait channel")
	}
	if m.waitCh != nil {
		t.Error("waitCh should be nil after release")
	}
}
