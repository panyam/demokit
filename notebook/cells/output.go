package cells

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/panyam/demokit/notebook"
)

// defaultMaxBody is the OutputCell body row cap used when NewOutput
// is given maxBody <= 0.
const defaultMaxBody = 12

// OutputStyle is OutputCell's per-cell styling.
type OutputStyle struct {
	BorderColor      color.Color
	FocusBorderColor color.Color
	TitleColor       color.Color
	DimColor         color.Color
	LiveColor        color.Color // "·live" state accent
	DoneColor        color.Color // "·end" state accent
}

// DarkOutputStyle returns the dark-terminal defaults.
func DarkOutputStyle() OutputStyle {
	return OutputStyle{
		BorderColor:      lipgloss.Color("#04B575"),
		FocusBorderColor: lipgloss.Color("#FF6B6B"),
		TitleColor:       lipgloss.Color("#FAFAFA"),
		DimColor:         lipgloss.Color("#888888"),
		LiveColor:        lipgloss.Color("#FF6B6B"),
		DoneColor:        lipgloss.Color("#04B575"),
	}
}

// LightOutputStyle returns the light-terminal defaults.
func LightOutputStyle() OutputStyle {
	return OutputStyle{
		BorderColor:      lipgloss.Color("#03935F"),
		FocusBorderColor: lipgloss.Color("#D34545"),
		TitleColor:       lipgloss.Color("#1A1A1A"),
		DimColor:         lipgloss.Color("#777777"),
		LiveColor:        lipgloss.Color("#D34545"),
		DoneColor:        lipgloss.Color("#03935F"),
	}
}

// DefaultOutputStyle returns the package default — Dark.
func DefaultOutputStyle() OutputStyle { return DarkOutputStyle() }

// OutputCell renders a streaming text buffer in a bordered box,
// capped at maxBody rows. While following (the default), new lines
// keep the bottom of the buffer visible — like `tail -f`. Manual
// scroll (j/k/g/pgup/pgdown) turns follow off; G turns it back on.
// 'c' copies the whole buffer via the injected Clipboard.
//
// Content is read live from the OutputBuffer on every render, so a
// caller streaming into the buffer needs only to trigger a repaint.
type OutputCell struct {
	Style OutputStyle

	id           string
	buf          *notebook.OutputBuffer
	maxBody      int
	clip         notebook.Clipboard
	scrollOffset int
	follow       bool
	done         bool
	copyMsg      string
}

// NewOutput builds an OutputCell over a fresh OutputBuffer.
// maxBody <= 0 uses the package default (12). Follow mode starts
// on; clipboard defaults to notebook.NoClipboard.
func NewOutput(id string, maxBody int) *OutputCell {
	if maxBody <= 0 {
		maxBody = defaultMaxBody
	}
	return &OutputCell{
		id:      id,
		buf:     notebook.NewOutputBuffer(),
		maxBody: maxBody,
		Style:   DefaultOutputStyle(),
		clip:    notebook.NoClipboard,
		follow:  true,
	}
}

// Buffer returns the cell's underlying OutputBuffer. The notebook
// runtime hands this out via Stream(id) so callers can write
// streaming output directly.
func (c *OutputCell) Buffer() *notebook.OutputBuffer { return c.buf }

// SetClipboard wires the clipboard the cell uses on 'c'.
func (c *OutputCell) SetClipboard(clip notebook.Clipboard) {
	if clip == nil {
		clip = notebook.NoClipboard
	}
	c.clip = clip
}

// MarkDone flips the box state accent from "·live" to "·end".
func (c *OutputCell) MarkDone() { c.done = true }

// ID implements notebook.Cell.
func (c *OutputCell) ID() string { return c.id }

// HeightHint implements notebook.Cell. Box = top border + title +
// body + status + bottom border. An empty buffer still reserves
// one body row for the "(no output yet)" placeholder.
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

