package notebook

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/panyam/demokit"
)

// SectionCell renders a Note(...) / explanatory block. Stand-alone
// `c` in focused mode copies the body via demokit.Copy so a user
// can grab a chunk of context without having to mouse-select.
//
// Read-only otherwise; future EditMode would expose body editing.
type SectionCell struct {
	id    string
	title string
	body  string

	cachedWidth  int
	cachedLines  []string
	cachedHeight int

	// copyMsg holds the transient status line shown right under the
	// section body for a few ticks after `c`. The model drives the
	// fade via the clearCopyMsg tea.Cmd returned from Update.
	copyMsg string
}

// NewSectionCell builds a section cell.
func NewSectionCell(id, title, body string) *SectionCell {
	return &SectionCell{id: id, title: title, body: body}
}

// ID implements Cell.
func (c *SectionCell) ID() string { return c.id }

// HeightHint implements Cell.
func (c *SectionCell) HeightHint(width int) int {
	c.materialize(width)
	h := c.cachedHeight
	if c.copyMsg != "" {
		h++
	}
	return h
}

// RenderRows implements Cell.
func (c *SectionCell) RenderRows(width, startRow, endRow int, focused bool, mode Mode) []string {
	c.materialize(width)
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
		var line string
		switch {
		case i < c.cachedHeight:
			line = c.cachedLines[i]
		default:
			line = "  " + c.copyMsg
		}
		rows[i-startRow] = applyFocusMarker(line, focused)
	}
	return rows
}

// Update implements Cell. In focused/view mode, `c` copies the body.
func (c *SectionCell) Update(msg tea.Msg, mode Mode) (Cell, tea.Cmd) {
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
	if _, isClear := msg.(clearCopyMsg); isClear {
		c.copyMsg = ""
		return c, nil
	}
	return c, nil
}

// StatusHint implements Cell.
func (c *SectionCell) StatusHint(_ Mode) string { return "c copy" }

func (c *SectionCell) materialize(width int) {
	if width <= 0 {
		width = 80
	}
	if c.cachedWidth == width && c.cachedLines != nil {
		return
	}
	var rows []string
	rows = append(rows, "")
	rows = append(rows, "§ "+c.title)
	rows = append(rows, "")
	for _, line := range wrapPlain(c.body, width-2) {
		rows = append(rows, "  "+line)
	}
	rows = append(rows, "")
	c.cachedWidth = width
	c.cachedLines = rows
	c.cachedHeight = len(rows)
}
