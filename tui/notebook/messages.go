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

// --- Renderer-bridge messages (PR2; produced by NotebookRenderer,
//     consumed by Model.Update) ---

// BridgeHeaderMsg arrives once per demo, at the start when
// demokit.RenderHeader fires. The model stashes the title for the
// top banner row.
type BridgeHeaderMsg struct {
	Title       string
	Description string
	StepCount   int
}

// BridgeStepCellsMsg installs a fresh single-step cell list — the
// MetaCell + 0..N VerbatimCells + OutputCell built for the step
// just rendered. Replaces any previous cells (Phase A.1's
// single-step-on-screen contract). OutputBuf is the buffer the
// new OutputCell pulls from; OutputCellID is used for re-arming
// the SubscribeOutputBuffer listener.
type BridgeStepCellsMsg struct {
	Cells        []Cell
	OutputBuf    *OutputBuffer
	OutputCellID string
}

// BridgeSectionCellMsg appends a SectionCell to the current list.
// demokit.RenderSection fires for non-executable blocks that sit
// between executable steps; in Phase A.1's single-step-on-screen
// mode we treat them as standalone "step-shaped" cell lists too
// (just a single SectionCell, no MetaCell/OutputCell).
type BridgeSectionCellMsg struct {
	Cell Cell
}

// BridgeOutputDoneMsg flips the active OutputCell from "live" to
// "end" — fired when demokit.RenderResult is invoked.
type BridgeOutputDoneMsg struct {
	CellID string
}

// BridgeWaitMsg hands the model a channel to close when the user
// presses the advance key. WaitForStep on the renderer side blocks
// on the same channel — closing unblocks both the renderer and
// frees Execute to call into the next step.
//
// Ch is buffered/unbuffered at the caller's choice; the model
// only ever closes it. The model nils out its internal pointer on
// close so a second advance press becomes a no-op rather than
// closing twice.
type BridgeWaitMsg struct {
	Ch chan struct{}
}

// BridgeDoneMsg fires when demokit.RenderDone is invoked. The model
// shows a "Done." banner and stays alive until the user presses q
// so they can scroll back through prior cells before exiting.
type BridgeDoneMsg struct{}

// BridgePromptMsg appends a PromptCell to the current cell list
// and blocks the renderer's Prompt call on Reply. The model
// auto-focuses the new cell so the user can start typing
// immediately. When the user submits valid answers, the model
// sends them via Reply and the renderer's Prompt returns.
//
// Reply is closed (not sent to) on submission — the model sends
// the answer map via the buffered channel before close. Tests
// can construct this message directly.
type BridgePromptMsg struct {
	Inputs []promptInput
	Reply  chan map[string]any
}

// promptInput is the per-field projection of demokit.InputDef that
// PromptCell consumes. Kept package-local so the notebook doesn't
// re-export an alias of demokit's type; renderer.Prompt builds the
// slice from each InputDef before sending.
type promptInput struct {
	Name    string
	Prompt  string
	Default any
	Kind    string
	Options []string
	parse   func(string) (any, error)
}
