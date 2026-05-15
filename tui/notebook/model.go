package notebook

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// AdvanceMsg is emitted as a tea.Cmd when the user presses
// Enter / Space from SelectMode in the standalone demo path
// (WithQuitOnAdvance). The renderer-bridged path never sees this
// — its sync rendezvous goes through eventWaitForAdvance's Done
// channel instead.
type AdvanceMsg struct{}

// eventsAvailableMsg is the internal wake-up the model receives
// when the event queue has new content. The Update handler drains
// queue[offset:], applies each event, and re-arms the listener.
type eventsAvailableMsg struct{}

// Model is the Bubble Tea model — a pure projection of the
// renderer's event log. State changes happen exclusively in
// Apply(event); user input only drives navigation / focus / copy
// gestures, never producer state.
//
// The cell list, cursor, mode, and viewport offset are the
// presentation state. queue + offset + outputCellByVisit are the
// projection state.
type Model struct {
	cells  []Cell
	cursor int
	mode   Mode

	width  int
	height int

	// Renderer-bridged event log. nil in the standalone demo path
	// (no event-driven updates; main supplies cells at New time).
	queue  *eventQueue
	offset int

	// outputCellByVisit routes eventOutputChunk to the right cell.
	// A step's OutputCell lives here from eventStepReadyToRun
	// onward — chunks for that visit are appended to its buffer
	// even after eventStepEnd (live-graph use case).
	outputCellByVisit map[int]*OutputCell

	// waitCh, when non-nil, is the Done channel from a pending
	// eventWaitForAdvance. The key handler closes it on Enter to
	// release demokit's WaitForStep.
	waitCh chan struct{}

	// quitOnAdvance: if true (standalone demo), Enter emits
	// AdvanceMsg paired with tea.Quit. The renderer-bridged path
	// leaves it off.
	quitOnAdvance bool

	header     string
	headerDesc string
	done       bool

	viewportOffset int

	palette Palette
}

// New constructs a model over an initial cell list. Use New(nil)
// when the model will be event-driven (renderer-bridged); the
// queue + WithQueue do the rest.
func New(cells []Cell) Model {
	return Model{cells: cells, mode: SelectMode, palette: DefaultPalette()}
}

// WithQueue attaches an event queue. Required for the renderer-
// bridged path; nil-queue Model is the standalone-demo shape.
func (m Model) WithQueue(q *eventQueue) Model {
	m.queue = q
	return m
}

// WithPalette overrides the palette used for chrome (status,
// banner) and dynamically-constructed cells (PromptCell from
// eventPromptOpen).
func (m Model) WithPalette(p Palette) Model {
	m.palette = p
	return m
}

// WithQuitOnAdvance enables the standalone-demo affordance:
// Enter / Space pair AdvanceMsg with tea.Quit.
func (m Model) WithQuitOnAdvance() Model {
	m.quitOnAdvance = true
	return m
}

// Cells returns the current cell slice (read-only view).
func (m Model) Cells() []Cell { return m.cells }

// CursorIndex returns the cursor position.
func (m Model) CursorIndex() int { return m.cursor }

// Mode returns the current mode.
func (m Model) Mode() Mode { return m.mode }

// Init implements tea.Model. Returns the queue listener if one is
// attached; otherwise nil. The first call to Update with an
// eventsAvailableMsg drains everything queued before BT started,
// fixing the startup race by construction.
func (m Model) Init() tea.Cmd {
	if m.queue == nil {
		return nil
	}
	// Eager wake-up: even if no event has been appended yet, the
	// listener returns the moment one arrives. If events were
	// already queued before Init (the common case — demokit's
	// Execute starts firing events before BT's Run completes
	// setup), the notify channel's capacity-1 buffer holds the
	// wake-up and the listener returns immediately.
	return listenForEvents(m.queue)
}

