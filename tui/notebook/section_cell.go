package notebook

import (
	"strings"

	"charm.land/lipgloss/v2"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/panyam/demokit"
)

// SectionCell renders a Note(...) / explanatory block. Stand-alone
// `c` in focused mode copies the body via demokit.Copy so a user
// can grab a chunk of context without having to mouse-select.
//
// Read-only otherwise; future EditMode would expose body editing.
type SectionCell struct {
	id      string
	title   string
	body    string
	palette Palette

	cachedWidth   int
	cachedFocused bool
	cachedLines   []string
	cachedHeight  int

	// copyMsg holds the transient "(copied …)" status line. Rendered
	// below the box so it doesn't reflow the cell geometry; cleared
	// via clearCopyMsgAfter on a tea.Tick.
	copyMsg string
}

// NewSectionCell builds a section cell.
func NewSectionCell(id, title, body string) *SectionCell {
	return &SectionCell{id: id, title: title, body: body, palette: DefaultPalette()}
}

// SetPalette overrides the cell's palette.
func (c *SectionCell) SetPalette(p Palette) { c.palette = p; c.cachedLines = nil }

// ID implements Cell.
func (c *SectionCell) ID() string { return c.id }

// HeightHint implements Cell.
func (c *SectionCell) HeightHint(width int) int {
	c.materialize(width, c.cachedFocused)
	h := c.cachedHeight
	if c.copyMsg != "" {
		h++
	}
	return h
}

// RenderRows implements Cell.
func (c *SectionCell) RenderRows(width, startRow, endRow int, focused bool, _ Mode) []string {
	c.materialize(width, focused)
	total := c.cachedHeight
	if c.copyMsg != "" {
		total++
	}
	if startRow < 0 {
		startRow = 0
	}
	if endRow > total {
		endRow = total
	}
	if startRow >= endRow {
		return nil
	}
	rows := make([]string, endRow-startRow)
	for i := startRow; i < endRow; i++ {
		if i < c.cachedHeight {
			rows[i-startRow] = c.cachedLines[i]
			continue
		}
		rows[i-startRow] = "  " + c.copyMsg
	}
	return rows
}

// Update implements Cell. In ViewMode, `c` copies the body.
func (c *SectionCell) Update(msg tea.Msg, mode Mode) (Cell, tea.Cmd) {
	if cm, ok := msg.(clearCopyMsg); ok && cm.cellID == c.id {
		c.copyMsg = ""
		return c, nil
	}
	if mode != ViewMode {
		return c, nil
	}
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return c, nil
	}
	if keyMsg.String() == "c" {
		strategy, ok := demokit.Copy(c.body)
		if ok {
			c.copyMsg = "(copied via " + strategy + ")"
		} else {
			c.copyMsg = "(copy failed — no clipboard provider)"
		}
		return c, clearCopyMsgAfter(c.id)
	}
	return c, nil
}

// StatusHint implements Cell.
func (c *SectionCell) StatusHint(_ Mode) string { return "c copy" }

func (c *SectionCell) materialize(width int, focused bool) {
	if width <= 0 {
		width = 80
	}
	if c.cachedWidth == width && c.cachedFocused == focused && c.cachedLines != nil {
		return
	}
	border := c.palette.SectionBorder
	if focused {
		border = c.palette.FocusBorder
	}

	titleStyle := lipgloss.NewStyle().Italic(true).Foreground(c.palette.Title)
	bodyStyle := lipgloss.NewStyle().Foreground(c.palette.Note)
	content := titleStyle.Render(c.title) + "\n\n" + bodyStyle.Render(c.body)

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(border).
		Padding(0, 1).
		Width(maxBoxWidth(width))

	rendered := boxStyle.Render(content)
	c.cachedWidth = width
	c.cachedFocused = focused
	c.cachedLines = strings.Split(rendered, "\n")
	c.cachedHeight = len(c.cachedLines)
}
