package notebook

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// AdvanceMsg is what the model emits as a tea.Cmd when the user
// presses Space / Shift+Enter from SelectMode (Phase A.1's
// "advance to next step" signal). The renderer bridge consumes it
// from outside the program in PR2; in Phase A.1's standalone demo
// the program simply quits on receipt.
type AdvanceMsg struct{}

// Model is the Bubble Tea model that owns the cell list, cursor
// position, and current mode. Construction is `New(cells)`; the
// program then drives it via Init/Update/View. The model is a value
// type — Update returns a fresh copy with mutated state, the
// standard Bubble Tea pattern.
//
// Phase A.1 ships single-step-on-screen: the model is given a flat
// []Cell at construction time; reassignment happens between steps
// (via SetCells, called by the renderer bridge in PR2). Cross-step
// retention is Phase B.
type Model struct {
	cells  []Cell
	cursor int
	mode   Mode

	// width / height are the latest terminal size reported by
	// WindowSizeMsg. Used by RenderRows and to compute the visible
	// viewport row range.
	width  int
	height int

	// statusOverride is a transient banner line (e.g. "still running")
	// that replaces the cell's StatusHint on the bottom row.
	statusOverride string

	// quitOnAdvance: if true (the standalone demo / smoke entry), an
	// AdvanceMsg quits the program. In renderer-bridged mode (PR2)
	// this stays false and the bridge consumes AdvanceMsg.
	quitOnAdvance bool

	// initCmd is dispatched once on the first tick — used by the
	// standalone demo / renderer bridge to wire up streaming
	// subscriptions. nil means "no initial command."
	initCmd tea.Cmd

	// resubscribe maps OutputAppendedMsg.CellID → a tea.Cmd that
	// re-listens on that buffer. Registered via WithOutputSubscription
	// so the model can keep streaming alive without knowing about
	// OutputBuffer internals.
	resubscribe map[string]tea.Cmd
}

// New constructs a model over a flat cell list. Cursor starts on
// the first cell; mode starts in SelectMode (the outermost level).
func New(cells []Cell) Model {
	return Model{cells: cells, mode: SelectMode}
}

// WithQuitOnAdvance enables a small-demo affordance: pressing the
// advance key emits tea.Quit alongside the AdvanceMsg. PR2's
// renderer bridge leaves it off and consumes AdvanceMsg itself.
func (m Model) WithQuitOnAdvance() Model {
	m.quitOnAdvance = true
	return m
}

// WithOutputSubscription registers a streaming OutputBuffer with the
// model. The model's Init returns a tea.Cmd that listens on the
// buffer; each OutputAppendedMsg the model receives triggers an
// auto-resubscribe so the listener stays alive for the buffer's
// lifetime. Call once per streaming cell — the model accumulates
// subscriptions and merges their Init commands.
func (m Model) WithOutputSubscription(buf *OutputBuffer, cellID string) Model {
	cmd := SubscribeOutputBuffer(buf, cellID)
	if m.resubscribe == nil {
		m.resubscribe = map[string]tea.Cmd{}
	}
	m.resubscribe[cellID] = cmd
	if m.initCmd == nil {
		m.initCmd = cmd
	} else {
		prev := m.initCmd
		m.initCmd = tea.Batch(prev, cmd)
	}
	return m
}

// Cells returns the current cell slice (read-only view).
func (m Model) Cells() []Cell { return m.cells }

// CursorIndex returns the cursor position.
func (m Model) CursorIndex() int { return m.cursor }

// Mode returns the current mode.
func (m Model) Mode() Mode { return m.mode }

// SetCells installs a new cell list and resets cursor + mode to
// SelectMode/0. Used by the renderer bridge in PR2 between steps;
// also useful for tests.
func (m Model) SetCells(cells []Cell) Model {
	m.cells = cells
	m.cursor = 0
	m.mode = SelectMode
	m.invalidateCaches()
	return m
}

// Init implements tea.Model. Returns the accumulated subscription
// commands (from WithOutputSubscription) so streaming buffers start
// pushing OutputAppendedMsg events immediately.
func (m Model) Init() tea.Cmd { return m.initCmd }