// listenForEvents returns a tea.Cmd that blocks on the queue's
// notify channel and emits eventsAvailableMsg when a new event
// arrives. Re-armed by the Update handler after each drain.
func listenForEvents(q *eventQueue) tea.Cmd {
	return func() tea.Msg {
		<-q.Notify()
		return eventsAvailableMsg{}
	}
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.invalidateCaches()
		return m, nil
	case eventsAvailableMsg:
		// Drain everything currently queued, apply in order, then
		// re-arm. Multiple events appended between wake-ups get
		// applied in a single Update — one render at the end.
		events, newOffset := m.queue.Read(m.offset)
		for _, e := range events {
			m.applyEvent(e)
		}
		m.offset = newOffset
		return m, listenForEvents(m.queue)
	case clearCopyMsg:
		// Route to the cell that owns the toast; cells that don't
		// match ignore it.
		for i, c := range m.cells {
			if c.ID() != msg.cellID {
				continue
			}
			updated, cmd := c.Update(msg, ViewMode)
			m.cells[i] = updated
			return m, cmd
		}
		return m, nil
	case cellAdvanceMsg:
		// A focused cell finished and wants us back to SelectMode +
		// advance the demo. Same rendezvous as Enter: close the
		// pending wait channel; if none, fall back to AdvanceMsg
		// (standalone-demo path).
		m.mode = SelectMode
		if m.waitCh != nil {
			close(m.waitCh)
			m.waitCh = nil
			return m, nil
		}
		if m.quitOnAdvance {
			return m, tea.Sequence(emitAdvance, tea.Quit)
		}
		return m, emitAdvance
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// applyEvent is the single state-mutation entry point. Every
// renderer-side event passes through here. Pure mutation — no
// I/O, no tea.Cmd return — so the function is trivially
// testable.
//
// Pointer receiver because we're mutating the model. Update
// returns the modified value by Go's auto-addressing.
func (m *Model) applyEvent(e Event) {
	switch e := e.(type) {
	case eventHeader:
		m.header = e.Title
		m.headerDesc = e.Description
	case eventSection:
		id := "section#" + slugify(e.Title)
		cell := NewSectionCell(id, e.Title, e.Body)
		cell.SetPalette(m.palette)
		m.cells = append(m.cells, cell)
		m.invalidateCaches()
	case eventStepStart:
		firstNew := len(m.cells)
		m.cells = append(m.cells, e.BodyCells...)
		m.cursor = firstNew
		m.mode = SelectMode
		m.invalidateCaches()
		m.ensureCursorVisible()
	case eventStepReadyToRun:
		oc, _ := e.Output.(*OutputCell)
		if oc != nil {
			if m.outputCellByVisit == nil {
				m.outputCellByVisit = map[int]*OutputCell{}
			}
			m.outputCellByVisit[e.Visit] = oc
		}
		m.cells = append(m.cells, e.Output)
		m.invalidateCaches()
		m.ensureCursorVisible()
	case eventOutputChunk:
		// Route by visit. Chunks for a visit can arrive after
		// eventStepEnd (a step's Run spawned a background
		// goroutine that keeps emitting). MarkDone is a label, not
		// a seal — keep appending.
		if oc, ok := m.outputCellByVisit[e.Visit]; ok && oc != nil {
			oc.buf.Append(e.Chunk)
		}
	case eventStepEnd:
		if oc, ok := m.outputCellByVisit[e.Visit]; ok && oc != nil {
			oc.MarkDone()
			if e.Result != nil && e.Result.Err != nil {
				oc.buf.Append([]byte("\n[error] " + e.Result.Err.Error() + "\n"))
			}
		}
	case eventDone:
		m.done = true
	case eventPromptOpen:
		pid := fmt.Sprintf("prompt#%d", len(m.cells))
		cell := NewPromptCell(pid, e.Inputs, e.Reply, m.palette)
		m.cells = append(m.cells, cell)
		m.cursor = len(m.cells) - 1
		m.mode = ViewMode
		m.invalidateCaches()
		m.ensureCursorVisible()
	case eventWaitForAdvance:
		m.waitCh = e.Done
	}
}

// handleKey dispatches keystrokes by mode.
func (m Model) handleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "ctrl+c", "ctrl+d":
		return m, tea.Quit
	case "ctrl+l":
		return m, tea.ClearScreen
	}
	if m.mode == SelectMode {
		switch key.String() {
		case "q":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.ensureCursorVisible()
			}
			return m, nil
		case "down", "j":
			if m.cursor < len(m.cells)-1 {
				m.cursor++
				m.ensureCursorVisible()
			}
			return m, nil
		case "s", "f":
			if m.cursorOnFocusable() {
				m.mode = ViewMode
			}
			return m, nil
		case "enter", " ":
			if m.done {
				return m, tea.Quit
			}
			if m.waitCh != nil {
				close(m.waitCh)
				m.waitCh = nil
				return m, nil
			}
			if m.quitOnAdvance {
				return m, tea.Sequence(emitAdvance, tea.Quit)
			}
			return m, emitAdvance
		}
		return m, nil
	}

	// FocusedMode (ViewMode): Esc pops back; otherwise delegate.
	if key.String() == "esc" {
		m.mode = SelectMode
		return m, nil
	}
	idx := m.cursor
	if idx < 0 || idx >= len(m.cells) {
		return m, nil
	}
	updated, cmd := m.cells[idx].Update(key, m.mode)
	m.cells[idx] = updated
	return m, cmd
}

