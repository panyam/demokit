package notebook

import (
	"strings"

	"charm.land/lipgloss/v2"
	tea "github.com/charmbracelet/bubbletea"
)

// MetaCell is the always-present step-header cell — title plus the
// step's body of arrows / refs / note rendered as plain wrapped
// text. Read-only in Phase A; future EditMode would let the user
// rename or rewrite the note.
//
// Renders as a lipgloss-bordered box; the border color comes from
// the configured Palette (MetaBorder when unfocused, FocusBorder
// when focused). Width caching keeps repeated HeightHint calls
// cheap.
type MetaCell struct {
	id      string
	title   string
	body    string
	palette Palette

	// Box geometry cache: cachedFor describes the (width, focused)
	// pair the cached lines/height correspond to. Invalidated when
	// either changes.
	cachedWidth   int
	cachedFocused bool
	cachedLines   []string
	cachedHeight  int
}

// NewMetaCell builds a MetaCell. The cell uses DefaultPalette until
// the renderer bridge overrides it via SetPalette — keeps test
// construction trivial.
func NewMetaCell(id, title, body string) *MetaCell {
	return &MetaCell{id: id, title: title, body: body, palette: DefaultPalette()}
}

// SetPalette overrides the cell's palette. Called by the renderer
// bridge so the notebook-level theme propagates to every cell.
func (c *MetaCell) SetPalette(p Palette) { c.palette = p; c.cachedLines = nil }

// ID implements Cell.
func (c *MetaCell) ID() string { return c.id }

// HeightHint implements Cell.
func (c *MetaCell) HeightHint(width int) int {
	c.materialize(width, c.cachedFocused)
	return c.cachedHeight
}

// RenderRows implements Cell — returns the half-open row range,
// clamped to availability so a viewport asking past the end gets
// fewer rows back.
func (c *MetaCell) RenderRows(width, startRow, endRow int, focused bool, _ Mode) []string {
	c.materialize(width, focused)
	if startRow < 0 {
		startRow = 0
	}
	if endRow > c.cachedHeight {
		endRow = c.cachedHeight
	}
	if startRow >= endRow {
		return nil
	}
	out := make([]string, endRow-startRow)
	copy(out, c.cachedLines[startRow:endRow])
	return out
}

// Update implements Cell. MetaCell is read-only in Phase A.
func (c *MetaCell) Update(_ tea.Msg, _ Mode) (Cell, tea.Cmd) { return c, nil }

// StatusHint implements Cell — MetaCell exposes only the
// advance-step gesture since it has no per-cell action.
func (c *MetaCell) StatusHint(_ Mode) string {
	return "Space/Shift+Enter advance"
}

// materialize rebuilds cachedLines + cachedHeight when width or
// focus state changes. The box renders title (bold, palette.Title)
// + an optional body (wrapped to fit inside the border).
func (c *MetaCell) materialize(width int, focused bool) {
	if width <= 0 {
		width = 80
	}
	if c.cachedWidth == width && c.cachedFocused == focused && c.cachedLines != nil {
		return
	}
	border := c.palette.MetaBorder
	if focused {
		border = c.palette.FocusBorder
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(c.palette.Title)
	content := titleStyle.Render(c.title)
	if strings.TrimSpace(c.body) != "" {
		bodyStyle := lipgloss.NewStyle().Foreground(c.palette.Note)
		content = content + "\n\n" + bodyStyle.Render(c.body)
	}

	boxStyle := lipgloss.NewStyle().
		Border(focusedBorder(focused)).
		BorderForeground(border).
		Padding(0, 1).
		Width(maxBoxWidth(width))

	rendered := boxStyle.Render(content)
	c.cachedWidth = width
	c.cachedFocused = focused
	c.cachedLines = strings.Split(rendered, "\n")
	c.cachedHeight = len(c.cachedLines)
}

// maxBoxWidth returns the inner content width for a lipgloss
// rounded-border box at the given outer width: 2 chars for the
// vertical borders + 2 chars of horizontal padding = 4 chars
// reserved. Clamped to a minimum so narrow terminals don't blow up
// the wrap logic.
func maxBoxWidth(outer int) int {
	w := outer - 4
	if w < 10 {
		w = 10
	}
	return w
}
