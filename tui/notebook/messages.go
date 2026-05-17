package notebook

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// clearCopyMsg is the deferred command-result that tells a cell to
// drop its transient "(copied …)" status line. Carries cellID so
// the model can route it back to the correct cell.
type clearCopyMsg struct {
	cellID string
}

// clearCopyMsgAfter returns a tea.Cmd that fires a clearCopyMsg
// for cellID after copyToastDuration. Cells call this from Update
// after a successful copy.
func clearCopyMsgAfter(cellID string) tea.Cmd {
	return tea.Tick(copyToastDuration, func(_ time.Time) tea.Msg {
		return clearCopyMsg{cellID: cellID}
	})
}

// copyToastDuration is how long the per-cell "(copied …)" line
// stays on screen before fading.
const copyToastDuration = 1200 * time.Millisecond

// cellAdvanceMsg is the model-internal signal that a focused cell
// has finished its work and the user should be returned to
// SelectMode AND the demo advanced.
//
// Cells that don't use Enter for their own purposes (Verbatim,
// Output, Section) return cellAdvance as a tea.Cmd from Update on
// Enter so the default UX matches SelectMode Enter. PromptCell
// consumes Enter for form submission; on a successful submit it
// returns cellAdvance too. A future multiline-input cell that
// wants Enter to insert a literal newline simply doesn't return
// this cmd.
type cellAdvanceMsg struct{}

// cellAdvance is the tea.Cmd that emits cellAdvanceMsg.
func cellAdvance() tea.Msg { return cellAdvanceMsg{} }
