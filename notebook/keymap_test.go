package notebook

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestKeyMapLookupGlobalFirstThenMode(t *testing.T) {
	gAction := func(*Notebook) tea.Cmd { return nil }
	mAction := func(*Notebook) tea.Cmd { return nil }
	custom := NewMode("CUSTOM")
	km := KeyMap{
		Global: map[string]Action{"ctrl+c": gAction},
		Modes: map[Mode]map[string]Action{
			NavigationMode: {"j": mAction},
			custom:         {"x": mAction},
		},
	}
	// Global matches in any mode.
	if got := km.lookup(NavigationMode, "ctrl+c"); got == nil {
		t.Error("global ctrl+c missing in NavigationMode")
	}
	if got := km.lookup(custom, "ctrl+c"); got == nil {
		t.Error("global ctrl+c missing in custom mode")
	}
	// Mode-specific matches only in their mode.
	if got := km.lookup(NavigationMode, "j"); got == nil {
		t.Error("NavigationMode j missing")
	}
	if got := km.lookup(custom, "j"); got != nil {
		t.Error("NavigationMode j leaked into custom mode")
	}
	// Unknown keys return nil.
	if got := km.lookup(NavigationMode, "z"); got != nil {
		t.Error("unknown key z should not match")
	}
}

func TestDefaultKeyMapDoesNotBindQ(t *testing.T) {
	km := DefaultKeyMap()
	if km.lookup(NavigationMode, "q") != nil {
		t.Error("default KeyMap should NOT bind 'q' (apps add it explicitly)")
	}
	if km.lookup(NavigationMode, "ctrl+c") == nil {
		t.Error("default KeyMap MUST bind ctrl+c")
	}
}

func TestNewModeProducesDistinctValues(t *testing.T) {
	a := NewMode("X")
	b := NewMode("X")
	if a == b {
		t.Error("two NewMode(\"X\") calls produced equal values; should be distinct")
	}
	if a.Name() != "X" || b.Name() != "X" {
		t.Errorf("Name() = %q,%q; want X,X", a.Name(), b.Name())
	}
}

// Cell that records every key and never claims any, used to
// confirm the cell-first dispatch and the passthrough path.
type fallthroughCell struct {
	id      string
	gotKeys []string
}

func (c *fallthroughCell) ID() string             { return c.id }
func (c *fallthroughCell) HeightHint(int) int     { return 1 }
func (c *fallthroughCell) StatusHint(Mode) string { return "" }
func (c *fallthroughCell) RenderRows(int, int, int, bool, Mode) []string {
	return []string{c.id}
}
func (c *fallthroughCell) Update(msg tea.Msg, _ Mode) (Cell, tea.Cmd, bool) {
	if k, ok := msg.(tea.KeyMsg); ok {
		c.gotKeys = append(c.gotKeys, k.String())
	}
	return c, nil, false
}

// Cell that claims every KeyMsg (handled=true) — used to confirm
// KeyMap is suppressed when the cell handles the key.
type claimingCell struct{ fallthroughCell }

func (c *claimingCell) Update(msg tea.Msg, _ Mode) (Cell, tea.Cmd, bool) {
	if k, ok := msg.(tea.KeyMsg); ok {
		c.gotKeys = append(c.gotKeys, k.String())
		return c, nil, true
	}
	return c, nil, false
}

func TestCellFirstThenKeyMapWhenPassthrough(t *testing.T) {
	m, st := newSizedModel(t, 40, 24)
	cell := &fallthroughCell{id: "a"}
	st.insert(-1, cell)

	// 'j' in NavigationMode: cell sees it (passthrough), then KeyMap
	// fires NavDown — but only one cell, so cursor stays clamped.
	// Better: add a second cell so NavDown has somewhere to go.
	st.insert(-1, &fallthroughCell{id: "b"})

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if len(cell.gotKeys) != 1 || cell.gotKeys[0] != "j" {
		t.Errorf("cell.gotKeys = %v, want [j]", cell.gotKeys)
	}
	if got := st.cursorPos(); got != 1 {
		t.Errorf("cursor = %d, want 1 (j fell through to NavDown)", got)
	}
}

