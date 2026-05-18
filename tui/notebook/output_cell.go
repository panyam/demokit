package notebook

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/panyam/demokit"
)

// OutputCell renders a step's captured stdout in a bordered box.
// The cell's box height is capped by maxBody rows; when content
// exceeds the cap the focused cell exposes j/k (and PgUp/PgDn,
// g/G) to scroll within the box. ↑/↓ at the model level still
// navigates between cells.
//
// Content is read from an OutputBuffer on every RenderRows so live
// streaming Just Works. 'c' copies the entire current buffer.
type OutputCell struct {
	id      string
	buf     *OutputBuffer
	maxBody int
	palette Palette

	scrollOffset int
	copyMsg      string
	done         bool
	// follow is the "tail -f" sticky-bottom mode. New chunks bump
	// scrollOffset so the last maxBody lines are visible. Manual
	// scrolling (j/k/g/pgup/pgdown) turns it off; G turns it back
	// on. Defaults to true on construction.
	follow bool
}

// NewOutputCell builds a cell over the given buffer. maxBody == 0
// uses the package default (12).
func NewOutputCell(id string, buf *OutputBuffer, maxBody int) *OutputCell {
	if maxBody <= 0 {
		maxBody = 12
	}
	return &OutputCell{
		id: id, buf: buf, maxBody: maxBody,
		palette: DefaultPalette(),
		follow:  true,
	}
}

// OnAppend is called by the model after a chunk lands in the cell's
// buffer. While follow is true, advances scrollOffset so the last
// maxBody lines are visible.
func (c *OutputCell) OnAppend() {
	if !c.follow {
		return
	}
	c.scrollOffset = c.buf.LineCount() - c.maxBody
	c.clampScroll()
}

// SetPalette overrides the cell's palette.
func (c *OutputCell) SetPalette(p Palette) { c.palette = p }

// MarkDone flips the box title from "» output (live)" to "(end)".
func (c *OutputCell) MarkDone() { c.done = true }

// ID implements Cell.
func (c *OutputCell) ID() string { return c.id }

// HeightHint implements Cell. Rendered box = top border + title +
// body + status + bottom border. The empty-buffer placeholder
// "(no output yet)" still takes one body row.
func (c *OutputCell) HeightHint(_ int) int {
	bodyRows := c.buf.LineCount()
	if bodyRows == 0 {
		bodyRows = 1
	}
	if bodyRows > c.maxBody {
		bodyRows = c.maxBody
	}
	h := bodyRows + 4
	if c.copyMsg != "" {
		h++
	}
	return h
}

// RenderRows implements Cell.
func (c *OutputCell) RenderRows(width, startRow, endRow int, focused bool, _ Mode) []string {
	c.clampScroll()

	border := c.palette.OutputBorder
	if focused {
		border = c.palette.FocusBorder
	}

	totalLines := c.buf.LineCount()
	bodyRows := totalLines
	if bodyRows > c.maxBody {
		bodyRows = c.maxBody
	}
	body := c.buf.Lines(c.scrollOffset, c.scrollOffset+bodyRows)
	// Hard-truncate long lines so lipgloss doesn't wrap them: a
	// wrap would silently push the box past HeightHint's
	// reported height and desync the viewport's row math.
	innerWidth := maxBoxWidth(width)
	for i, line := range body {
		line = strings.TrimRight(line, "\r")
		if len(line) > innerWidth {
			line = line[:innerWidth]
		}
		body[i] = line
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(c.palette.Title)
	dim := lipgloss.NewStyle().Foreground(c.palette.Dim)
	state := "live"
	stateStyle := lipgloss.NewStyle().Foreground(c.palette.Header)
	if c.done {
		state = "end"
		stateStyle = lipgloss.NewStyle().Foreground(c.palette.Success)
	}

	title := titleStyle.Render("output") + " " + stateStyle.Render("·"+state)
	bodyText := strings.Join(body, "\n")
	if bodyText == "" {
		bodyText = dim.Render("(no output yet)")
	}
	status := dim.Render(c.statusLine(totalLines))
	content := title + "\n" + bodyText + "\n" + status

	boxStyle := lipgloss.NewStyle().
		Border(focusedBorder(focused)).
		BorderForeground(border).
		Padding(0, 1).
		Width(maxBoxWidth(width))
	rendered := boxStyle.Render(content)
	rows := strings.Split(rendered, "\n")
	if c.copyMsg != "" {
		rows = append(rows, "  "+c.copyMsg)
	}

	if startRow < 0 {
		startRow = 0
	}
	if endRow > len(rows) {
		endRow = len(rows)
	}
	if startRow >= endRow {
		return nil
	}
	out := make([]string, endRow-startRow)
	copy(out, rows[startRow:endRow])
	return out
}

// Update implements Cell.
func (c *OutputCell) Update(msg tea.Msg, mode Mode) (Cell, tea.Cmd) {
	if cm, ok := msg.(clearCopyMsg); ok && cm.cellID == c.id {
		c.copyMsg = ""
		return c, nil
	}
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return c, nil
	}
	// 'c' is processed regardless of mode — copying is a
	// frictionless action available while just navigating
	// between cells.
	if keyMsg.String() == "c" {
		all := strings.Join(c.buf.AllLines(), "\n")
		strategy, ok := demokit.Copy(all)
		if ok {
			c.copyMsg = fmt.Sprintf("(copied %d lines via %s)", c.buf.LineCount(), strategy)
		} else {
			c.copyMsg = "(copy failed — no clipboard provider)"
		}
		return c, clearCopyMsgAfter(c.id)
	}
	if mode != ViewMode {
		return c, nil
	}
	switch keyMsg.String() {
	case "enter":
		// Cell doesn't use Enter — signal release + advance.
		return c, cellAdvance
	case "j", "down":
		c.follow = false
		c.scrollOffset++
		c.clampScroll()
	case "k", "up":
		c.follow = false
		c.scrollOffset--
		c.clampScroll()
	case "pgdown":
		c.follow = false
		c.scrollOffset += c.maxBody
		c.clampScroll()
	case "pgup":
		c.follow = false
		c.scrollOffset -= c.maxBody
		c.clampScroll()
	case "g":
		c.follow = false
		c.scrollOffset = 0
	case "G":
		c.follow = true
		c.scrollOffset = c.buf.LineCount()
		c.clampScroll()
	}
	return c, nil
}

// StatusHint implements Cell.
func (c *OutputCell) StatusHint(_ Mode) string {
	if c.buf.LineCount() > c.maxBody {
		return "j/k scroll · g/G top/bot · c copy"
	}
	return "c copy"
}

// clampScroll pins scrollOffset to [0, max(0, total-maxBody)].
func (c *OutputCell) clampScroll() {
	total := c.buf.LineCount()
	max := total - c.maxBody
	if max < 0 {
		max = 0
	}
	if c.scrollOffset > max {
		c.scrollOffset = max
	}
	if c.scrollOffset < 0 {
		c.scrollOffset = 0
	}
}

// statusLine builds the "[ x-y / total ]" indicator.
func (c *OutputCell) statusLine(total int) string {
	end := c.scrollOffset + c.maxBody
	if end > total {
		end = total
	}
	if total <= c.maxBody {
		return fmt.Sprintf("[ %d line(s) ]", total)
	}
	return fmt.Sprintf("[ %d-%d / %d ]", c.scrollOffset+1, end, total)
}
