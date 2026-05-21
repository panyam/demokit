package notebook

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// click is a 1-press tea.MouseMsg helper used by the mouse-customization
// tests below. Mirrors what bubbletea sends on a press event.
func click(btn tea.MouseButton, y int) tea.MouseMsg {
	return tea.MouseMsg{Action: tea.MouseActionPress, Button: btn, Y: y}
}

// drain runs cmd (if any) and feeds its msg back through the model
// so test assertions can read the final state. Used to validate
// async-mode-change paths like ClickActivate → setModeMsg.
func drain(t *testing.T, m tea.Model, cmd tea.Cmd) tea.Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	msg := cmd()
	if msg == nil {
		return m
	}
	next, _ := m.Update(msg)
	return next
}

func TestDefaultMouseConfigUsesClickActivate(t *testing.T) {
	m, st := newSizedModel(t, 40, 30)
	st.insert(-1, &fallthroughCell{id: "a"})
	st.insert(-1, &fallthroughCell{id: "b"})

	next, cmd := m.Update(click(tea.MouseButtonLeft, 1))
	if st.cursorPos() != 1 {
		t.Errorf("default OnClick should move cursor to clicked cell; cursor = %d", st.cursorPos())
	}
	got := drain(t, next, cmd).(model)
	if got.mode != CellActiveMode {
		t.Errorf("default OnClick should switch to CellActiveMode; mode = %v", got.mode)
	}
}

func TestClickCursorOnlyMovesCursorWithoutModeChange(t *testing.T) {
	m, st := newSizedModel(t, 40, 30)
	m.nb.mouseConfig = MouseConfig{
		OnClick: func(nb *Notebook, ctx MouseContext) tea.Cmd {
			if ctx.Button == tea.MouseButtonLeft {
				return ClickCursorOnly(nb, ctx)
			}
			return nil
		},
		OnWheelFallback: WheelNavCursor,
	}
	st.insert(-1, &fallthroughCell{id: "a"})
	st.insert(-1, &fallthroughCell{id: "b"})

	startMode := m.mode
	next, cmd := m.Update(click(tea.MouseButtonLeft, 1))
	if st.cursorPos() != 1 {
		t.Errorf("ClickCursorOnly should move cursor; cursor = %d", st.cursorPos())
	}
	if cmd != nil {
		t.Errorf("ClickCursorOnly should not return a mode-changing cmd; got non-nil cmd")
	}
	if next.(model).mode != startMode {
		t.Errorf("ClickCursorOnly should not change mode; mode = %v, want %v", next.(model).mode, startMode)
	}
}

func TestOnClickRightButtonInvokesCustomHandler(t *testing.T) {
	m, st := newSizedModel(t, 40, 30)
	var rightClicked CellID
	m.nb.mouseConfig = MouseConfig{
		OnClick: func(nb *Notebook, ctx MouseContext) tea.Cmd {
			if ctx.Button == tea.MouseButtonRight {
				rightClicked = ctx.CellID
				return nil
			}
			return DefaultOnClick(nb, ctx)
		},
		OnWheelFallback: WheelNavCursor,
	}
	st.insert(-1, &fallthroughCell{id: "a"})
	st.insert(-1, &fallthroughCell{id: "b"})

	m.Update(click(tea.MouseButtonRight, 1))
	if rightClicked != "b" {
		t.Errorf("right-click handler saw CellID = %q, want %q", rightClicked, "b")
	}
	if st.cursorPos() != 0 {
		t.Errorf("right-click should not move cursor; cursor = %d", st.cursorPos())
	}
}

func TestNilOnClickIsSafelyNoop(t *testing.T) {
	m, st := newSizedModel(t, 40, 30)
	m.nb.mouseConfig.OnClick = nil
	st.insert(-1, &fallthroughCell{id: "a"})
	st.insert(-1, &fallthroughCell{id: "b"})

	next, cmd := m.Update(click(tea.MouseButtonLeft, 1))
	if st.cursorPos() != 0 || cmd != nil {
		t.Errorf("nil OnClick should be a no-op; cursor=%d cmd!=nil=%v", st.cursorPos(), cmd != nil)
	}
	_ = next
}

func TestWheelFallbackInvokedWhenCellPassesThrough(t *testing.T) {
	m, st := newSizedModel(t, 40, 24)
	called := false
	m.nb.mouseConfig = MouseConfig{
		OnClick: DefaultOnClick,
		OnWheelFallback: func(nb *Notebook, ctx MouseContext) tea.Cmd {
			called = true
			return nil
		},
	}
	st.insert(-1, &fallthroughCell{id: "a"})
	st.insert(-1, &fallthroughCell{id: "b"})

	m.Update(click(tea.MouseButtonWheelDown, 0))
	if !called {
		t.Error("OnWheelFallback should fire when cell passes wheel through")
	}
}

func TestWheelFallbackSkippedWhenCellClaims(t *testing.T) {
	m, st := newSizedModel(t, 40, 24)
	called := false
	m.nb.mouseConfig = MouseConfig{
		OnClick: DefaultOnClick,
		OnWheelFallback: func(nb *Notebook, ctx MouseContext) tea.Cmd {
			called = true
			return nil
		},
	}
	st.insert(-1, &wheelClaimingCell{fallthroughCell{id: "a"}})
	st.insert(-1, &fallthroughCell{id: "b"})

	m.Update(click(tea.MouseButtonWheelDown, 0))
	if called {
		t.Error("OnWheelFallback should NOT fire when the cursor cell claimed the wheel event")
	}
}

func TestMouseContextResolvesCellAtRow(t *testing.T) {
	m, st := newSizedModel(t, 40, 30)
	var got MouseContext
	m.nb.mouseConfig = MouseConfig{
		OnClick: func(_ *Notebook, ctx MouseContext) tea.Cmd { got = ctx; return nil },
	}
	st.insert(-1, &fallthroughCell{id: "a"})
	st.insert(-1, &fallthroughCell{id: "b"})
	st.insert(-1, &fallthroughCell{id: "c"})

	m.Update(click(tea.MouseButtonLeft, 2))
	if got.CellID != "c" || got.CellIndex != 2 {
		t.Errorf("MouseContext at Y=2 → CellID=%q CellIndex=%d, want \"c\"/2", got.CellID, got.CellIndex)
	}

	// Click past the last cell — outside the cell rows.
	got = MouseContext{}
	m.Update(click(tea.MouseButtonLeft, 20))
	if got.CellID != "" || got.CellIndex != -1 {
		t.Errorf("click outside cells should produce empty CellID, -1 CellIndex; got %q/%d", got.CellID, got.CellIndex)
	}
}