// Update implements tea.Model. Routes input by mode:
//
//   - WindowSizeMsg: update terminal dims, invalidate caches.
//   - clearCopyMsg: route to the addressed cell so it can drop its
//     toast.
//   - SelectMode: ↑/↓ moves cursor; Enter focuses; Space/Shift+Enter
//     emits AdvanceMsg; q / ctrl+c quit.
//   - FocusedMode (ViewMode in Phase A.1): Esc → SelectMode; all
//     other keys delegate to the focused cell.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.invalidateCaches()
		return m, nil
	case clearCopyMsg:
		// Route to the cell that owns the toast; cells that don't
		// match ignore it.
		for i, c := range m.cells {
			if c.ID() != msg.cellID {
				continue
			}
			updated, cmd := c.Update(msg, ViewMode) // mode doesn't matter for clear
			m.cells[i] = updated
			return m, cmd
		}
		return m, nil
	case OutputAppendedMsg:
		// New committed lines in this buffer — Bubble Tea redraws on
		// any Update return, so the view picks up the new content
		// without us touching cell state. We just need to re-arm the
		// listener so future appends keep flowing.
		if resub, ok := m.resubscribe[msg.CellID]; ok {
			return m, resub
		}
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// handleKey dispatches keystrokes by mode. Kept separate from
// Update so the switch over Msg types stays readable.
func (m Model) handleKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.mode == SelectMode {
		switch key.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil
		case "down", "j":
			if m.cursor < len(m.cells)-1 {
				m.cursor++
			}
			return m, nil
		case "enter":
			if m.cursorOnFocusable() {
				m.mode = ViewMode
			}
			return m, nil
		case " ", "shift+enter":
			if m.quitOnAdvance {
				return m, tea.Sequence(emitAdvance, tea.Quit)
			}
			return m, emitAdvance
		}
		return m, nil
	}

	// FocusedMode (ViewMode): Esc pops back to SelectMode; otherwise
	// delegate to the focused cell.
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

// cursorOnFocusable returns true if the current cursor cell can be
// entered. MetaCell technically has no focused-mode actions, but we
// still allow focusing it for visual symmetry — Esc gets the user
// back out instantly.
func (m Model) cursorOnFocusable() bool {
	return m.cursor >= 0 && m.cursor < len(m.cells)
}

// emitAdvance is a tea.Cmd factory that returns AdvanceMsg. Kept as
// a function (not a value) because tea.Cmd is itself a function type.
func emitAdvance() tea.Msg { return AdvanceMsg{} }

// invalidateCaches walks every cell and clears its width-dependent
// caches by forcing a HeightHint call at the new width — concrete
// cells rebuild on width change automatically.
func (m *Model) invalidateCaches() {
	if m.width <= 0 {
		return
	}
	for _, c := range m.cells {
		// HeightHint is the cheap entry point; cells trigger their
		// internal materialize() and rebuild caches lazily.
		_ = c.HeightHint(m.width)
	}
}

// View implements tea.Model. Renders cells top-to-bottom, clipping
// to the terminal viewport. Bottom row is a status line that shows
// the focused cell's StatusHint or a mode banner.
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		// Bubble Tea hasn't sent the first WindowSizeMsg yet; render
		// a blank screen so we don't print partial garbage.
		return ""
	}

	bodyRows := m.height - 1 // reserve last row for status
	if bodyRows < 1 {
		bodyRows = 1
	}

	// Phase A.1: render every cell top-to-bottom in declaration
	// order. Cross-step viewport scrolling is Phase B.
	var lines []string
	for i, c := range m.cells {
		focused := i == m.cursor
		h := c.HeightHint(m.width)
		rows := c.RenderRows(m.width, 0, h, focused, m.mode)
		lines = append(lines, rows...)
		if len(lines) >= bodyRows {
			break
		}
	}
	// Pad or trim to exactly bodyRows.
	if len(lines) > bodyRows {
		lines = lines[:bodyRows]
	}
	for len(lines) < bodyRows {
		lines = append(lines, "")
	}
	lines = append(lines, m.statusLine())
	return strings.Join(lines, "\n")
}

// statusLine builds the bottom-row banner. Format:
//
//   [MODE]  cellID  ·  <cell hint>          [override or countdown ]
//
// Phase A.1 keeps it simple — left side is mode + cursor cell ID +
// the focused cell's hint; right side is unused (reserved for the
// countdown overlay, which lands with the bridge in PR2).
func (m Model) statusLine() string {
	if m.statusOverride != "" {
		return m.statusOverride
	}
	if m.cursor < 0 || m.cursor >= len(m.cells) {
		return "[" + m.mode.Name() + "]"
	}
	c := m.cells[m.cursor]
	hint := c.StatusHint(m.mode)
	if m.mode == SelectMode {
		// At the outer level, advance is the dominant action.
		hint = "↑/↓ navigate · Enter focus · Space advance · q quit"
	}
	return "[" + m.mode.Name() + "] " + c.ID() + " · " + hint
}
