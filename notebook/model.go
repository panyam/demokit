package notebook

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// repaintInterval drives the re-render tick. Store mutations made
// from caller goroutines don't Send (a Send before Run would
// block); the tick picks up store changes within one frame.
const repaintInterval = 16 * time.Millisecond

type repaintTickMsg struct{}

func repaintTick() tea.Cmd {
	return tea.Tick(repaintInterval, func(time.Time) tea.Msg { return repaintTickMsg{} })
}

// snapshotMsg is a synchronous query: the model recomputes the
// viewport, renders, and sends the result on reply. Snapshot()
// uses it so tests see a freshly-laid-out frame regardless of
// tick timing.
type snapshotMsg struct {
	reply chan string
}

// model is the notebook's tea.Model. The cell list / cursor /
// header live in the shared store; the model owns only the
// BT-goroutine-local viewport, size, and mode.
type model struct {
	nb             *Notebook
	viewportOffset int
	width          int
	height         int
	mode           Mode
}

func newModel(nb *Notebook, width, height int) model {
	return model{
		nb:     nb,
		mode:   NavigationMode,
		width:  width,
		height: height,
	}
}

// Init implements tea.Model. The first batched cmd closes the
// ready channel once the program loop is live — Snapshot waits on
// it before issuing a Send.
func (m model) Init() tea.Cmd {
	ready := m.nb.ready
	return tea.Batch(
		func() tea.Msg { close(ready); return nil },
		repaintTick(),
	)
}

// Update implements tea.Model.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ensureCursorVisible()
		m.applyPendingAlignment()
		return m, nil
	case repaintTickMsg:
		m.ensureCursorVisible()
		m.applyPendingAlignment()
		return m, repaintTick()
	case snapshotMsg:
		m.ensureCursorVisible()
		m.applyPendingAlignment()
		msg.reply <- m.View()
		return m, nil
	case setModeMsg:
		m.mode = msg.mode
		return m, nil
	case ReleaseFocusMsg:
		// A docked cell emitting ReleaseFocus also drops dock focus
		// so subsequent keys land on the main cursor cell.
		m.nb.dockFocusKey.Store(nil)
		m.mode = NavigationMode
		return m, nil
	case CellAdvanceMsg:
		m.mode = NavigationMode
		m.nb.store.moveCursor(+1)
		m.ensureCursorVisible()
		return m, nil
	case PromptSubmittedMsg:
		// Resolve the pending AwaitInputBy (if any) and exit
		// CellActiveMode. Cursor is NOT moved — the caller (typically
		// the awaiter goroutine) decides what cursor position
		// follows a successful submit.
		if m.nb.rdv != nil {
			src := msg.Source
			if src == "" {
				src = "user-submitted"
			}
			m.nb.rdv.resolveInput(msg.CellID, msg.Answers, src)
		}
		m.mode = NavigationMode
		return m, nil
	case ClearCopyMsg:
		// Try main cells first; if no main cell claims the ID, try
		// docked cells too so dock-resident copy toasts also clear.
		if _, ok := m.nb.store.get(msg.CellID); ok {
			cmd, _ := m.routeToCell(msg.CellID, msg)
			return m, cmd
		}
		cmd, _ := m.routeMsgToDockByID(msg.CellID, msg)
		return m, cmd
	case tea.KeyMsg:
		return m.handleKey(msg)
	case tea.MouseMsg:
		return m.handleMouse(msg)
	}
	return m, nil
}

