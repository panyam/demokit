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
//
// Unlike CellAdvanceMsg, the notebook does NOT auto-move the
// cursor on receipt — the caller (typically the AwaitInput
// awaiter goroutine) decides what cursor / focus state should
// follow a successful submit.
type PromptSubmittedMsg struct {
	CellID  string
	Answers map[string]any
}

// ReleaseFocusMsg signals that a focused cell wants to give focus
// back to the notebook (drop to SelectMode) without advancing the
// cursor. Built-in cells emit it on Esc by convention; custom
// cells decide their own release semantics.
type ReleaseFocusMsg struct{}

// ReleaseFocus is the tea.Cmd that emits ReleaseFocusMsg. Return
// it from Cell.Update when the cell wants to exit ViewMode but
// keep the cursor where it is.
func ReleaseFocus() tea.Msg { return ReleaseFocusMsg{} }

// setModeMsg is the internal msg used by NotebookActions and
// AwaitInput to change the current Mode from outside the model.
// Apps don't construct it directly — they call nb.SetMode(m) or
// use a built-in Action like notebook.SetMode(m).
type setModeMsg struct {
	mode Mode
}
