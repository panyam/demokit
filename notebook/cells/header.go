package cells

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/panyam/demokit/notebook"
)

// HeaderStyle is HeaderCell's per-cell styling. Concrete colors,
// no mode flag — light/dark selection happens at the theme layer
// (see DarkTheme / LightTheme in theme.go).
type HeaderStyle struct {
	BorderColor      color.Color
	FocusBorderColor color.Color
	TitleColor       color.Color
	BodyColor        color.Color
}

// DarkHeaderStyle returns the dark-terminal defaults — currently
// the package default.
func DarkHeaderStyle() HeaderStyle {
	return HeaderStyle{
		BorderColor:      lipgloss.Color("#7D56F4"),
		FocusBorderColor: lipgloss.Color("#FF6B6B"),
		TitleColor:       lipgloss.Color("#FAFAFA"),
		BodyColor:        lipgloss.Color("#CCCCCC"),
	}
}

// LightHeaderStyle returns the light-terminal defaults.
func LightHeaderStyle() HeaderStyle {
	return HeaderStyle{
		BorderColor:      lipgloss.Color("#5A3FCE"),
		FocusBorderColor: lipgloss.Color("#D34545"),
		TitleColor:       lipgloss.Color("#1A1A1A"),
		BodyColor:        lipgloss.Color("#444444"),
	}
}

// DefaultHeaderStyle returns the package default style — Dark.
func DefaultHeaderStyle() HeaderStyle { return DarkHeaderStyle() }

// HeaderCell renders a titled-text block — bold title plus a
// wrapped body. Read-only. Typically used as the header of a
// logical section (e.g. a demokit step header). Title and Body
// are mutable; Update() can rewrite them in place via
// notebook.Notebook.Update.
type HeaderCell struct {
	Title string
	Body  string
	Style HeaderStyle

	id            string
	cachedWidth   int
	cachedFocused bool
	cachedStyle   HeaderStyle
	cachedTitle   string
	cachedBody    string
	cachedLines   []string
	cachedHeight  int
}

// NewHeader builds a HeaderCell with DefaultHeaderStyle. Caller
// can mutate Style afterward or assign a Theme-provided style.
func NewHeader(id, title, body string) *HeaderCell {
	return &HeaderCell{
		id:    id,
		Title: title,
		Body:  body,
		Style: DefaultHeaderStyle(),
	}
}

// ID implements notebook.Cell.
func (c *HeaderCell) ID() string { return c.id }

// HeightHint implements notebook.Cell.
func (c *HeaderCell) HeightHint(width int) int {
	c.materialize(width, c.cachedFocused)
	return c.cachedHeight
}

// RenderRows implements notebook.Cell.
func (c *HeaderCell) RenderRows(width, startRow, endRow int, focused bool, _ notebook.Mode) []string {
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

// Update implements notebook.Cell. HeaderCell is read-only.
func (c *HeaderCell) Update(_ tea.Msg, _ notebook.Mode) (notebook.Cell, tea.Cmd) {
	return c, nil
}

// StatusHint implements notebook.Cell.
func (c *HeaderCell) StatusHint(_ notebook.Mode) string {
	return "Space/Shift+Enter advance"
}

func (c *HeaderCell) materialize(width int, focused bool) {
	if width <= 0 {
		width = 80
	}
	if c.cachedWidth == width &&
		c.cachedFocused == focused &&
		c.cachedStyle == c.Style &&
		c.cachedTitle == c.Title &&
		c.cachedBody == c.Body &&
		c.cachedLines != nil {
		return
	}
	border := c.Style.BorderColor
	if focused {
		border = c.Style.FocusBorderColor
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(c.Style.TitleColor)
	content := titleStyle.Render(c.Title)
	if strings.TrimSpace(c.Body) != "" {
		bodyStyle := lipgloss.NewStyle().Foreground(c.Style.BodyColor)
		content = content + "\n\n" + bodyStyle.Render(c.Body)
	}

	boxStyle := lipgloss.NewStyle().
		Border(focusedBorder(focused)).
		BorderForeground(border).
		Padding(0, 1).
		Width(maxBoxWidth(width))

	rendered := boxStyle.Render(content)
	c.cachedWidth = width
	c.cachedFocused = focused
	c.cachedStyle = c.Style
	c.cachedTitle = c.Title
	c.cachedBody = c.Body
	c.cachedLines = strings.Split(rendered, "\n")
	c.cachedHeight = len(c.cachedLines)
}