func TestCellHandledSuppressesKeyMap(t *testing.T) {
	m, st := newSizedModel(t, 40, 24)
	cell := &claimingCell{fallthroughCell{id: "a"}}
	st.insert(-1, cell)
	st.insert(-1, &fallthroughCell{id: "b"})

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if len(cell.gotKeys) != 1 || cell.gotKeys[0] != "j" {
		t.Errorf("cell.gotKeys = %v, want [j]", cell.gotKeys)
	}
	if got := st.cursorPos(); got != 0 {
		t.Errorf("cursor = %d, want 0 (cell claimed j, NavDown suppressed)", got)
	}
}

func TestReleaseFocusReturnsToNavigationMode(t *testing.T) {
	m, st := newSizedModel(t, 40, 24)
	st.insert(-1, &fallthroughCell{id: "a"})
	m.mode = CellActiveMode

	next, _ := m.Update(ReleaseFocusMsg{})
	got := next.(model)
	if got.mode != NavigationMode {
		t.Errorf("mode after ReleaseFocusMsg = %v, want NavigationMode", got.mode)
	}
}

func TestSetModeMsgChangesMode(t *testing.T) {
	m, _ := newSizedModel(t, 40, 24)
	custom := NewMode("CUSTOM")
	next, _ := m.Update(setModeMsg{mode: custom})
	got := next.(model)
	if got.mode != custom {
		t.Errorf("mode after setModeMsg = %v, want CUSTOM", got.mode)
	}
}

func TestCellAdvanceMovesCursorAndExits(t *testing.T) {
	m, st := newSizedModel(t, 40, 24)
	st.insert(-1, &fallthroughCell{id: "a"})
	st.insert(-1, &fallthroughCell{id: "b"})
	m.mode = CellActiveMode

	next, _ := m.Update(CellAdvanceMsg{})
	got := next.(model)
	if got.mode != NavigationMode {
		t.Errorf("mode after CellAdvance = %v, want NavigationMode", got.mode)
	}
	if got := st.cursorPos(); got != 1 {
		t.Errorf("cursor after CellAdvance = %d, want 1", got)
	}
}

func TestPromptSubmittedExitsCellActiveModeWithoutMovingCursor(t *testing.T) {
	m, st := newSizedModel(t, 40, 24)
	st.insert(-1, &fallthroughCell{id: "a"})
	st.insert(-1, &fallthroughCell{id: "b"})
	m.mode = CellActiveMode
	// Cursor at "a" (idx 0).
	next, _ := m.Update(PromptSubmittedMsg{CellID: "a", Answers: map[string]any{}})
	got := next.(model)
	if got.mode != NavigationMode {
		t.Errorf("mode = %v, want NavigationMode", got.mode)
	}
	if c := st.cursorPos(); c != 0 {
		t.Errorf("cursor = %d, want 0 (PromptSubmitted must NOT move cursor)", c)
	}
}

func TestMouseWheelMovesCursor(t *testing.T) {
	m, st := newSizedModel(t, 40, 24)
	st.insert(-1, &fallthroughCell{id: "a"})
	st.insert(-1, &fallthroughCell{id: "b"})
	st.insert(-1, &fallthroughCell{id: "c"})

	m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	if got := st.cursorPos(); got != 2 {
		t.Errorf("cursor after 2 wheel-down = %d, want 2", got)
	}
	m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelUp})
	if got := st.cursorPos(); got != 1 {
		t.Errorf("cursor after wheel-up = %d, want 1", got)
	}
}

