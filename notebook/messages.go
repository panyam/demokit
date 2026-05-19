package notebook

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// ClearCopyMsg is the deferred message that tells a cell to drop
// its transient "(copied …)" status line. Built-in cells listen
// for it; the notebook routes it back to the originating cell by
// CellID.
//
// Exported so cells in subpackages (notebook/cells) can both emit
// it via ClearCopyAfter and pattern-match against it in their
// Update implementations.
type ClearCopyMsg struct {
	CellID string
}

// CopyToastDuration is how long the per-cell "(copied …)" line
// stays on screen before fading. Exposed so consumers building
// custom cells can match the built-in cadence.
const CopyToastDuration = 1200 * time.Millisecond

// ClearCopyAfter returns a tea.Cmd that emits a ClearCopyMsg for
// cellID after CopyToastDuration. Cells call this from Update
// after a successful copy so the toast clears itself.
func ClearCopyAfter(cellID string) tea.Cmd {
	return tea.Tick(CopyToastDuration, func(_ time.Time) tea.Msg {
		return ClearCopyMsg{CellID: cellID}
	})
}

// CellAdvanceMsg signals that a focused cell has finished its
// interactive work — the notebook should return to SelectMode AND
// advance to the next cell.
//
// Cells that don't use Enter for their own purposes (Verbatim,
// Output, etc.) emit this on Enter so the default UX matches
// SelectMode Enter. PromptCell emits it on successful submit.
type CellAdvanceMsg struct{}

// CellAdvance is the tea.Cmd that emits CellAdvanceMsg. Use it as
// the second return value from Cell.Update when a cell wants to
// hand focus back to the notebook.
func CellAdvance() tea.Msg { return CellAdvanceMsg{} }

// PromptSubmittedMsg is emitted by a PromptCell when the user
// submits a valid answer set. The notebook routes it to the
// pending AwaitInput call by CellID. Answers maps each input's
// Name to its parsed value.
type PromptSubmittedMsg struct {
	CellID  string
	Answers map[string]any
}
