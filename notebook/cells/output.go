package cells

import (
	"fmt"
	"image/color"
	"strings"
	"sync/atomic"

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

	// Edges controls which sides of the box draw a border line.
	// Default Dark/LightOutputStyle use HorizontalEdges (top +
	// bottom only) so users can drag-select the output body
	// without picking up vertical bar characters.
	Edges BorderEdges
	// Border overrides the lipgloss border shape (which glyphs
	// the four sides + corners use). Zero value falls back to the
	// focus-based default (RoundedBorder unfocused, ThickBorder
	// focused). When set, focus is signaled via BorderColor only.
	Border lipgloss.Border
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
		Edges:            HorizontalEdges(),
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
		Edges:            HorizontalEdges(),
	}
}

// DefaultOutputStyle returns the package default — Dark.
func DefaultOutputStyle() OutputStyle { return DarkOutputStyle() }

// OutputCell renders a streaming text buffer in a bordered box,
// capped at maxBody rows. While following (the default), new lines
// keep the bottom of the buffer visible — like `tail -f`. Manual
// scroll (j/k/g/pgup/pgdown) turns follow off; G turns it back on.
// Apps that want every line visible (no in-cell scroll) pass a
// maxBody >= the known final line count via NewOutput or
// SetMaxBody; the notebook's viewport handles scroll between
// cells when the cell outgrows the screen.
// 'c' copies the whole buffer via the injected Clipboard; 't'
// invokes the optional FallbackClipboard with the same payload —
// useful when the primary clipboard write was silently suppressed
// (e.g. iTerm blocking OSC 52).
//
// 'c' and 't' are OutputCell conventions, not notebook framework
// rules — custom cells handle their own copy UX. When no fallback
// is configured the 't' hint is omitted and 't' passes through.
//
// Content is read live from the OutputBuffer on every render, so a
// caller streaming into the buffer needs only to trigger a repaint.
type OutputCell struct {
	Style OutputStyle

	id           string
	buf          *notebook.OutputBuffer // storage + streaming sink (Stream/Buffer)
	layout       notebook.LineSource    // width→visual-rows view over buf; swappable impl
	maxBody      int
	clip         notebook.Clipboard
	fallbackClip notebook.Clipboard
	scrollOffset int
	follow       bool
	// done is set by MarkDone() from the streaming goroutine and
	// read by RenderRows() from the bubbletea View goroutine. Use
	// atomic so the two-write/read pair stays race-free.
	done    atomic.Bool
	copyMsg string
	lastCopy     string // payload retained after 'c' so 't' can replay it
	lastWidth    int    // body width from the last render; scopes key/wheel scroll math to visual rows
}