func TestMouseLeftClickSelectsCellAndEntersCellActiveMode(t *testing.T) {
	m, st := newSizedModel(t, 40, 30)
	// Three 1-row cells: a@row 0, b@row 1, c@row 2. No header.
	st.insert(-1, &fallthroughCell{id: "a"})
	st.insert(-1, &fallthroughCell{id: "b"})
	st.insert(-1, &fallthroughCell{id: "c"})

	// ClickActivate sets cursor synchronously (via store) and
	// returns a cmd that emits setModeMsg for the mode change.
	next, cmd := m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, Y: 2})
	if pos := st.cursorPos(); pos != 2 {
		t.Errorf("click at Y=2 → cursor = %d, want 2", pos)
	}
	if cmd == nil {
		t.Fatal("ClickActivate returned nil cmd; want a setModeMsg-emitting cmd")
	}
	next, _ = next.Update(cmd())
	got := next.(model)
	if got.mode != CellActiveMode {
		t.Errorf("after draining cmd: mode = %v, want CellActiveMode", got.mode)
	}
}

// Cell that claims mouse wheel events — used to verify cell-first
// wheel routing.
type wheelClaimingCell struct{ fallthroughCell }

func (c *wheelClaimingCell) Update(msg tea.Msg, _ Mode) (Cell, tea.Cmd, bool) {
	if mouse, ok := msg.(tea.MouseMsg); ok {
		if mouse.Button == tea.MouseButtonWheelUp || mouse.Button == tea.MouseButtonWheelDown {
			return c, nil, true
		}
	}
	return c, nil, false
}

func TestMouseWheelRoutedCellFirstHandledSuppressesNav(t *testing.T) {
	m, st := newSizedModel(t, 40, 24)
	st.insert(-1, &wheelClaimingCell{fallthroughCell{id: "a"}})
	st.insert(-1, &fallthroughCell{id: "b"})

	m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	if got := st.cursorPos(); got != 0 {
		t.Errorf("cursor moved despite cell claiming wheel; cursor = %d, want 0", got)
	}
}

func TestMouseWheelFallsThroughToNavWhenCellPassesThrough(t *testing.T) {
	m, st := newSizedModel(t, 40, 24)
	st.insert(-1, &fallthroughCell{id: "a"})
	st.insert(-1, &fallthroughCell{id: "b"})

	m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	if got := st.cursorPos(); got != 1 {
		t.Errorf("cursor after wheel passthrough = %d, want 1 (nav fallback)", got)
	}
}

func TestMouseClickOutsideBodyIsNoop(t *testing.T) {
	m, st := newSizedModel(t, 40, 8)
	st.insert(-1, &fallthroughCell{id: "a"})
	st.insert(-1, &fallthroughCell{id: "b"})
	st.moveCursor(+1) // cursor on b
	// Click on the status row (Y = height - 1 = 7).
	m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, Y: 7})
	if got := st.cursorPos(); got != 1 {
		t.Errorf("status-row click moved cursor (= %d, want 1 unchanged)", got)
	}
}

func TestMouseReleaseIsIgnored(t *testing.T) {
	m, st := newSizedModel(t, 40, 24)
	st.insert(-1, &fallthroughCell{id: "a"})
	st.insert(-1, &fallthroughCell{id: "b"})
	m.Update(tea.MouseMsg{Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft, Y: 0})
	if got := st.cursorPos(); got != 0 {
		t.Errorf("release event affected cursor (= %d, want 0 unchanged)", got)
	}
}

func TestCustomModeBindings(t *testing.T) {
	custom := NewMode("CUSTOM")
	fired := false
	km := KeyMap{
		Global: map[string]Action{"ctrl+c": Quit},
		Modes: map[Mode]map[string]Action{
			custom: {"x": func(*Notebook) tea.Cmd { fired = true; return nil }},
		},
	}
	nb := &Notebook{
		store:       newStore(),
		rdv:         newRendezvous(),
		keymap:      km,
		mouseConfig: DefaultMouseConfig(),
		ready:       make(chan struct{}),
		stopped:     make(chan struct{}),
	}
	nb.store.insert(-1, &fallthroughCell{id: "a"})
	m := newModel(nb, 40, 24)
	m.mode = custom

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if !fired {
		t.Error("custom mode binding for 'x' did not fire")
	}
	// Wrong mode: shouldn't fire.
	fired = false
	m.mode = NavigationMode
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if fired {
		t.Error("custom mode binding fired in wrong mode")
	}
}