// RenderRows implements notebook.Cell.
func (c *OutputCell) RenderRows(width, startRow, endRow int, focused bool, _ notebook.Mode) []string {
	c.clampScroll()

	border := c.Style.BorderColor
	if focused {
		border = c.Style.FocusBorderColor
	}

	totalLines := c.buf.LineCount()
	bodyRows := min(totalLines, c.maxBody)
	body := c.buf.Lines(c.scrollOffset, c.scrollOffset+bodyRows)
	// Hard-truncate long lines so lipgloss doesn't wrap them: a
	// wrap would push the box past HeightHint and desync viewport
	// row math.
	innerWidth := maxBoxWidth(width)
	for i, line := range body {
		line = strings.TrimRight(line, "\r")
		if len(line) > innerWidth {
			line = line[:innerWidth]
		}
		body[i] = line
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(c.Style.TitleColor)
	dim := lipgloss.NewStyle().Foreground(c.Style.DimColor)
	state := "live"
	stateStyle := lipgloss.NewStyle().Foreground(c.Style.LiveColor)
	if c.done {
		state = "end"
		stateStyle = lipgloss.NewStyle().Foreground(c.Style.DoneColor)
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

// Update implements notebook.Cell. 'c' copies regardless of mode;
// in ViewMode scroll keys (j/k/g/G/pgup/pgdown) move within the
// buffer, Enter advances, Esc releases focus. Other keys
// passthrough.
func (c *OutputCell) Update(msg tea.Msg, mode notebook.Mode) (notebook.Cell, tea.Cmd, bool) {
	if cm, ok := msg.(notebook.ClearCopyMsg); ok && cm.CellID == c.id {
		c.copyMsg = ""
		return c, nil, true
	}
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return c, nil, false
	}
	if keyMsg.String() == "c" {
		all := strings.Join(c.buf.AllLines(), "\n")
		strategy, ok := c.clip(all)
		if ok {
			c.copyMsg = fmt.Sprintf("(copied %d lines via %s)", c.buf.LineCount(), strategy)
		} else {
			c.copyMsg = "(copy failed — no clipboard provider)"
		}
		return c, notebook.ClearCopyAfter(c.id), true
	}
	if mode != notebook.ViewMode {
		return c, nil, false
	}
	switch keyMsg.String() {
	case "enter":
		return c, notebook.CellAdvance, true
	case "esc":
		return c, notebook.ReleaseFocus, true
	case "j", "down":
		c.follow = false
		c.scrollOffset++
		c.clampScroll()
		return c, nil, true
	case "k", "up":
		c.follow = false
		c.scrollOffset--
		c.clampScroll()
		return c, nil, true
	case "pgdown":
		c.follow = false
		c.scrollOffset += c.maxBody
		c.clampScroll()
		return c, nil, true
	case "pgup":
		c.follow = false
		c.scrollOffset -= c.maxBody
		c.clampScroll()
		return c, nil, true
	case "g":
		c.follow = false
		c.scrollOffset = 0
		return c, nil, true
	case "G":
		c.follow = true
		c.scrollOffset = c.buf.LineCount()
		c.clampScroll()
		return c, nil, true
	}
	return c, nil, false
}

// StatusHint implements notebook.Cell.
func (c *OutputCell) StatusHint(_ notebook.Mode) string {
	if c.buf.LineCount() > c.maxBody {
		return "j/k scroll · g/G top/bot · c copy"
	}
	return "c copy"
}

// clampScroll updates scrollOffset for the current LineCount. When
// follow is true, it sticks scrollOffset to the end of the buffer
// so the latest maxBody lines stay visible (the "tail -f" behavior).
// When follow is false, it just clamps the manually-set offset.
//
// Called at the top of RenderRows so every frame reflects the
// current buffer state without an external "append happened"
// hook — Stream writes to the buffer, the next render picks it up.
func (c *OutputCell) clampScroll() {
	total := c.buf.LineCount()
	if c.follow {
		c.scrollOffset = total - c.maxBody
	}
	hi := max(total-c.maxBody, 0)
	if c.scrollOffset > hi {
		c.scrollOffset = hi
	}
	if c.scrollOffset < 0 {
		c.scrollOffset = 0
	}
}

// statusLine builds the "[ x-y / total ]" indicator.
func (c *OutputCell) statusLine(total int) string {
	end := min(c.scrollOffset+c.maxBody, total)
	if total <= c.maxBody {
		return fmt.Sprintf("[ %d line(s) ]", total)
	}
	return fmt.Sprintf("[ %d-%d / %d ]", c.scrollOffset+1, end, total)
}