// handleMouse routes mouse events through the configured MouseConfig.
//
// Wheel events are cell-first: the cursor cell sees them via
// cell.Update (so OutputCell can scroll line-by-line). If the cell
// doesn't claim, the notebook calls MouseConfig.OnWheelFallback —
// typically WheelNavCursor, which moves the cell cursor.
//
// Non-wheel presses (and touchscreen taps) are geometric: the
// notebook resolves the click's cell from Y, builds a MouseContext,
// and calls MouseConfig.OnClick. Cells don't see clicks today —
// click semantics are app-policy, not cell-policy. Handlers can
// branch on ctx.Button to differentiate left/right/middle.
func (m model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action != tea.MouseActionPress {
		return m, nil
	}
	switch msg.Button {
	case tea.MouseButtonWheelUp, tea.MouseButtonWheelDown:
		cellCmd, handled := m.routeMouseToCursor(msg)
		if handled {
			return m, cellCmd
		}
		if h := m.nb.mouseConfig.OnWheelFallback; h != nil {
			cmd := h(m.nb, m.mouseContext(msg))
			m.ensureCursorVisible()
			return m, cmd
		}
		return m, cellCmd
	default:
		if h := m.nb.mouseConfig.OnClick; h != nil {
			cmd := h(m.nb, m.mouseContext(msg))
			m.ensureCursorVisible()
			return m, cmd
		}
	}
	return m, nil
}

// mouseContext builds a MouseContext for the current mouse event,
// resolving the cell at the click position. CellID is "" and
// CellIndex is -1 when the event landed outside any cell.
func (m model) mouseContext(msg tea.MouseMsg) MouseContext {
	ctx := MouseContext{
		X:         msg.X,
		Y:         msg.Y,
		Button:    msg.Button,
		CellID:    "",
		CellIndex: -1,
		Mode:      m.mode,
		Alt:       msg.Alt,
		Ctrl:      msg.Ctrl,
		Shift:     msg.Shift,
	}
	if idx, ok := m.cellAtRow(msg.Y); ok {
		snap := m.nb.store.snapshot()
		if idx >= 0 && idx < len(snap.cells) {
			ctx.CellIndex = idx
			ctx.CellID = CellID(snap.cells[idx].ID())
		}
	}
	return ctx
}

// routeMouseToCursor delivers a mouse msg to the cursor cell via
// cell.Update (same plumbing as keys). Used for wheel events.
func (m model) routeMouseToCursor(msg tea.MouseMsg) (tea.Cmd, bool) {
	snap := m.nb.store.snapshot()
	if snap.cursor < 0 || snap.cursor >= len(snap.cells) {
		return nil, false
	}
	return m.routeToCell(snap.cells[snap.cursor].ID(), msg)
}

// cellAtRow translates an absolute terminal Y row into a cell
// index, accounting for the header row, the Top dock, and the
// current viewport offset. Returns (-1, false) when the click
// landed outside the body (header / Top dock / Bottom dock / past
// last cell). Clicks landing on Before/After anchored docks
// resolve to their anchor cell — they're not cursor-targetable.
func (m model) cellAtRow(y int) (int, bool) {
	snap := m.nb.store.snapshot()
	bodyStart := 0
	if snap.header != "" {
		bodyStart++
	}
	if top, ok := snap.docks[edgePosition{edgeTop}.positionKey()]; ok {
		bodyStart += top.HeightHint(m.width)
	}
	if y < bodyStart {
		return -1, false
	}
	bodyEnd := bodyStart + m.bodyHeight(snap)
	if y >= bodyEnd {
		return -1, false
	}
	logicalRow := (y - bodyStart) + m.viewportOffset
	row := 0
	for i, c := range snap.cells {
		before, after := m.anchoredRowSpan(snap, c)
		row += before
		h := c.HeightHint(m.width)
		if logicalRow < row+h+after {
			return i, true
		}
		row += h + after
	}
	return -1, false
}

// handleKey routes a keystroke focus-first. The focused target
// (a docked cell if one is focused, otherwise the cursor cell)
// sees every key via Update before the notebook tries its KeyMap.
// If it returns handled=true the notebook stops; otherwise the
// notebook looks the key up in Global then current-mode bindings.
//
// The rare cell-cmd-with-handled=false case is honored: if the
// target returned a side-effect cmd AND a KeyMap action also fires,
// the action's cmd takes precedence (target's cmd is dropped). If
// only one fires, that one's cmd is used.
func (m model) handleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cellCmd tea.Cmd
	var handled bool
	if dockKey, ok := m.nb.focusedDockKey(); ok {
		cellCmd, handled = m.routeKeyToDock(dockKey, key)
	} else {
		cellCmd, handled = m.routeKeyToCursor(key)
	}
	if handled {
		return m, cellCmd
	}
	if action := m.nb.keymap.lookup(m.mode, key.String()); action != nil {
		return m, action(m.nb)
	}
	return m, cellCmd
}

