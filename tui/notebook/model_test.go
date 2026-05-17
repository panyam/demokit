package notebook

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/panyam/demokit"
)

// sendKey is a tiny helper that pushes a single key through the
// model's Update and returns the updated model. tea.Model.Update
// returns the interface — we re-assert to Model for ergonomic test
// code.
func sendKey(t *testing.T, m Model, key string) Model {
	t.Helper()
	var km tea.KeyMsg
	switch key {
	case "up":
		km = tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		km = tea.KeyMsg{Type: tea.KeyDown}
	case "enter":
		km = tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		km = tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		km = tea.KeyMsg{Type: tea.KeyTab}
	case "space":
		km = tea.KeyMsg{Type: tea.KeySpace}
	default:
		km = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}
	next, _ := m.Update(km)
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want Model", next)
	}
	return got
}

// makeThreeCells constructs a small representative cell list: Meta,
// Verbatim (multi-variant), Output. Used by most model tests.
func makeThreeCells() []Cell {
	return []Cell{
		NewMetaCell("step.intro#0.meta", "Get a token", "Authorize against the IdP."),
		NewVerbatimCell("step.intro#0.verbatim0", "Fetch", []demokit.Variant{
			{Label: "curl", Content: "curl ...", IsDefault: true},
			{Label: "python", Content: "import requests"},
		}),
		NewOutputCell("step.intro#0.output", func() *OutputBuffer {
			b := NewOutputBuffer()
			b.Append([]byte("starting...\n"))
			return b
		}(), 6),
	}
}

func TestModelInitialState(t *testing.T) {
	m := New(makeThreeCells())
	if m.CursorIndex() != 0 {
		t.Errorf("cursor = %d, want 0", m.CursorIndex())
	}
	if m.Mode() != SelectMode {
		t.Errorf("mode = %v, want SelectMode", m.Mode())
	}
}

func TestModelArrowsMoveCursor(t *testing.T) {
	m := New(makeThreeCells())
	m = sendKey(t, m, "down")
	if m.CursorIndex() != 1 {
		t.Errorf("after down, cursor = %d, want 1", m.CursorIndex())
	}
	m = sendKey(t, m, "down")
	m = sendKey(t, m, "down") // bounded
	if m.CursorIndex() != 2 {
		t.Errorf("cursor should clamp at last cell; got %d, want 2", m.CursorIndex())
	}
	m = sendKey(t, m, "up")
	if m.CursorIndex() != 1 {
		t.Errorf("after up, cursor = %d, want 1", m.CursorIndex())
	}
}

func TestModelSFocusesEscReleases(t *testing.T) {
	m := New(makeThreeCells())
	m = sendKey(t, m, "down") // onto VerbatimCell
	m = sendKey(t, m, "s")
	if m.Mode() != ViewMode {
		t.Errorf("after s, mode = %v, want ViewMode", m.Mode())
	}
	m = sendKey(t, m, "esc")
	if m.Mode() != SelectMode {
		t.Errorf("after esc, mode = %v, want SelectMode", m.Mode())
	}
}

func TestModelFAlsoFocuses(t *testing.T) {
	// "f" is the alternate focus mnemonic alongside "s". Both
	// should reach ViewMode identically.
	m := New(makeThreeCells())
	m = sendKey(t, m, "down")
	m = sendKey(t, m, "f")
	if m.Mode() != ViewMode {
		t.Errorf("after f, mode = %v, want ViewMode", m.Mode())
	}
}

func TestModelFocusedKeysReachCell(t *testing.T) {
	cells := makeThreeCells()
	m := New(cells)
	m = sendKey(t, m, "down") // onto VerbatimCell
	m = sendKey(t, m, "s")    // step into → ViewMode
	m = sendKey(t, m, "tab")  // cycle variant
	vc := m.Cells()[1].(*VerbatimCell)
	if vc.active != 1 {
		t.Errorf("Tab through model did not reach VerbatimCell; active = %d", vc.active)
	}
}

