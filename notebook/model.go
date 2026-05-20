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
	store          *store
	rdv            *rendezvous
	viewportOffset int
	width          int
	height         int
	mode           Mode
	ready          chan struct{}
}

func newModel(s *store, rdv *rendezvous, ready chan struct{}, width, height int) model {
	return model{
		store:  s,
		rdv:    rdv,
		mode:   SelectMode,
		width:  width,
		height: height,
		ready:  ready,
	}
}

// Init implements tea.Model. The first batched cmd closes ready
// once the program loop is live — Snapshot waits on it before
// issuing a Send.
func (m model) Init() tea.Cmd {
	ready := m.ready
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
	case CellAdvanceMsg:
		m.mode = SelectMode
		m.store.moveCursor(+1)
		m.ensureCursorVisible()
		return m, nil
	case PromptSubmittedMsg:
		// The PromptCell already updated itself; the model just
		// resolves the pending AwaitInputBy and moves on like any
		// other "this cell is done" event.
		if m.rdv != nil {
			m.rdv.resolveInput(msg.CellID, msg.Answers, "user-submitted")
		}
		m.mode = SelectMode
		m.store.moveCursor(+1)
		m.ensureCursorVisible()
		return m, nil
	case ClearCopyMsg:
		return m, m.routeToCell(msg.CellID, msg)
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// handleKey routes a keystroke. Global quit keys first, then
// mode-specific navigation, then the focused cell. A cell sees
// the key in both modes (so 'c'-copy works while navigating); the
// cell itself gates behavior on mode.
func (m model) handleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	}
	if m.mode == SelectMode {
		switch key.String() {
		case "up", "k":
			m.store.moveCursor(-1)
			m.ensureCursorVisible()
			return m, nil
		case "down", "j":
			m.store.moveCursor(+1)
			m.ensureCursorVisible()
			return m, nil
		case "s", "f":
			if m.store.count() > 0 {
				m.mode = ViewMode
			}
			return m, nil
		case "enter", " ":
			// Resolve a pending AwaitAdvance, if any. If no
			// advance is pending, Enter is a no-op in SelectMode.
			if m.rdv != nil {
				m.rdv.resolveAdvance("user-enter")
			}
			return m, nil
		}
		// Fall through: route other keys (notably 'c') to the
		// focused cell even in SelectMode.
		return m, m.routeKeyToCursor(key)
	}
	// ViewMode.
	if key.String() == "esc" {
		m.mode = SelectMode
		return m, nil
	}
	return m, m.routeKeyToCursor(key)
}

// routeKeyToCursor delivers a key to the focused cell, applying
// any cell mutation in place via store.update.
func (m model) routeKeyToCursor(key tea.KeyMsg) tea.Cmd {
	snap := m.store.snapshot()
	if snap.cursor < 0 || snap.cursor >= len(snap.cells) {
		return nil
	}
	return m.routeToCell(snap.cells[snap.cursor].ID(), key)
}

// routeToCell delivers a message to the cell with the given ID
// and writes the (possibly new) cell back into the store.
func (m model) routeToCell(id CellID, msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	m.store.update(id, func(c Cell) Cell {
		updated, c2 := c.Update(msg, m.mode)
		cmd = c2
		return updated
	})
	return cmd
}

// View implements tea.Model. Range-based render: each cell is
// asked only for the row window the viewport will display.
func (m model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	snap := m.store.snapshot()

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
	m.store.mu.RLock()
	hasHeader := m.store.header != ""
	m.store.mu.RUnlock()
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
	snap := m.store.snapshot()
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

