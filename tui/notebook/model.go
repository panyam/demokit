package notebook

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/panyam/demokit"
	"github.com/panyam/demokit/events"
)

// AdvanceMsg is emitted as a tea.Cmd when the user presses
// Enter / Space from SelectMode and no queue rendezvous is
// pending. The renderer-bridged path resolves the wait through
// the event queue; this msg only fires for the standalone test
// path (WithQuitOnAdvance).
type AdvanceMsg struct{}

// eventsAvailableMsg is the internal wake-up the model receives
// when the event queue has new content.
type eventsAvailableMsg struct{}

// Model is the Bubble Tea model — a pure projection of the
// demo's event log. State changes happen exclusively in
// applyEvent(offset, event); user input only drives
// navigation / focus / copy gestures and resolves pending sync
// events via the queue.
type Model struct {
	cells  []Cell
	cursor int
	mode   Mode

	width  int
	height int

	// Renderer-bridged event log. nil for standalone test usage.
	queue  *events.EventQueue
	sub    *events.Subscription // per-model wake-up handle
	offset int

	// outputCellByVisit routes events.OutputChunk to the right
	// cell. A step's OutputCell lives here from
	// events.StepReadyToRun onward. Chunks for the visit keep
	// applying even after events.StepEnd.
	outputCellByVisit map[int]*OutputCell

	// stepIDByVisit lets StepReadyToRun reconstruct a stable
	// OutputCell ID matching the MetaCell's slug. Populated on
	// applyEvent(StepStart).
	stepIDByVisit map[int]string

	// pendingWaitOffset is the queue offset of the latest
	// outstanding WaitForAdvance, or -1 when none. Enter / Space
	// from SelectMode resolves it via queue.Resolve.
	pendingWaitOffset int

	quitOnAdvance bool

	header     string
	headerDesc string
	done       bool

	viewportOffset int

	palette Palette
}

// New constructs a model. Use New(nil) when event-driven; the
// queue is wired separately via WithQueue.
func New(cells []Cell) Model {
	return Model{
		cells: cells, mode: SelectMode, palette: DefaultPalette(),
		pendingWaitOffset: -1,
	}
}

// WithQueue attaches an event queue. Subscribes immediately so
// the model has its own wake-up channel, independent of any
// other consumer on the same queue (Kafka-style multi-reader).
func (m Model) WithQueue(q *events.EventQueue) Model {
	m.queue = q
	if q != nil {
		m.sub = q.Subscribe()
	}
	return m
}

// WithPalette overrides the palette.
func (m Model) WithPalette(p Palette) Model {
	m.palette = p
	return m
}

// WithQuitOnAdvance enables the standalone-test affordance.
func (m Model) WithQuitOnAdvance() Model {
	m.quitOnAdvance = true
	return m
}

// Cells returns the current cell slice.
func (m Model) Cells() []Cell { return m.cells }

// CursorIndex returns the cursor position.
func (m Model) CursorIndex() int { return m.cursor }

// Mode returns the current mode.
func (m Model) Mode() Mode { return m.mode }

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	if m.sub == nil {
		return repaintTick()
	}
	return tea.Batch(listenForEvents(m.sub), repaintTick())
}