func TestModelEnterAdvanceEmitsAdvanceMsg(t *testing.T) {
	// Enter is the universal advance key (matches PlainRenderer).
	m := New(makeThreeCells())
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter in SelectMode should return a tea.Cmd")
	}
	if msg := cmd(); msg == nil {
		t.Fatal("advance cmd returned nil msg")
	} else if _, ok := msg.(AdvanceMsg); !ok {
		t.Errorf("advance cmd returned %T, want AdvanceMsg", msg)
	}
}

func TestModelSpaceAlsoAdvances(t *testing.T) {
	// Space is the secondary advance key kept for muscle memory.
	m := New(makeThreeCells())
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	if cmd == nil {
		t.Fatal("Space in SelectMode should return a tea.Cmd")
	}
	if msg := cmd(); msg == nil {
		t.Fatal("space cmd returned nil msg")
	} else if _, ok := msg.(AdvanceMsg); !ok {
		t.Errorf("space cmd returned %T, want AdvanceMsg", msg)
	}
}

func TestModelResizeInvalidatesCaches(t *testing.T) {
	longBody := "one two three four five six seven eight nine ten " +
		"eleven twelve thirteen fourteen fifteen sixteen seventeen " +
		"eighteen nineteen twenty twenty-one twenty-two twenty-three"
	cells := []Cell{NewMetaCell("m", "Title", longBody)}
	m := New(cells)
	h40 := cells[0].HeightHint(40)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = next.(Model)
	h120 := m.Cells()[0].HeightHint(120)
	if h40 == h120 {
		t.Errorf("expected width-dependent HeightHint; both = %d", h40)
	}
	// Going back: cache must rebuild, not stick at h120.
	if again := m.Cells()[0].HeightHint(40); again != h40 {
		t.Errorf("HeightHint(40) after resize = %d, want %d", again, h40)
	}
}

func TestModelCtrlCQuitsFromAnyMode(t *testing.T) {
	// SelectMode: Ctrl+C → Quit.
	m := New(makeThreeCells())
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("Ctrl+C in SelectMode should return a quit cmd")
	}
	if msg := cmd(); msg != nil {
		if _, ok := msg.(tea.QuitMsg); !ok {
			t.Errorf("Ctrl+C cmd returned %T, want tea.QuitMsg", msg)
		}
	}

	// ViewMode: Ctrl+C should still quit, not pass to the cell.
	m2 := New(makeThreeCells())
	m2 = sendKey(t, m2, "down")
	m2 = sendKey(t, m2, "s") // focus
	if m2.Mode() != ViewMode {
		t.Fatal("setup: expected ViewMode")
	}
	_, cmd2 := m2.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd2 == nil {
		t.Fatal("Ctrl+C in ViewMode should also return a quit cmd (universal interrupt)")
	}
	if msg := cmd2(); msg != nil {
		if _, ok := msg.(tea.QuitMsg); !ok {
			t.Errorf("Ctrl+C in ViewMode cmd returned %T, want tea.QuitMsg", msg)
		}
	}
}

func TestModelCtrlDQuits(t *testing.T) {
	m := New(makeThreeCells())
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	if cmd == nil {
		t.Fatal("Ctrl+D should return a quit cmd")
	}
	if msg := cmd(); msg != nil {
		if _, ok := msg.(tea.QuitMsg); !ok {
			t.Errorf("Ctrl+D cmd returned %T, want tea.QuitMsg", msg)
		}
	}
}

func TestModelQuitOnQ(t *testing.T) {
	m := New(makeThreeCells())
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatal("q should return a quit cmd")
	}
	if msg := cmd(); msg != tea.Quit() {
		// tea.Quit is a function returning tea.QuitMsg; compare via type
		if _, ok := msg.(tea.QuitMsg); !ok {
			t.Errorf("q cmd returned %T, want tea.QuitMsg", msg)
		}
	}
}