// routeKeyToCursor delivers a key to the focused cell. Returns
// the cell's cmd and handled flag; (nil, false) when there's no
// cell to route to.
func (m model) routeKeyToCursor(key tea.KeyMsg) (tea.Cmd, bool) {
	snap := m.nb.store.snapshot()
	if snap.cursor < 0 || snap.cursor >= len(snap.cells) {
		return nil, false
	}
	return m.routeToCell(snap.cells[snap.cursor].ID(), key)
}

// routeToCell delivers a msg to the cell with the given ID,
// writes the (possibly new) cell back into the store, and returns
// the cell's cmd and handled flag.
func (m model) routeToCell(id CellID, msg tea.Msg) (tea.Cmd, bool) {
	var cmd tea.Cmd
	var handled bool
	m.nb.store.update(id, func(c Cell) Cell {
		updated, c2, h := c.Update(msg, m.mode)
		cmd = c2
		handled = h
		return updated
	})
	return cmd, handled
}

// routeKeyToDock delivers a msg to the docked cell at k, writes
// the (possibly new) cell back into the registry, and returns the
// cell's cmd and handled flag. Mirrors routeToCell but for docks.
func (m model) routeKeyToDock(k positionKey, msg tea.Msg) (tea.Cmd, bool) {
	var cmd tea.Cmd
	var handled bool
	m.nb.store.updateDock(k, func(c Cell) Cell {
		updated, c2, h := c.Update(msg, m.mode)
		cmd = c2
		handled = h
		return updated
	})
	return cmd, handled
}

// routeMsgToDockByID finds the dock whose Cell.ID matches the given
// ID and delivers msg to it. Used for ID-keyed routings like
// ClearCopyMsg where the sender knows its own ID but not its
// dock position.
func (m model) routeMsgToDockByID(id CellID, msg tea.Msg) (tea.Cmd, bool) {
	snap := m.nb.store.snapshot()
	for k, c := range snap.docks {
		if c.ID() == id {
			return m.routeKeyToDock(k, msg)
		}
	}
	return nil, false
}

