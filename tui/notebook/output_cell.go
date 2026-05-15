package notebook

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/panyam/demokit"
)

// OutputCell renders a step's captured stdout. The cell's height is
// capped by maxBody rows; when content exceeds the cap the focused
// cell exposes j/k (and PgUp/PgDn) to scroll within its own
// viewport. ↑/↓ at the model level still navigates between cells —
// the two key vocabularies are intentionally distinct.
//
// Content is read from an OutputBuffer on every RenderRows so live
// streaming Just Works — the cell holds a pointer, not a snapshot.
// 'c' copies the entire current buffer via demokit.Copy.
type OutputCell struct {
	id     string
	buf    *OutputBuffer
	maxBody int // soft cap on visible-body rows (default 12)

	scrollOffset int // first body line shown
	copyMsg      string

	// done indicates the step has finished streaming. Renders a
	// trailing "(end)" hint so the user can tell live vs. complete.
	done bool
}

// NewOutputCell builds a cell over the given buffer. maxBody == 0
// uses the package default (12). The buffer's wakeups should be
// forwarded to the Bubble Tea program by the renderer bridge (PR2);
// the cell itself is content-pull and doesn't subscribe.
func NewOutputCell(id string, buf *OutputBuffer, maxBody int) *OutputCell {
	if maxBody <= 0 {
		maxBody = 12
	}
	return &OutputCell{id: id, buf: buf, maxBody: maxBody}
}

// MarkDone is called by the renderer bridge when the underlying
// step's Run returns. Adds the "(end)" hint and stops the cell
// from drawing "..." for in-progress streams.
func (c *OutputCell) MarkDone() { c.done = true }

// ID implements Cell.
func (c *OutputCell) ID() string { return c.id }

// HeightHint implements Cell. Two parts:
//
//   - Header: blank + "» output" + blank = 3 rows.
//   - Body: min(buf.LineCount(), maxBody) + trailing status row.
//
// The cell does not grow indefinitely — once buf.LineCount() exceeds
// maxBody the height is fixed and the user scrolls in-cell.
func (c *OutputCell) HeightHint(_ int) int {
	bodyRows := c.buf.LineCount()
	if bodyRows > c.maxBody {
		bodyRows = c.maxBody
	}
	h := 3 + bodyRows + 1 // header(3) + body + status(1)
	if c.copyMsg != "" {
		h++
	}
	return h
}

// RenderRows implements Cell.
func (c *OutputCell) RenderRows(width, startRow, endRow int, focused bool, mode Mode) []string {
	c.clampScroll()

	totalLines := c.buf.LineCount()
	bodyRows := totalLines
	if bodyRows > c.maxBody {
		bodyRows = c.maxBody
	}

	// Slice the visible body window from the buffer.
	bodyStart := c.scrollOffset
	bodyEnd := bodyStart + bodyRows
	if bodyEnd > totalLines {
		bodyEnd = totalLines
	}
	body := c.buf.Lines(bodyStart, bodyEnd)

	// Compose the full row list, then clip to [startRow, endRow).
	var rows []string
	rows = append(rows, "")
	header := "» output"
	if focused {
		header = "▶ output"
	}
	rows = append(rows, header)
	rows = append(rows, "")
	for _, line := range body {
		line = strings.TrimRight(line, "\r")
		if w := width - 4; w > 0 && len(line) > w {
			line = line[:w]
		}
		rows = append(rows, "    "+line)
	}
	rows = append(rows, "    "+c.statusLine(totalLines))
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

// Update implements Cell. Handles j/k/pgup/pgdn scroll and `c` copy
// when in view mode. Also accepts clearCopyMsg routed back from
// tea.Tick.
func (c *OutputCell) Update(msg tea.Msg, mode Mode) (Cell, tea.Cmd) {
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
	switch keyMsg.String() {
	case "j", "down":
		c.scrollOffset++
		c.clampScroll()
	case "k", "up":
		c.scrollOffset--
		c.clampScroll()
	case "pgdown":
		c.scrollOffset += c.maxBody
		c.clampScroll()
	case "pgup":
		c.scrollOffset -= c.maxBody
		c.clampScroll()
	case "g":
		c.scrollOffset = 0
	case "G":
		c.scrollOffset = c.buf.LineCount() // clamp will pull back to last page
		c.clampScroll()
	case "c":
		all := strings.Join(c.buf.AllLines(), "\n")
		strategy, ok := demokit.Copy(all)
		if ok {
			c.copyMsg = fmt.Sprintf("(copied %d lines via %s)", c.buf.LineCount(), strategy)
		} else {
			c.copyMsg = "(copy failed — no clipboard provider)"
		}
		return c, clearCopyMsgAfter(c.id)
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

// clampScroll pins scrollOffset to a valid range: 0 ≤ off ≤
// max(0, totalLines − maxBody). Cheap; called on every nav and
// before each render to defend against width-shrink desync.
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

// statusLine builds the right-aligned "[ x/y, running… ]" indicator
// shown under the body. Distinguishes streaming-in-progress from
// completed runs.
func (c *OutputCell) statusLine(total int) string {
	end := c.scrollOffset + c.maxBody
	if end > total {
		end = total
	}
	state := "live"
	if c.done {
		state = "end"
	}
	if total <= c.maxBody {
		return fmt.Sprintf("[ %d line(s) · %s ]", total, state)
	}
	return fmt.Sprintf("[ %d-%d / %d · %s ]", c.scrollOffset+1, end, total, state)
}
