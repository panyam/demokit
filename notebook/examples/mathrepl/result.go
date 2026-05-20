package main

import (
	"strings"

	"charm.land/lipgloss/v2"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/panyam/demokit/notebook"
)

// ResultCell is a minimal custom notebook.Cell: one row showing
// either `<expr> = <value>` or `<expr> → error: <msg>`, inside a
// rounded border.
//
// Lives in the mathrepl module to demonstrate that consumers can
// implement notebook.Cell themselves without touching notebook/.
type ResultCell struct {
	id      string
	expr    string
	value   string
	errMsg  string
	rendered []string
}

// NewResult builds a ResultCell. errMsg is "" on success.
func NewResult(id, expr, value, errMsg string) *ResultCell {
	return &ResultCell{id: id, expr: expr, value: value, errMsg: errMsg}
}

// ID implements notebook.Cell.
func (c *ResultCell) ID() string { return c.id }

// HeightHint implements notebook.Cell.
func (c *ResultCell) HeightHint(width int) int {
	c.materialize(width)
	return len(c.rendered)
}

// RenderRows implements notebook.Cell.
func (c *ResultCell) RenderRows(width, startRow, endRow int, _ bool, _ notebook.Mode) []string {
	c.materialize(width)
	if startRow < 0 {
		startRow = 0
	}
	if endRow > len(c.rendered) {
		endRow = len(c.rendered)
	}
	if startRow >= endRow {
		return nil
	}
	out := make([]string, endRow-startRow)
	copy(out, c.rendered[startRow:endRow])
	return out
}

// Update implements notebook.Cell. Read-only; Esc in ViewMode
// releases focus by convention. Other keys passthrough.
func (c *ResultCell) Update(msg tea.Msg, mode notebook.Mode) (notebook.Cell, tea.Cmd, bool) {
	if k, ok := msg.(tea.KeyMsg); ok && k.String() == "esc" && mode == notebook.ViewMode {
		return c, notebook.ReleaseFocus, true
	}
	return c, nil, false
}

// StatusHint implements notebook.Cell.
func (c *ResultCell) StatusHint(notebook.Mode) string { return "" }

func (c *ResultCell) materialize(width int) {
	if c.rendered != nil {
		return
	}
	border := lipgloss.Color("#888888")
	if c.errMsg != "" {
		border = lipgloss.Color("#FF4444")
	}
	exprStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#CCCCCC"))
	valStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FAFAFA"))
	errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF4444"))

	var line string
	if c.errMsg != "" {
		line = exprStyle.Render(c.expr) + " → " + errStyle.Render(c.errMsg)
	} else {
		line = exprStyle.Render(c.expr) + " = " + valStyle.Render(c.value)
	}
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Padding(0, 1).
		Width(boxInnerWidth(width)).
		Render(line)
	c.rendered = strings.Split(box, "\n")
}

// boxInnerWidth returns the inner content width for a lipgloss
// padded+bordered box at the given outer width. Mirrors what the
// built-in cells do internally.
func boxInnerWidth(outer int) int {
	w := outer - 4
	if w < 10 {
		w = 10
	}
	return w
}