// cursorOnFocusable returns true if the cursor cell can be
// entered. Phase A allows entry on any cell.
func (m Model) cursorOnFocusable() bool {
	return m.cursor >= 0 && m.cursor < len(m.cells)
}

// emitAdvance is the standalone-demo AdvanceMsg producer.
func emitAdvance() tea.Msg { return AdvanceMsg{} }

// invalidateCaches walks every cell and clears its width-dependent
// caches by re-querying HeightHint at the current width.
func (m *Model) invalidateCaches() {
	if m.width <= 0 {
		return
	}
	for _, c := range m.cells {
		_ = c.HeightHint(m.width)
	}
}

// bodyHeight returns rows available for cell content (terminal
// height minus header banner + bottom status row).
func (m *Model) bodyHeight() int {
	reserved := 1
	if m.header != "" {
		reserved++
	}
	body := m.height - reserved
	if body < 1 {
		body = 1
	}
	return body
}

// cellRowSpan returns the [start, end) row range the cell at idx
// occupies in the unscrolled stack.
func (m *Model) cellRowSpan(idx int) (int, int) {
	if idx < 0 || idx >= len(m.cells) {
		return 0, 0
	}
	start := 0
	for i := 0; i < idx; i++ {
		start += m.cells[i].HeightHint(m.width)
	}
	return start, start + m.cells[idx].HeightHint(m.width)
}

// ensureCursorVisible scrolls viewportOffset just enough to put
// the focused cell on screen. Cells taller than the viewport pin
// to their top.
func (m *Model) ensureCursorVisible() {
	if m.width == 0 || m.height == 0 {
		return
	}
	body := m.bodyHeight()
	start, end := m.cellRowSpan(m.cursor)
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

// View implements tea.Model.
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	reserved := 1
	if m.header != "" {
		reserved++
	}
	bodyRows := m.height - reserved
	if bodyRows < 1 {
		bodyRows = 1
	}

	var lines []string
	if m.header != "" {
		banner := "≡ " + m.header
		if m.done {
			banner += "  · Done."
		}
		lines = append(lines, banner)
	}

	bodyStart := len(lines)
	rowCursor := 0
	windowStart := m.viewportOffset
	windowEnd := m.viewportOffset + bodyRows
	for i, c := range m.cells {
		focused := i == m.cursor
		h := c.HeightHint(m.width)
		cellEnd := rowCursor + h
		if cellEnd <= windowStart {
			rowCursor = cellEnd
			continue
		}
		if rowCursor >= windowEnd {
			break
		}
		lo := 0
		hi := h
		if rowCursor < windowStart {
			lo = windowStart - rowCursor
		}
		if cellEnd > windowEnd {
			hi = h - (cellEnd - windowEnd)
		}
		rows := c.RenderRows(m.width, lo, hi, focused, m.mode)
		lines = append(lines, rows...)
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
	lines = append(lines, m.statusLine())
	return strings.Join(lines, "\n")
}

// statusLine builds the bottom-row status banner.
func (m Model) statusLine() string {
	if m.cursor < 0 || m.cursor >= len(m.cells) {
		return "[" + m.mode.Name() + "]"
	}
	c := m.cells[m.cursor]
	hint := c.StatusHint(m.mode)
	if m.mode == SelectMode {
		if m.done {
			hint = "↑/↓ navigate · Enter exit · q quit"
		} else {
			hint = "↑/↓ navigate · Enter advance · s/f focus · q quit"
		}
	}
	return "[" + m.mode.Name() + "] " + c.ID() + " · " + hint + " · Ctrl+L refresh"
}
