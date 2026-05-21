package notebook

import (
	"strconv"
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
		return m, nil
	case repaintTickMsg:
		m.ensureCursorVisible()
		return m, repaintTick()
	case snapshotMsg:
		m.ensureCursorVisible()
		msg.reply <- m.View()
		return m, nil
	case setModeMsg:
		m.mode = msg.mode
		return m, nil
	case ReleaseFocusMsg:
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
		cmd, _ := m.routeToCell(msg.CellID, msg)
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
// index, accounting for the header row + the current viewport
// offset. Returns (-1, false) when the click landed outside the
// body (header / status row / past last cell).
func (m model) cellAtRow(y int) (int, bool) {
	snap := m.nb.store.snapshot()
	bodyStart := 0
	if snap.header != "" {
		bodyStart = 1
	}
	if y < bodyStart {
		return -1, false
	}
	bodyEnd := bodyStart + m.bodyHeight()
	if y >= bodyEnd {
		return -1, false
	}
	logicalRow := (y - bodyStart) + m.viewportOffset
	row := 0
	for i, c := range snap.cells {
		h := c.HeightHint(m.width)
		if logicalRow < row+h {
			return i, true
		}
		row += h
	}
	return -1, false
}

// handleKey routes a keystroke cell-first. The cursor cell sees
// every key via cell.Update before the notebook tries its KeyMap.
// If the cell returns handled=true the notebook stops; otherwise
// the notebook looks the key up in Global then current-mode
// bindings.
//
// The rare cell-cmd-with-handled=false case is honored: if the
// cell returned a side-effect cmd AND a KeyMap action also fires,
// the action's cmd takes precedence (cell's cmd is dropped). If
// only one fires, that one's cmd is used.
func (m model) handleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	cellCmd, handled := m.routeKeyToCursor(key)
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

// View implements tea.Model. Range-based render: each cell is
// asked only for the row window the viewport will display.
func (m model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	snap := m.nb.store.snapshot()

	var lines []string
	if snap.header != "" {
		banner := "≡ " + snap.header
		if snap.done {
			banner += "  · Done."
		}
		lines = append(lines, banner)
	}
	bodyStart := len(lines)
	bodyRows := m.bodyHeight()

	rowCursor := 0
	windowStart := m.viewportOffset
	windowEnd := m.viewportOffset + bodyRows
	for i, c := range snap.cells {
		focused := i == snap.cursor
		h := c.HeightHint(m.width)
		cellEnd := rowCursor + h
		if cellEnd <= windowStart {
			rowCursor = cellEnd
			continue
		}
		if rowCursor >= windowEnd {
			break
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
		if len(lines)-bodyStart >= bodyRows {
			break
		}
	}
	target := bodyStart + bodyRows
	if len(lines) > target {
		lines = lines[:target]
	}
	for len(lines) < target {
		lines = append(lines, "")
	}
	lines = append(lines, m.statusLine(snap))
	return strings.Join(lines, "\n")
}

// statusLine builds the bottom row: mode + cursor position.
func (m model) statusLine(snap snapshot) string {
	mode := m.mode.Name()
	pos := "—"
	if len(snap.cells) > 0 {
		pos = strconv.Itoa(snap.cursor+1) + "/" + strconv.Itoa(len(snap.cells))
	}
	return mode + "  cell " + pos
}

// bodyHeight is the row count available for cells: total height
// minus the status row and (if present) the header row.
func (m model) bodyHeight() int {
	reserved := 1 // status row
	m.nb.store.mu.RLock()
	hasHeader := m.nb.store.header != ""
	m.nb.store.mu.RUnlock()
	if hasHeader {
		reserved++
	}
	body := m.height - reserved
	if body < 1 {
		body = 1
	}
	return body
}

// cellRowSpan returns the [start, end) absolute row range of cell
// idx, given the current width.
func (m model) cellRowSpan(cells []Cell, idx int) (int, int) {
	start := 0
	for i := 0; i < idx && i < len(cells); i++ {
		start += cells[i].HeightHint(m.width)
	}
	if idx < 0 || idx >= len(cells) {
		return start, start
	}
	return start, start + cells[idx].HeightHint(m.width)
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
	body := m.bodyHeight()
	start, end := m.cellRowSpan(snap.cells, snap.cursor)
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
