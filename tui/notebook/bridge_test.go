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

func TestBridgeStepCellsMsgReplacesAndSubscribes(t *testing.T) {
	m := New(nil)
	// Move-then-focus before the bridge fires.
	m.cursor = 99
	m.mode = ViewMode

	buf := NewOutputBuffer()
	cells := []Cell{
		NewMetaCell("s1#0.meta", "Step One", "body"),
		NewOutputCell("s1#0.output", buf, 6),
	}
	next, cmd := m.Update(BridgeStepCellsMsg{Cells: cells, OutputBuf: buf, OutputCellID: "s1#0.output"})
	m = next.(Model)

	if got := len(m.Cells()); got != 2 {
		t.Fatalf("cell count = %d, want 2", got)
	}
	if m.CursorIndex() != 0 || m.Mode() != SelectMode {
		t.Errorf("BridgeStepCellsMsg did not reset cursor/mode: cursor=%d mode=%v", m.CursorIndex(), m.Mode())
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
