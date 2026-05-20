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
			SelectMode: {"j": mAction},
			custom:     {"x": mAction},
		},
	}
	// Global matches in any mode.
	if got := km.lookup(SelectMode, "ctrl+c"); got == nil {
		t.Error("global ctrl+c missing in SelectMode")
	}
	if got := km.lookup(custom, "ctrl+c"); got == nil {
		t.Error("global ctrl+c missing in custom mode")
	}
	// Mode-specific matches only in their mode.
	if got := km.lookup(SelectMode, "j"); got == nil {
		t.Error("SelectMode j missing")
	}
	if got := km.lookup(custom, "j"); got != nil {
		t.Error("SelectMode j leaked into custom mode")
	}
	// Unknown keys return nil.
	if got := km.lookup(SelectMode, "z"); got != nil {
		t.Error("unknown key z should not match")
	}
}

func TestDefaultKeyMapDoesNotBindQ(t *testing.T) {
	km := DefaultKeyMap()
	if km.lookup(SelectMode, "q") != nil {
		t.Error("default KeyMap should NOT bind 'q' (apps add it explicitly)")
	}
	if km.lookup(SelectMode, "ctrl+c") == nil {
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

func (c *fallthroughCell) ID() string              { return c.id }
func (c *fallthroughCell) HeightHint(int) int      { return 1 }
func (c *fallthroughCell) StatusHint(Mode) string  { return "" }
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

	// 'j' in SelectMode: cell sees it (passthrough), then KeyMap
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

func TestReleaseFocusReturnsToSelectMode(t *testing.T) {
	m, st := newSizedModel(t, 40, 24)
	st.insert(-1, &fallthroughCell{id: "a"})
	m.mode = ViewMode

	next, _ := m.Update(ReleaseFocusMsg{})
	got := next.(model)
	if got.mode != SelectMode {
		t.Errorf("mode after ReleaseFocusMsg = %v, want SelectMode", got.mode)
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
	m.mode = ViewMode

	next, _ := m.Update(CellAdvanceMsg{})
	got := next.(model)
	if got.mode != SelectMode {
		t.Errorf("mode after CellAdvance = %v, want SelectMode", got.mode)
	}
	if got := st.cursorPos(); got != 1 {
		t.Errorf("cursor after CellAdvance = %d, want 1", got)
	}
}

func TestPromptSubmittedExitsViewModeWithoutMovingCursor(t *testing.T) {
	m, st := newSizedModel(t, 40, 24)
	st.insert(-1, &fallthroughCell{id: "a"})
	st.insert(-1, &fallthroughCell{id: "b"})
	m.mode = ViewMode
	// Cursor at "a" (idx 0).
	next, _ := m.Update(PromptSubmittedMsg{CellID: "a", Answers: map[string]any{}})
	got := next.(model)
	if got.mode != SelectMode {
		t.Errorf("mode = %v, want SelectMode", got.mode)
	}
	if c := st.cursorPos(); c != 0 {
		t.Errorf("cursor = %d, want 0 (PromptSubmitted must NOT move cursor)", c)
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
		store:   newStore(),
		rdv:     newRendezvous(),
		keymap:  km,
		ready:   make(chan struct{}),
		stopped: make(chan struct{}),
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
	m.mode = SelectMode
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if fired {
		t.Error("custom mode binding fired in wrong mode")
	}
}
