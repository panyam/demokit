package notebook

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// clearCopyMsg is the deferred command-result that tells a cell to
// drop its transient "(copied …)" status line. Carries cellID so the
// model can route it back to the correct cell when multiple cells
// could in principle have outstanding copy toasts (Phase B with
// cross-step nav).
type clearCopyMsg struct {
	cellID string
}

// clearCopyMsgAfter returns a tea.Cmd that fires a clearCopyMsg for
// cellID after copyToastDuration. Cells call this from Update after
// a successful copy; the model routes the resulting msg back to the
// same cell via its ID.
func clearCopyMsgAfter(cellID string) tea.Cmd {
	return tea.Tick(copyToastDuration, func(_ time.Time) tea.Msg {
		return clearCopyMsg{cellID: cellID}
	})
}

// copyToastDuration is how long the per-cell "(copied …)" line
// stays on screen before fading. ~1.2s matches the TUI's existing
// rhythm without lingering long enough to feel sticky during a
// fast-paced demo.
const copyToastDuration = 1200 * time.Millisecond

// OutputAppendedMsg is fired when an OutputBuffer attached to a
// running step has committed at least one new line. The model
// receives this only as a "trigger a redraw" signal — cells read
// their buffer fresh on RenderRows, so no state mutation is needed.
// Carries the cell ID for future routing in Phase B.
type OutputAppendedMsg struct {
	CellID string
}

// SubscribeOutputBuffer returns a tea.Cmd that blocks on the
// buffer's wakeup channel and emits an OutputAppendedMsg when a
// new line is committed. The model's Update on receipt should
// return SubscribeOutputBuffer again to keep listening — the
// idiomatic Bubble Tea reactive-event pattern.
//
// When the buffer is drained / closed the goroutine driving Append
// is expected to stop; the wakeup channel simply stays empty and
// no more messages flow.
func SubscribeOutputBuffer(buf *OutputBuffer, cellID string) tea.Cmd {
	return func() tea.Msg {
		<-buf.Subscribe()
		return OutputAppendedMsg{CellID: cellID}
	}
}
