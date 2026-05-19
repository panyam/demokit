// Package notebook is a standalone cell-based TUI component built
// on Bubble Tea. It has no dependency on demokit, events, or any
// other consumer — callers drive it via Append/Update/Stream and
// AwaitAdvance/AwaitInput. Bridges (e.g. demokit/notebookbridge)
// translate domain events into these calls.
//
// Rendering is range-based: the viewport asks each cell for just
// the row window it intends to display, never the full cell body.
// That contract is what unlocks lazy materialization without
// touching the viewport code.
package notebook

import (
	tea "github.com/charmbracelet/bubbletea"
)

// Mode is the notebook's mode state. Hierarchical and vim-ish:
// SelectMode (cursor navigates cells) wraps FocusedMode (focused
// cell owns keystrokes), which in turn has ViewMode (today's
// interactive defaults) and EditMode (reserved). Esc pops up one
// level.
//
// Encoded as a small interface rather than an enum so future modes
// (Edit, Visual, Search) don't churn switch statements.
type Mode interface {
	Name() string
}

type selectMode struct{}
type viewMode struct{}
type editMode struct{}

func (selectMode) Name() string { return "SELECT" }
func (viewMode) Name() string   { return "VIEW" }
func (editMode) Name() string   { return "EDIT" }

// SelectMode is the default outermost mode — the cell cursor moves
// between cells; the focused cell, if any, does not receive keys.
var SelectMode Mode = selectMode{}

// ViewMode is the inner interactive mode that owns keys when a cell
// is focused.
var ViewMode Mode = viewMode{}

// EditMode is reserved for in-TUI authoring. Not wired into key
// handling yet; declared so the Cell interface can already receive
// it without a future signature change.
var EditMode Mode = editMode{}

// Cell is the unit the notebook viewport navigates and renders.
// Implementations own their content.
//
// Rendering is range-based — viewport calls RenderRows(width, lo, hi)
// for just the visible row window, never RenderAll.
type Cell interface {
	// ID returns a stable identifier unique within the notebook.
	// Insert rejects duplicate IDs; Remove/Get/IndexOf lookup by ID.
	ID() string

	// HeightHint reports the row count the cell would occupy at the
	// given width. Must be cheap & deterministic — the viewport
	// invokes it once per visible-window calculation. Implementations
	// should cache (width → height) and invalidate on width change.
	HeightHint(width int) int

	// RenderRows returns the row slice for the half-open range
	// [startRow, endRow) at the given width. Cells with large bodies
	// compute only the requested window.
	RenderRows(width, startRow, endRow int, focused bool, mode Mode) []string

	// Update receives Bubble Tea messages only when the cell is the
	// focused cell. Returns the updated cell and an optional command.
	Update(msg tea.Msg, mode Mode) (Cell, tea.Cmd)

	// StatusHint returns the right-side status text shown when this
	// cell is focused.
	StatusHint(mode Mode) string
}