// View implements tea.Model. Range-based render: each cell is
// asked only for the row window the viewport will display.
//
// Layout (top to bottom): optional header banner, Top dock if any,
// the body window (main cells interleaved with their After/Before
// anchored docks), Bottom dock if any. Top and Bottom claim layout
// space; the body's height is what's left.
func (m model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	snap := m.nb.store.snapshot()
	focusedDockKey, dockFocused := m.nb.focusedDockKey()
	_, topH, bottomH, bodyRows := m.edgeAllotments(snap)

	var lines []string
	if snap.header != "" {
		banner := "≡ " + snap.header
		if snap.done {
			banner += "  · Done."
		}
		lines = append(lines, banner)
	}

	// Top dock — claims topH rows. If the dock wanted more
	// (HeightHint > topH), we show the HEAD; tabs/breadcrumbs read
	// left-to-right, top-first, so head-truncation is the right
	// default. Apps that want different semantics swap the dock.
	if topH > 0 {
		topKey := edgePosition{edgeTop}.positionKey()
		focused := dockFocused && focusedDockKey == topKey
		lines = append(lines, snap.docks[topKey].RenderRows(m.width, 0, topH, focused, m.mode)...)
	}

	bodyStart := len(lines)
	windowStart := m.viewportOffset
	windowEnd := m.viewportOffset + bodyRows

	// renderSegment appends the visible slice of segment c (with
	// the given focused flag) given a rolling rowCursor, returning
	// the new rowCursor.
	rowCursor := 0
	renderSegment := func(c Cell, focused bool) {
		h := c.HeightHint(m.width)
		cellEnd := rowCursor + h
		if cellEnd <= windowStart {
			rowCursor = cellEnd
			return
		}
		if rowCursor >= windowEnd {
			rowCursor = cellEnd
			return
		}
		lo, hi := 0, h
		if rowCursor < windowStart {
			lo = windowStart - rowCursor
		}
		if cellEnd > windowEnd {
			hi = h - (cellEnd - windowEnd)
		}
		lines = append(lines, c.RenderRows(m.width, lo, hi, focused, m.mode)...)
		rowCursor = cellEnd
	}

	// Body: for each main cell, interleave Before / cell / After
	// anchored docks. Cursor focus only fires when a docked cell
	// isn't taking focus.
	for i, c := range snap.cells {
		if rowCursor >= windowEnd {
			break
		}
		if before, ok := snap.docks[(cellAnchor{rel: relBefore, cellID: c.ID()}).positionKey()]; ok {
			focused := dockFocused && focusedDockKey == (cellAnchor{rel: relBefore, cellID: c.ID()}).positionKey()
			renderSegment(before, focused)
		}
		renderSegment(c, !dockFocused && i == snap.cursor)
		if after, ok := snap.docks[(cellAnchor{rel: relAfter, cellID: c.ID()}).positionKey()]; ok {
			focused := dockFocused && focusedDockKey == (cellAnchor{rel: relAfter, cellID: c.ID()}).positionKey()
			renderSegment(after, focused)
		}
	}

	// Pad/truncate body to exactly bodyRows.
	target := bodyStart + bodyRows
	if len(lines) > target {
		lines = lines[:target]
	}
	for len(lines) < target {
		lines = append(lines, "")
	}

	// Bottom dock — claims bottomH rows. If desired > bottomH the
	// dock yielded — render its TAIL so the cursor and most-recent
	// content stay visible. Command bars and tail-style status
	// readouts get the right behavior for free; cells that want
	// head-truncation pass an explicit anchored top-style cell.
	if bottomH > 0 {
		botKey := edgePosition{edgeBottom}.positionKey()
		desired := m.dockDesiredHeight(snap, botKey)
		startRow := 0
		endRow := desired
		if desired > bottomH {
			startRow = desired - bottomH
		} else {
			endRow = bottomH
		}
		focused := dockFocused && focusedDockKey == botKey
		lines = append(lines, snap.docks[botKey].RenderRows(m.width, startRow, endRow, focused, m.mode)...)
	}

	return strings.Join(lines, "\n")
}

// edgeAllotments returns the row counts actually allotted to the
// header, Top dock, and Bottom dock, plus the body height. Edge
// docks report a "desired" via HeightHint but auto-grow content
// (vim-style command bars, multi-line status) can ask for more
// than the terminal has room for — so the layout enforces:
//
//  1. Body always gets at least 1 row (chrome can't starve the body).
//  2. When desired Top + Bottom + header would oversubscribe, Bottom
//     yields first (command bars scroll their own tail), then Top.
//
// Returned allotments are what View should actually render; cells
// that wanted more handle the truncation in RenderRows by drawing
// only their visible window.
func (m model) edgeAllotments(snap snapshot) (headerH, topH, bottomH, bodyH int) {
	if snap.header != "" {
		headerH = 1
	}
	desiredTop := 0
	if top, ok := snap.docks[edgePosition{edgeTop}.positionKey()]; ok {
		desiredTop = top.HeightHint(m.width)
	}
	desiredBot := 0
	if bot, ok := snap.docks[edgePosition{edgeBottom}.positionKey()]; ok {
		desiredBot = bot.HeightHint(m.width)
	}
	available := m.height - headerH
	if available < 1 {
		return headerH, 0, 0, 1
	}
	const minBody = 1
	topH = desiredTop
	bottomH = desiredBot
	for topH+bottomH > available-minBody {
		if bottomH > 0 {
			bottomH--
			continue
		}
		if topH > 0 {
			topH--
			continue
		}
		break
	}
	bodyH = available - topH - bottomH
	if bodyH < minBody {
		bodyH = minBody
	}
	return
}

// bodyHeight is the row count available for the body. Wraps
// edgeAllotments for callers that only need the body figure.
func (m model) bodyHeight(snap snapshot) int {
	_, _, _, body := m.edgeAllotments(snap)
	return body
}

