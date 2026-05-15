package notebook

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestBridgeHeaderMsgSetsBanner(t *testing.T) {
	m := New(nil)
	next, _ := m.Update(BridgeHeaderMsg{Title: "My Demo", Description: "hi", StepCount: 5})
	m = next.(Model)
	if m.header != "My Demo" {
		t.Errorf("header = %q, want %q", m.header, "My Demo")
	}
}

func TestBridgeStepCellsMsgAppendsAndSubscribes(t *testing.T) {
	// Seed the model with cells from a prior step so we can assert
	// the new step's cells are appended (trace projection), not
	// substituted.
	prior := []Cell{
		NewMetaCell("s0#0.meta", "Step Zero", "body"),
		NewOutputCell("s0#0.output", NewOutputBuffer(), 6),
	}
	m := New(prior)
	m.cursor = 0
	m.mode = ViewMode

	buf := NewOutputBuffer()
	newCells := []Cell{
		NewMetaCell("s1#0.meta", "Step One", "body"),
		NewOutputCell("s1#0.output", buf, 6),
	}
	next, cmd := m.Update(BridgeStepCellsMsg{Cells: newCells, OutputBuf: buf, OutputCellID: "s1#0.output"})
	m = next.(Model)

	if got := len(m.Cells()); got != 4 {
		t.Fatalf("cell count = %d, want 4 (2 prior + 2 new)", got)
	}
	// Cursor should snap to the first newly-appended cell — the
	// MetaCell of the step that just rendered.
	if got := m.CursorIndex(); got != 2 {
		t.Errorf("cursor = %d, want 2 (first newly-appended cell)", got)
	}
	if m.Mode() != SelectMode {
		t.Errorf("mode = %v, want SelectMode", m.Mode())
	}
	if cmd == nil {
		t.Errorf("BridgeStepCellsMsg with OutputBuf should return a SubscribeOutputBuffer cmd")
	}
}

func TestBridgeWaitMsgStoresChannelAndSpaceClosesIt(t *testing.T) {
	m := New([]Cell{NewMetaCell("only", "T", "")})

	ch := make(chan struct{})
	next, _ := m.Update(BridgeWaitMsg{Ch: ch})
	m = next.(Model)
	if m.waitCh == nil {
		t.Fatal("BridgeWaitMsg did not store the channel")
	}

	// Send Space → channel should close, waitCh cleared.
	next2, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = next2.(Model)
	select {
	case <-ch:
		// expected — channel closed
	default:
		t.Fatal("space did not close the wait channel")
	}
	if m.waitCh != nil {
		t.Error("waitCh should be nil after close")
	}
}

func TestBridgeOutputDoneMsgMarksCellDone(t *testing.T) {
	buf := NewOutputBuffer()
	oc := NewOutputCell("out", buf, 4)
	m := New([]Cell{oc})
	if oc.done {
		t.Fatal("OutputCell should start not-done")
	}
	next, _ := m.Update(BridgeOutputDoneMsg{CellID: "out"})
	_ = next
	if !oc.done {
		t.Error("BridgeOutputDoneMsg did not flip OutputCell.done")
	}
}

func TestBridgeSectionCellMsgAppends(t *testing.T) {
	m := New([]Cell{NewMetaCell("m", "T", "")})
	sc := NewSectionCell("s", "Heads up", "note")
	next, _ := m.Update(BridgeSectionCellMsg{Cell: sc})
	m = next.(Model)
	if len(m.Cells()) != 2 {
		t.Errorf("section append: got %d cells, want 2", len(m.Cells()))
	}
	if m.Cells()[1].ID() != "s" {
		t.Errorf("appended cell ID = %q, want %q", m.Cells()[1].ID(), "s")
	}
}

func TestBridgeDoneMsgFlipsDoneFlag(t *testing.T) {
	m := New(nil)
	next, _ := m.Update(BridgeDoneMsg{})
	m = next.(Model)
	if !m.done {
		t.Error("BridgeDoneMsg did not flip done flag")
	}
}

func TestCellAdvanceMsgPopsAndAdvances(t *testing.T) {
	// Set up: focused on a cell, waiting on a bridge channel
	// (simulates the renderer being mid-WaitForStep).
	cells := []Cell{NewMetaCell("m", "Hi", "")}
	m := New(cells)
	m.cursor = 0
	m.mode = ViewMode
	ch := make(chan struct{})
	m.waitCh = ch

	next, _ := m.Update(cellAdvanceMsg{})
	m = next.(Model)

	if m.Mode() != SelectMode {
		t.Errorf("cellAdvanceMsg should pop to SelectMode; got %v", m.Mode())
	}
	select {
	case <-ch:
		// expected — wait channel closed
	default:
		t.Error("cellAdvanceMsg should have closed the wait channel")
	}
	if m.waitCh != nil {
		t.Error("waitCh should be nil after release")
	}
}
