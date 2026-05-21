package notebook

import (
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
)

// StatusCell is the built-in single-row Cell installed at the
// Bottom dock by New(). It reproduces the legacy status line:
//
//	NAV  cell 3/7
//
// Apps that want richer chrome (vim-style command bar, multi-
// segment status, themed colors) replace this via
// SetDockedCell(Bottom, customCell). The default lives in package
// notebook (not in notebook/cells) so the package can self-install
// at New() without an import cycle.
//
// StatusCell reads its content from the notebook's store at render
// time, so callers never need to mutate it; mode and cursor
// position pick up automatically.
type StatusCell struct {
	nb *Notebook
}

// NewStatusCell returns the StatusCell bound to nb. Apps that
// removed the default Bottom dock can reinstall it with:
//
//	nb.SetDockedCell(notebook.Bottom, notebook.NewStatusCell(nb))
func NewStatusCell(nb *Notebook) *StatusCell { return &StatusCell{nb: nb} }

// ID implements Cell. Stable identifier; nothing in the framework
// looks it up but custom keymap actions could.
func (s *StatusCell) ID() string { return "notebook.status" }

// HeightHint implements Cell. One row — matches the legacy status
// line. Width is ignored (the status line never wraps).
func (s *StatusCell) HeightHint(int) int { return 1 }

// RenderRows implements Cell. Single row: "<MODE>  cell <pos>".
// focused / mode are ignored — the cell reads the current mode
// from the notebook directly so status reflects the GLOBAL mode
// even when some other cell or dock is focused.
func (s *StatusCell) RenderRows(_ int, startRow, endRow int, _ bool, mode Mode) []string {
	if startRow >= 1 || endRow <= 0 {
		return nil
	}
	return []string{s.line(mode)}
}

// Update implements Cell. StatusCell is read-only — it never
// claims a key. handled=false keeps the notebook's KeyMap in
// charge of bindings that fire while the status cell is focused.
func (s *StatusCell) Update(tea.Msg, Mode) (Cell, tea.Cmd, bool) {
	return s, nil, false
}

// StatusHint implements Cell. Status row has no right-side hint of
// its own.
func (s *StatusCell) StatusHint(Mode) string { return "" }

// line builds the rendered string. Matches the legacy
// model.statusLine output character-for-character so existing
// tests (and apps doing substring assertions) keep working.
func (s *StatusCell) line(mode Mode) string {
	name := "NAV"
	if mode != nil {
		name = mode.Name()
	}
	pos := "—"
	if s.nb != nil {
		count := s.nb.store.count()
		if count > 0 {
			pos = strconv.Itoa(s.nb.store.cursorPos()+1) + "/" + strconv.Itoa(count)
		}
	}
	return name + "  cell " + pos
}