// dockDesiredHeight returns the dock's reported HeightHint (not
// the clamped allotment) — used by View to decide whether to
// render the head or the tail of an oversubscribed dock.
func (m model) dockDesiredHeight(snap snapshot, k positionKey) int {
	if c, ok := snap.docks[k]; ok {
		return c.HeightHint(m.width)
	}
	return 0
}

// anchoredRowSpan returns the row span of any anchored docks for
// cells[i] in {Before, After} order — used by cellRowSpan and
// cellAtRow to walk the body in render order.
func (m model) anchoredRowSpan(snap snapshot, c Cell) (beforeH, afterH int) {
	if d, ok := snap.docks[(cellAnchor{rel: relBefore, cellID: c.ID()}).positionKey()]; ok {
		beforeH = d.HeightHint(m.width)
	}
	if d, ok := snap.docks[(cellAnchor{rel: relAfter, cellID: c.ID()}).positionKey()]; ok {
		afterH = d.HeightHint(m.width)
	}
	return
}

// cellRowSpan returns the [start, end) absolute body-row range of
// cells[idx] including the Before-anchored dock space that
// precedes it. End is just past the cell itself (After-anchored
// rows are NOT in the span — they trail and aren't part of the
// cell's own scroll target).
func (m model) cellRowSpan(snap snapshot, idx int) (int, int) {
	start := 0
	for i := 0; i < idx && i < len(snap.cells); i++ {
		before, after := m.anchoredRowSpan(snap, snap.cells[i])
		start += before + snap.cells[i].HeightHint(m.width) + after
	}
	if idx < 0 || idx >= len(snap.cells) {
		return start, start
	}
	before, _ := m.anchoredRowSpan(snap, snap.cells[idx])
	start += before
	return start, start + snap.cells[idx].HeightHint(m.width)
}

// ensureCursorVisible nudges viewportOffset so the focused cell is
// within the visible window.
func (m *model) ensureCursorVisible() {
	if m.width == 0 || m.height == 0 {
		return
	}
	snap := m.nb.store.snapshot()
	if len(snap.cells) == 0 {
		m.viewportOffset = 0
		return
	}
	body := m.bodyHeight(snap)
	start, end := m.cellRowSpan(snap, snap.cursor)
	if start < m.viewportOffset {
		m.viewportOffset = start
		return
	}
	if end > m.viewportOffset+body {
		m.viewportOffset = end - body
		if m.viewportOffset > start {
			m.viewportOffset = start
		}
	}
}

// applyPendingAlignment consumes any queued RevealTop / RevealMiddle
// from the store and adjusts viewportOffset so the requested cell
// sits at the top or center of the body. Runs after
// ensureCursorVisible in the per-frame Update cases — explicit
// alignment overrides the cursor-tracking nudge for the one frame
// it fires on.
//
// Skips silently when width/height haven't been set yet (e.g.
// Insert ran before the first WindowSizeMsg); the request stays
// queued only if we don't consume — but here we DO consume, so
// a queued align before init is dropped. In practice the model
// gets WindowSizeMsg as its first message after Run starts, so
// this only matters in pathological setups.
func (m *model) applyPendingAlignment() {
	idx, r, ok := m.nb.store.consumePendingAlign()
	if !ok {
		return
	}
	if m.width == 0 || m.height == 0 {
		return
	}
	snap := m.nb.store.snapshot()
	if idx < 0 || idx >= len(snap.cells) {
		return
	}
	body := m.bodyHeight(snap)
	start, end := m.cellRowSpan(snap, idx)
	cellH := end - start
	switch r {
	case RevealTop:
		m.viewportOffset = start
	case RevealMiddle:
		// Center the cell's span in the body. Cells taller than
		// the body have no meaningful middle — clamp to top.
		if cellH >= body {
			m.viewportOffset = start
			return
		}
		off := start - (body-cellH)/2
		if off < 0 {
			off = 0
		}
		m.viewportOffset = off
	}
}
