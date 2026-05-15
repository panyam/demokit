// Package notebook implements demokit's vim-modal Bubble Tea TUI:
// a cell-based interactive renderer where the visited trace becomes
// a navigable stream of typed cells (MetaCell / SectionCell /
// VerbatimCell / OutputCell). See GitHub issue 13 for the canonical
// design doc.
//
// Rendering is range-based — viewport asks each cell for just the
// row window it intends to display, never for the full cell body.
// That contract is what unlocks future lazy-materialization without
// touching the viewport code.
package notebook

import (
	tea "github.com/charmbracelet/bubbletea"
)

// Mode is the demokit notebook's mode state. Hierarchical and
// vim-ish: SelectMode (cursor navigates cells) wraps FocusedMode
// (focused cell owns keystrokes), which in turn has ViewMode
// (today's interactive defaults) and EditMode (reserved, no Phase A
// implementation). Esc pops up one level.
//
// Encoded as a small interface rather than an enum so future modes
// (Edit, Visual, Search) don't churn switch statements.
type Mode interface {
	// Name returns a short human-readable label used by the status
	// line ("SELECT", "VIEW", later "EDIT").
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
// is focused. Phase A's only focused-mode value; EditMode is reserved.
var ViewMode Mode = viewMode{}

// EditMode is reserved for Phase E (in-TUI authoring). Not wired
// into key handling yet; declared so the Cell interface can already
// receive it without a future signature change.
var EditMode Mode = editMode{}

// Cell is the unit the notebook viewport navigates and renders.
// Implementations own their content (they're not views over the
// Demo); future EditMode mutates cell state directly.
//
// Rendering is range-based — viewport calls RenderRows(width, lo, hi)
// for just the visible row window, never RenderAll. This keeps the
// viewport rendering loop unchanged when storage moves from eager
// []Cell to lazy CellSource later.
type Cell interface {
	// ID returns a stable identifier unique across the trace. Per
	// the design doc, IDs are per-visit (e.g. "step.name#visit2.kind")
	// so a step visited twice produces distinguishable cells.
	ID() string

	// HeightHint reports the row count the cell would occupy at the
	// given width. Must be cheap & deterministic — the viewport
	// invokes it once per visible-window calculation, including for
	// cells just outside the viewport, to compute scroll offsets
	// WITHOUT materializing each cell's full body. Implementations
	// should cache `(width → height)` and invalidate on width change.
	HeightHint(width int) int

	// RenderRows returns the row slice for the half-open range
	// [startRow, endRow) at the given width. Cells with huge bodies
	// (an OutputCell with 10k lines) compute only the requested
	// window; off-screen rows never get formatted into ANSI.
	//
	// focused indicates whether this cell currently has focus
	// (different highlight); mode tells the cell which inner mode is
	// active so it can adjust e.g. cursor display.
	RenderRows(width, startRow, endRow int, focused bool, mode Mode) []string

	// Update receives Bubble Tea messages only when the cell is the
	// focused cell AND mode is one of the interactive modes (View /
	// Edit). Background cells never tick. Returns the updated cell
	// (cells are value types here so we can return a new copy or
	// mutate; either is fine) and an optional command.
	Update(msg tea.Msg, mode Mode) (Cell, tea.Cmd)

	// StatusHint returns the right-side status text shown when this
	// cell is focused — the per-cell keymap shown to the user
	// ("Tab cycle · c copy", "j/k scroll", ...). Renderer may omit
	// or truncate to fit terminal width.
	StatusHint(mode Mode) string
}
