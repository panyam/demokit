package notebook

import tea "github.com/charmbracelet/bubbletea"

// MouseHandler is the function the notebook runs on a mouse event
// it didn't already dispatch elsewhere. Returns an optional tea.Cmd
// the model relays after invocation.
//
// Receives *Notebook (for mutation calls — SetCursor, SetMode, …)
// and a MouseContext describing the event. The handler is invoked
// on the BT goroutine; mutating store methods are safe from here.
type MouseHandler func(nb *Notebook, ctx MouseContext) tea.Cmd

// MouseContext is the per-event context passed to a MouseHandler.
// Captures the position, the cell at that position (if any), which
// button was involved, modifier keys, and the current mode.
//
// CellID is "" and CellIndex is -1 when the event landed outside
// any cell (header row, status row, past the last cell).
type MouseContext struct {
	X, Y      int
	Button    tea.MouseButton
	CellID    CellID
	CellIndex int
	Mode      Mode

	Alt, Ctrl, Shift bool
}

// MouseConfig customizes how the notebook reacts to mouse events.
// Cells still see wheel events first via cell.Update (cell-first
// dispatch); MouseConfig handlers run when the cell passes through,
// OR for events with no clear cell target (clicks, right-clicks).
//
// Either handler may be nil to disable that branch entirely.
type MouseConfig struct {
	// OnClick fires on any non-wheel mouse press (or touchscreen
	// tap). ctx.Button identifies which button — left, right,
	// middle, back, forward, etc. Defaults to DefaultOnClick
	// (left-button → ClickActivate, other buttons no-op).
	OnClick MouseHandler

	// OnWheelFallback fires when a wheel event reaches the notebook
	// because the cursor cell passed it through (cells handle their
	// own internal scroll first). Defaults to WheelNavCursor (moves
	// the cell cursor one step in the wheel direction).
	OnWheelFallback MouseHandler
}

// DefaultMouseConfig returns the canonical mouse wiring:
//
//   - OnClick: DefaultOnClick — left-button → ClickActivate; other
//     buttons no-op.
//   - OnWheelFallback: WheelNavCursor — moves the cell cursor.
func DefaultMouseConfig() MouseConfig {
	return MouseConfig{
		OnClick:         DefaultOnClick,
		OnWheelFallback: WheelNavCursor,
	}
}

// --- Built-in click actions (button-agnostic) ---
//
// These describe an action, not a trigger — the OnClick handler
// decides which button(s) invoke them. Use directly when you
// always want the action regardless of button, or wrap in a
// button-filtering OnClick.

// ClickActivate sets the cursor to the clicked cell and switches
// to CellActiveMode so subsequent keys/wheel target that cell.
// No-op when the click landed outside any cell.
//
// Mode change is delivered via setModeMsg through the returned cmd
// (handlers run inside Update; calling nb.SetMode directly would
// re-enter the BT program loop). Cursor change is synchronous via
// the store.
func ClickActivate(nb *Notebook, ctx MouseContext) tea.Cmd {
	if ctx.CellIndex < 0 || ctx.CellID == "" {
		return nil
	}
	nb.SetCursor(ctx.CellID)
	return func() tea.Msg { return setModeMsg{mode: CellActiveMode} }
}

// ClickCursorOnly sets the cursor to the clicked cell without
// changing mode. Useful when an app wants "click = select, Enter =
// activate" (file-manager style).
func ClickCursorOnly(nb *Notebook, ctx MouseContext) tea.Cmd {
	if ctx.CellIndex < 0 || ctx.CellID == "" {
		return nil
	}
	nb.SetCursor(ctx.CellID)
	return nil
}

// --- Built-in wheel actions ---

// WheelNavCursor moves the cell cursor one step in the wheel's
// direction — same effect as pressing ↑/↓. Used as the default
// OnWheelFallback so a wheel event that no cell claimed scrolls
// between cells instead of getting dropped.
func WheelNavCursor(nb *Notebook, ctx MouseContext) tea.Cmd {
	switch ctx.Button {
	case tea.MouseButtonWheelUp:
		nb.store.moveCursor(-1)
	case tea.MouseButtonWheelDown:
		nb.store.moveCursor(+1)
	}
	return nil
}

// --- Default click handler (button-routing) ---

// DefaultOnClick is the default OnClick handler. Left-button presses
// invoke ClickActivate; other buttons no-op. Exported so apps that
// only want to extend behavior (e.g., add right-click) can fall
// through to it:
//
//	mc := notebook.DefaultMouseConfig()
//	mc.OnClick = func(nb *notebook.Notebook, ctx notebook.MouseContext) tea.Cmd {
//	    if ctx.Button == tea.MouseButtonRight { return myMenu(nb, ctx) }
//	    return notebook.DefaultOnClick(nb, ctx)
//	}
func DefaultOnClick(nb *Notebook, ctx MouseContext) tea.Cmd {
	if ctx.Button != tea.MouseButtonLeft {
		return nil
	}
	return ClickActivate(nb, ctx)
}
