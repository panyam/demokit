package notebook

import tea "github.com/charmbracelet/bubbletea"

// Action is the function the notebook runs when a key matches a
// KeyMap binding. Receives *Notebook so the action can call
// public mutation methods (MoveCursor, SetMode, Append, etc.).
//
// Actions can either:
//   - Mutate the notebook synchronously via store methods that
//     don't require model state (MoveCursor, SetCursor, store CRUD).
//   - Return a tea.Cmd that emits a msg the model handles (e.g.
//     setModeMsg for mode changes). Required for anything that
//     mutates fields owned by the model goroutine.
//
// The two are equivalent in effect from the user's perspective —
// both settle within one repaint tick.
type Action func(nb *Notebook) tea.Cmd

// KeyMap routes unhandled keys (keys the cursor cell returned
// handled=false for) to notebook-level actions.
//
// Lookup order in handleKey:
//
//  1. Cursor cell sees the key first via cell.Update; if it
//     returns handled=true the notebook stops.
//  2. KeyMap.Global is checked next — these bindings apply
//     regardless of current mode (Quit, app-level shortcuts).
//  3. KeyMap.Modes[currentMode] is checked last — per-mode
//     bindings (navigation, mode transitions).
//
// A mode with no entry in Modes means "no notebook-level
// bindings for this mode" — every key falls through to the cell.
// That's the natural setup for CellActiveMode: cells own everything
// while focused.
type KeyMap struct {
	Global map[string]Action
	Modes  map[Mode]map[string]Action
}

// DefaultKeyMap returns the canonical bindings:
//
//   - Ctrl+C → Quit (Global)
//   - NavigationMode: ↑/k NavUp, ↓/j NavDown, enter/s/f EnterFocus
//   - CellActiveMode: none (cells own everything)
//
// 'q' is intentionally NOT in the defaults — q is a printable
// character a cell might want. Apps that want q-to-quit add it
// explicitly via WithKeyMap.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Global: map[string]Action{
			"ctrl+c": Quit,
		},
		Modes: map[Mode]map[string]Action{
			NavigationMode: {
				"up":    NavUp,
				"k":     NavUp,
				"down":  NavDown,
				"j":     NavDown,
				"enter": EnterFocus,
				"s":     EnterFocus,
				"f":     EnterFocus,
			},
		},
	}
}

// lookup finds the Action for a key in the given mode, checking
// Global first then the mode-specific bindings. Returns nil if no
// match — caller treats that as "key was dropped."
func (km KeyMap) lookup(mode Mode, key string) Action {
	if action, ok := km.Global[key]; ok {
		return action
	}
	if bindings, ok := km.Modes[mode]; ok {
		if action, ok := bindings[key]; ok {
			return action
		}
	}
	return nil
}

// --- Built-in actions ---

// Quit signals BT to quit.
func Quit(*Notebook) tea.Cmd { return tea.Quit }

// NavUp moves the cursor up by one cell.
func NavUp(nb *Notebook) tea.Cmd {
	nb.store.moveCursor(-1)
	return nil
}

// NavDown moves the cursor down by one cell.
func NavDown(nb *Notebook) tea.Cmd {
	nb.store.moveCursor(+1)
	return nil
}

// EnterFocus switches to CellActiveMode (cell-owns-keys). No-op if
// there are no cells to focus.
func EnterFocus(nb *Notebook) tea.Cmd {
	if nb.store.count() == 0 {
		return nil
	}
	return func() tea.Msg { return setModeMsg{mode: CellActiveMode} }
}

// ExitFocus switches to NavigationMode. Equivalent to a cell
// returning ReleaseFocus, but accessible as an Action.
func ExitFocus(*Notebook) tea.Cmd {
	return func() tea.Msg { return setModeMsg{mode: NavigationMode} }
}

// SetMode returns an Action that switches to the given mode. Use
// for app-defined modes:
//
//	commandMode := notebook.NewMode("COMMAND")
//	km.Modes[notebook.NavigationMode][":"] = notebook.SetMode(commandMode)
func SetMode(m Mode) Action {
	return func(*Notebook) tea.Cmd {
		return func() tea.Msg { return setModeMsg{mode: m} }
	}
}