// listenForEvents returns a tea.Cmd that blocks on the
// subscription's notify channel and emits eventsAvailableMsg.
// Per-subscription channel means this consumer doesn't compete
// with any other consumer for wake-up tokens.
func listenForEvents(sub *events.Subscription) tea.Cmd {
	return func() tea.Msg {
		<-sub.Notify()
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
		all, newOffset := m.queue.ReadFrom(m.offset)
		for i, e := range all {
			m.applyEvent(m.offset+i, e)
		}
		m.offset = newOffset
		return m, listenForEvents(m.sub)
	case repaintTickMsg:
		return m, repaintTick()
	case clearCopyMsg:
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
		m.mode = SelectMode
		return m.resolvePendingWait("user-submitted-cell")
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// applyEvent is the single state-mutation entry point. Pure
// mutation per event type — no I/O, no Cmd returns.
func (m *Model) applyEvent(offset int, e events.Event) {
	switch e := e.(type) {
	case events.Header:
		m.header = e.Title
		m.headerDesc = e.Description
	case events.Section:
		id := "section#" + slugify(e.Title)
		cell := NewSectionCell(id, e.Title, e.Body)
		cell.SetPalette(m.palette)
		m.cells = append(m.cells, cell)
		m.invalidateCaches()
	case events.StepStart:
		if m.stepIDByVisit == nil {
			m.stepIDByVisit = map[int]string{}
		}
		m.stepIDByVisit[e.Visit] = e.StepID
		bodyCells := buildCellsFromStepStart(e, m.palette)
		firstNew := len(m.cells)
		m.cells = append(m.cells, bodyCells...)
		m.cursor = firstNew
		m.mode = SelectMode
		m.invalidateCaches()
		m.ensureCursorVisible()
	case events.StepReadyToRun:
		base := slugify(m.stepIDByVisit[e.Visit])
		if base == "" {
			base = fmt.Sprintf("step%d", e.Visit)
		}
		outputID := fmt.Sprintf("%s#%d.output", base, e.Visit)
		buf := NewOutputBuffer()
		oc := NewOutputCell(outputID, buf, 12)
		oc.SetPalette(m.palette)
		if m.outputCellByVisit == nil {
			m.outputCellByVisit = map[int]*OutputCell{}
		}
		m.outputCellByVisit[e.Visit] = oc
		m.cells = append(m.cells, oc)
		m.invalidateCaches()
		m.ensureCursorVisible()
	case events.OutputChunk:
		if oc, ok := m.outputCellByVisit[e.Visit]; ok && oc != nil {
			oc.buf.Append(e.Chunk)
		}
	case events.StepEnd:
		if oc, ok := m.outputCellByVisit[e.Visit]; ok && oc != nil {
			oc.MarkDone()
			if e.Status == "error" && e.ErrorText != "" {
				oc.buf.Append([]byte("\n[error] " + e.ErrorText + "\n"))
			}
		}
	case events.Done:
		m.done = true
	case events.PromptOpen:
		pid := fmt.Sprintf("prompt#%d", len(m.cells))
		cell := NewPromptCell(pid, e.Inputs, m.queue, offset, m.palette)
		m.cells = append(m.cells, cell)
		m.cursor = len(m.cells) - 1
		m.mode = ViewMode
		m.invalidateCaches()
		m.ensureCursorVisible()
	case events.WaitForAdvance:
		m.pendingWaitOffset = offset
	}
}

// resolvePendingWait is the shared resolve path for Enter /
// Space / cellAdvance. Closes the outstanding WaitForAdvance via
// the event queue and returns (model, advance cmd if needed).
func (m Model) resolvePendingWait(source string) (tea.Model, tea.Cmd) {
	if m.pendingWaitOffset >= 0 && m.queue != nil {
		_ = m.queue.Resolve(m.pendingWaitOffset, &events.AdvanceResolution{
			Source: source, Timestamp: time.Now(),
		})
		m.pendingWaitOffset = -1
		return m, nil
	}
	if m.quitOnAdvance {
		return m, tea.Sequence(emitAdvance, tea.Quit)
	}
	return m, emitAdvance
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
			return m.resolvePendingWait("user-enter")
		}
		return m, nil
	}
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

func (m Model) cursorOnFocusable() bool {
	return m.cursor >= 0 && m.cursor < len(m.cells)
}

func emitAdvance() tea.Msg { return AdvanceMsg{} }

func (m *Model) invalidateCaches() {
	if m.width <= 0 {
		return
	}
	for _, c := range m.cells {
		_ = c.HeightHint(m.width)
	}
}

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
	} else {
		hint = hint + " · Esc back"
	}
	return "[" + m.mode.Name() + "] " + c.ID() + " · " + hint + " · Ctrl+L refresh"
}

// --- Cell construction from events ---

// buildCellsFromStepStart projects an events.StepStart into the
// notebook's cell representation: MetaCell (title + body) plus
// one VerbatimCell per declared verbatim block.
func buildCellsFromStepStart(e events.StepStart, palette Palette) []Cell {
	base := slugify(e.StepID)
	if base == "" {
		base = fmt.Sprintf("step%d", e.Visit)
	}
	body := buildMetaBody(e)
	metaID := fmt.Sprintf("%s#%d.meta", base, e.Visit)
	meta := NewMetaCell(metaID, e.Title, body)
	meta.SetPalette(palette)
	cells := []Cell{meta}
	for i, vb := range e.Verbatims {
		vid := fmt.Sprintf("%s#%d.verbatim%d", base, e.Visit, i)
		variants := make([]demokit.Variant, len(vb.Variants))
		for j, v := range vb.Variants {
			variants[j] = demokit.Variant{
				Label: v.Label, Lang: v.Lang, Content: v.Content, IsDefault: v.IsDefault,
			}
		}
		vc := NewVerbatimCell(vid, vb.Label, variants)
		vc.SetPalette(palette)
		cells = append(cells, vc)
	}
	return cells
}

// buildMetaBody joins a step's note + arrows + refs into the
// MetaCell body string.
func buildMetaBody(e events.StepStart) string {
	var parts []string
	if note := strings.TrimSpace(e.Note); note != "" {
		parts = append(parts, note)
	}
	if len(e.Arrows) > 0 {
		var lines []string
		for _, a := range e.Arrows {
			arrow := "->"
			if a.Dashed {
				arrow = "-->"
			}
			label := ""
			if a.Label != "" {
				label = ": " + a.Label
			}
			lines = append(lines, fmt.Sprintf("%s %s %s%s", a.From, arrow, a.To, label))
		}
		parts = append(parts, strings.Join(lines, "\n"))
	}
	if len(e.Refs) > 0 {
		var lines []string
		for _, ref := range e.Refs {
			line := ref.Name
			if ref.URL != "" {
				if line == "" {
					line = ref.URL
				} else {
					line = line + " (" + ref.URL + ")"
				}
			}
			lines = append(lines, "ref: "+line)
		}
		parts = append(parts, strings.Join(lines, "\n"))
	}
	return strings.Join(parts, "\n\n")
}

// slugify normalizes IDs for cell prefixes.
func slugify(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return ""
	}
	var b strings.Builder
	prevDash := true
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevDash = false
		case r == '-' || r == '_':
			b.WriteByte('-')
			prevDash = true
		default:
			if !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	return strings.TrimRight(b.String(), "-")
}

// --- Ticker ---

const repaintInterval = 16 * time.Millisecond

type repaintTickMsg struct{}

func repaintTick() tea.Cmd {
	return tea.Tick(repaintInterval, func(time.Time) tea.Msg { return repaintTickMsg{} })
}
