// Package notebook is a standalone cell-based TUI component built
// on Bubble Tea. It has no dependency on demokit, events, or any
// other consumer — callers drive it via Append/Update/Stream and
// AwaitInput. Bridges (e.g. demokit/notebookbridge) translate
// domain events into these calls.
//
// Rendering is range-based: the viewport asks each cell for just
// the row window it intends to display, never the full cell body.
//
// Key dispatch is cell-first: every keystroke goes to the cursor
// cell's Update before the notebook tries its own KeyMap. The
// cell returns a `handled bool`; if false, the notebook tries
// Global then current-mode bindings. See KeyMap for the
// notebook-level binding surface.
package notebook

import (
	tea "github.com/charmbracelet/bubbletea"
)

// Mode is an opaque value identifying the notebook's current
// interaction mode. The framework ships SelectMode and ViewMode as
// convenient defaults but apps can define their own via NewMode
// and register per-mode bindings in KeyMap.
//
// Cells receive the current Mode as a parameter to Update so they
// can react contextually; cells do not store mode themselves.
type Mode interface {
	Name() string
}

// NewMode returns an opaque Mode value with the given short name
// (shown in the status line / used for KeyMap keys). Two NewMode
// calls with the same name produce different Mode values — modes
// are compared by identity, not by name.
func NewMode(name string) Mode {
	return &mode{name: name}
}

type mode struct{ name string }

func (m *mode) Name() string { return m.name }

// SelectMode is the canonical "navigate between cells" mode used
// by the DefaultKeyMap. Apps that don't use the defaults can
// ignore it.
var SelectMode = NewMode("SELECT")

// ViewMode is the canonical "focused-cell-owns-keys" mode used by
// the DefaultKeyMap. Apps that don't use the defaults can ignore
// it.
var ViewMode = NewMode("VIEW")

// Cell is the unit the notebook viewport navigates and renders.
// Implementations own their content; render is range-based.
//
// The Update signature returns three values:
//   - the (possibly mutated) Cell
//   - an optional tea.Cmd for side effects
//   - handled — true if the cell consumed this msg; false to let
//     the notebook try its own KeyMap bindings on the same key
//
// The handled=false / cmd=nil combo is "I don't claim this" — the
// notebook will look up the key in Global + current mode and
// dispatch the matching Action. The rare handled=false / cmd!=nil
// case means "I did something with side effects but still want the
// notebook to also try its bindings" — useful for instrumentation
// or chained handlers.
type Cell interface {
	// ID returns a stable identifier unique within the notebook.
	ID() string

	// HeightHint reports the row count the cell would occupy at
	// the given width. Must be cheap & deterministic.
	HeightHint(width int) int

	// RenderRows returns the row slice for the half-open range
	// [startRow, endRow) at the given width.
	RenderRows(width, startRow, endRow int, focused bool, mode Mode) []string

	// Update receives Bubble Tea messages. For tea.KeyMsg, the
	// notebook always routes to the cursor cell first; the cell's
	// returned `handled` bool determines whether the notebook
	// continues with its own KeyMap. For other msg types the
	// notebook handles routing differently (e.g. ClearCopyMsg is
	// routed by cell ID).
	Update(msg tea.Msg, mode Mode) (Cell, tea.Cmd, bool)

	// StatusHint returns the right-side status text shown when
	// this cell is focused.
	StatusHint(mode Mode) string
}