// NewOutput builds an OutputCell over a fresh OutputBuffer.
// maxBody <= 0 uses the package default (12). Follow mode starts
// on; clipboard defaults to notebook.NoClipboard.
func NewOutput(id string, maxBody int) *OutputCell {
	if maxBody <= 0 {
		maxBody = defaultMaxBody
	}
	buf := notebook.NewOutputBuffer()
	return &OutputCell{
		id:      id,
		buf:     buf,
		layout:  notebook.NewEagerLineSource(buf),
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

// SetFallbackClipboard wires the clipboard the cell uses on 't' as
// a backup after 'c'. Pass nil to disable — 't' then passes through.
// Typically wired to notebook.FileClipboard("") when the primary is
// OSC 52 so the user can recover from terminals that suppress the
// escape silently.
func (c *OutputCell) SetFallbackClipboard(clip notebook.Clipboard) {
	c.fallbackClip = clip
}

// MarkDone flips the box state accent from "·live" to "·end".
// Safe to call from any goroutine; the View goroutine reads the
// flag atomically.
func (c *OutputCell) MarkDone() { c.done.Store(true) }

// MaxBody returns the current row cap for the cell's rendered
// body. Useful for resize key bindings ("+"/"-" actions in the
// notebook's KeyMap that grow/shrink the focused output cell).
func (c *OutputCell) MaxBody() int { return c.maxBody }

// SetMaxBody updates the row cap. Non-positive values are
// rejected (a zero-row body would be a degenerate render).
// Clamps scrollOffset against the new ceiling so follow-mode +
// manual scroll positions stay valid.
func (c *OutputCell) SetMaxBody(n int) {
	if n <= 0 {
		return
	}
	c.maxBody = n
	c.clampScroll(c.rowCount())
}

// ID implements notebook.Cell.
func (c *OutputCell) ID() string { return c.id }

// bodyWidth is the usable text-column count inside the box at the
// given outer width. innerWidth nets out the border budget and one
// Padding(0,1); lipgloss's Width(inner) reserves that padding again
// inside the box, so the real content area is two columns narrower.
// Wrapping to anything wider lets lipgloss re-wrap and desync the
// row count against HeightHint.
func (c *OutputCell) bodyWidth(width int) int {
	return max(innerWidth(width, c.Style.Edges)-2, 1)
}

// rowCount reports the total visual-row count at the last rendered
// width. Before the first render (lastWidth == 0) it falls back to
// the logical line count — a safe proxy for the "does the buffer
// overflow maxBody?" checks until a real width is known.
func (c *OutputCell) rowCount() int {
	if c.lastWidth <= 0 {
		return c.buf.LineCount()
	}
	return c.layout.RowCount(c.lastWidth)
}

// HeightHint implements notebook.Cell. Box = (top border?) +
// title + body + status + (bottom border?). An empty buffer
// still reserves one body row for the "(no output yet)"
// placeholder. Chrome shrinks when Style.Edges turns sides off.
func (c *OutputCell) HeightHint(width int) int {
	bodyRows := c.layout.RowCount(c.bodyWidth(width))
	if bodyRows == 0 {
		bodyRows = 1
	}
	if bodyRows > c.maxBody {
		bodyRows = c.maxBody
	}
	h := bodyRows + 2 + chromeRows(c.Style.Edges) // 2 = title + status
	if c.copyMsg != "" {
		h++
	}
	return h
}

// RenderRows implements notebook.Cell.
func (c *OutputCell) RenderRows(width, startRow, endRow int, focused bool, _ notebook.Mode) []string {
	inner := innerWidth(width, c.Style.Edges)
	c.lastWidth = c.bodyWidth(width)
	totalRows := c.layout.RowCount(c.lastWidth)
	c.clampScroll(totalRows)

	border := c.Style.BorderColor
	if focused {
		border = c.Style.FocusBorderColor
	}

	bodyRows := min(totalRows, c.maxBody)
	body := c.layout.Rows(c.lastWidth, c.scrollOffset, c.scrollOffset+bodyRows)

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(c.Style.TitleColor)
	dim := lipgloss.NewStyle().Foreground(c.Style.DimColor)
	state := "live"
	stateStyle := lipgloss.NewStyle().Foreground(c.Style.LiveColor)
	if c.done.Load() {
		state = "end"
		stateStyle = lipgloss.NewStyle().Foreground(c.Style.DoneColor)
	}

	title := titleStyle.Render("output") + " " + stateStyle.Render("·"+state)
	bodyText := strings.Join(body, "\n")
	if bodyText == "" {
		bodyText = dim.Render("(no output yet)")
	}
	status := dim.Render(c.statusLine(totalRows))
	content := title + "\n" + bodyText + "\n" + status

	boxStyle := lipgloss.NewStyle().
		Border(borderFor(c.Style.Border, focused)).
		BorderForeground(border).
		BorderTop(c.Style.Edges.Top).
		BorderRight(c.Style.Edges.Right).
		BorderBottom(c.Style.Edges.Bottom).
		BorderLeft(c.Style.Edges.Left).
		Padding(0, 1).
		Width(inner)
	rendered := boxStyle.Render(content)
	boxRows := strings.Split(rendered, "\n")
	if c.copyMsg != "" {
		boxRows = append(boxRows, "  "+c.copyMsg)
	}

	if startRow < 0 {
		startRow = 0
	}
	if endRow > len(boxRows) {
		endRow = len(boxRows)
	}
	if startRow >= endRow {
		return nil
	}
	out := make([]string, endRow-startRow)
	copy(out, boxRows[startRow:endRow])
	return out
}

// Update implements notebook.Cell. 'c' copies regardless of mode;
// in CellActiveMode scroll keys (j/k/g/G/pgup/pgdown) move within the
// buffer, Enter advances, Esc releases focus. Other keys
// passthrough.
func (c *OutputCell) Update(msg tea.Msg, mode notebook.Mode) (notebook.Cell, tea.Cmd, bool) {
	if cm, ok := msg.(notebook.ClearCopyMsg); ok && cm.CellID == c.id {
		c.copyMsg = ""
		return c, nil, true
	}
	// Mouse wheel: scroll the body line-by-line when the cell
	// is activated (CellActiveMode) AND the buffer overflows maxBody.
	// In NavigationMode (cell-to-cell nav), the wheel passes through
	// so the notebook moves the cell cursor instead. Click on
	// the cell to activate it.
	if mouse, ok := msg.(tea.MouseMsg); ok {
		if mouse.Action != tea.MouseActionPress {
			return c, nil, false
		}
		if mode != notebook.CellActiveMode {
			return c, nil, false
		}
		if c.rowCount() <= c.maxBody {
			return c, nil, false
		}
		switch mouse.Button {
		case tea.MouseButtonWheelUp:
			c.follow = false
			c.scrollOffset--
			c.clampScroll(c.rowCount())
			return c, nil, true
		case tea.MouseButtonWheelDown:
			c.follow = false
			c.scrollOffset++
			c.clampScroll(c.rowCount())
			return c, nil, true
		}
		return c, nil, false
	}
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return c, nil, false
	}
	if keyMsg.String() == "c" {
		all := strings.Join(c.buf.AllLines(), "\n")
		c.lastCopy = all
		strategy, ok := c.clip(all)
		if ok {
			c.copyMsg = fmt.Sprintf("(copied %d lines via %s)", c.buf.LineCount(), strategy)
			if c.fallbackClip != nil {
				c.copyMsg += " · press t to save tmp file"
			}
		} else {
			c.copyMsg = "(copy failed — no clipboard provider)"
		}
		return c, notebook.ClearCopyAfter(c.id), true
	}
	// 't' invokes the fallback clipboard with the last payload —
	// usable only while the previous copy toast is still up AND
	// a fallback is configured. Otherwise passthrough so the
	// notebook (or another cell) can claim 't' for its own use.
	if keyMsg.String() == "t" {
		if c.fallbackClip == nil || c.copyMsg == "" || c.lastCopy == "" {
			return c, nil, false
		}
		strategy, ok := c.fallbackClip(c.lastCopy)
		if ok {
			c.copyMsg = fmt.Sprintf("(saved %d lines to %s)", c.buf.LineCount(), strategy)
		} else {
			c.copyMsg = "(fallback save failed)"
		}
		return c, notebook.ClearCopyAfter(c.id), true
	}
	if mode != notebook.CellActiveMode {
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
		c.clampScroll(c.rowCount())
		return c, nil, true
	case "k", "up":
		c.follow = false
		c.scrollOffset--
		c.clampScroll(c.rowCount())
		return c, nil, true
	case "pgdown":
		c.follow = false
		c.scrollOffset += c.maxBody
		c.clampScroll(c.rowCount())
		return c, nil, true
	case "pgup":
		c.follow = false
		c.scrollOffset -= c.maxBody
		c.clampScroll(c.rowCount())
		return c, nil, true
	case "g":
		c.follow = false
		c.scrollOffset = 0
		return c, nil, true
	case "G":
		c.follow = true
		c.scrollOffset = c.rowCount()
		c.clampScroll(c.rowCount())
		return c, nil, true
	}
	return c, nil, false
}

// StatusHint implements notebook.Cell.
func (c *OutputCell) StatusHint(_ notebook.Mode) string {
	if c.rowCount() > c.maxBody {
		return "j/k scroll · g/G top/bot · c copy"
	}
	return "c copy"
}

// clampScroll keeps scrollOffset valid for a body of total visual
// rows. When follow is true, it sticks the offset to the end so
// the latest maxBody rows stay visible (the "tail -f" behavior);
// when false, it just clamps the manually-set offset. total is the
// visual-row count (logical lines after wrapping), so follow tails
// the last wrapped row of the in-flight line, not just whole lines.
//
// Called at the top of RenderRows so every frame reflects the
// current buffer state without an external "append happened"
// hook — Stream writes to the buffer, the next render picks it up.
func (c *OutputCell) clampScroll(total int) {
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

// statusLine builds the "[ x-y / total ]" indicator over visual rows.
func (c *OutputCell) statusLine(total int) string {
	end := min(c.scrollOffset+c.maxBody, total)
	if total <= c.maxBody {
		return fmt.Sprintf("[ %d line(s) ]", total)
	}
	return fmt.Sprintf("[ %d-%d / %d ]", c.scrollOffset+1, end, total)
}
