package cells

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/panyam/demokit/notebook"
)

// NoteStyle is NoteCell's per-cell styling. Concrete colors, no
// mode flag — light/dark selection happens at the theme layer.
type NoteStyle struct {
	BorderColor      color.Color
	FocusBorderColor color.Color
	TitleColor       color.Color
	BodyColor        color.Color
	Edges            BorderEdges
}

// DarkNoteStyle returns the dark-terminal defaults.
func DarkNoteStyle() NoteStyle {
	return NoteStyle{
		BorderColor:      lipgloss.Color("#626262"),
		FocusBorderColor: lipgloss.Color("#FF6B6B"),
		TitleColor:       lipgloss.Color("#FAFAFA"),
		BodyColor:        lipgloss.Color("#CCCCCC"),
		Edges:            AllEdges(),
	}
}

// LightNoteStyle returns the light-terminal defaults.
func LightNoteStyle() NoteStyle {
	return NoteStyle{
		BorderColor:      lipgloss.Color("#999999"),
		FocusBorderColor: lipgloss.Color("#D34545"),
		TitleColor:       lipgloss.Color("#1A1A1A"),
		BodyColor:        lipgloss.Color("#444444"),
		Edges:            AllEdges(),
	}
}

// DefaultNoteStyle returns the package default — Dark.
func DefaultNoteStyle() NoteStyle { return DarkNoteStyle() }

// NoteCell renders an italic-titled explanatory block — title
// plus body of wrapped text. 'c' copies the body via the injected
// Clipboard regardless of mode (frictionless copy from navigation).
// Typically used for narrative breaks, hints, sidebars.
type NoteCell struct {
	Title string
	Body  string
	Style NoteStyle

	id            string
	clip          notebook.Clipboard
	fallbackClip  notebook.Clipboard
	cachedWidth   int
	cachedFocused bool
	cachedStyle   NoteStyle
	cachedTitle   string
	cachedBody    string
	cachedLines   []string
	cachedHeight  int
	copyMsg       string
	lastCopy      string
}

// NewNote builds a NoteCell with DefaultNoteStyle. Clipboard
// defaults to notebook.NoClipboard until SetClipboard is called.
func NewNote(id, title, body string) *NoteCell {
	return &NoteCell{
		id:    id,
		Title: title,
		Body:  body,
		Style: DefaultNoteStyle(),
		clip:  notebook.NoClipboard,
	}
}

// SetClipboard wires the clipboard the cell uses on 'c'. Pass nil
// to disable copy (falls back to notebook.NoClipboard).
func (c *NoteCell) SetClipboard(clip notebook.Clipboard) {
	if clip == nil {
		clip = notebook.NoClipboard
	}
	c.clip = clip
}

// SetFallbackClipboard wires the clipboard the cell uses on 't' as
// a backup after 'c'. Pass nil to disable — 't' then passes through.
// App-level wiring (the notebook ships no auto-injection).
func (c *NoteCell) SetFallbackClipboard(clip notebook.Clipboard) {
	c.fallbackClip = clip
}

// ID implements notebook.Cell.
func (c *NoteCell) ID() string { return c.id }

// HeightHint implements notebook.Cell.
func (c *NoteCell) HeightHint(width int) int {
	c.materialize(width, c.cachedFocused)
	h := c.cachedHeight
	if c.copyMsg != "" {
		h++
	}
	return h
}

// RenderRows implements notebook.Cell.
func (c *NoteCell) RenderRows(width, startRow, endRow int, focused bool, _ notebook.Mode) []string {
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

// Update implements notebook.Cell. 'c' copies regardless of mode;
// Enter in CellActiveMode signals cell-advance; Esc in CellActiveMode releases
// focus. Other keys passthrough (handled=false) so notebook KeyMap
// gets a turn.
func (c *NoteCell) Update(msg tea.Msg, mode notebook.Mode) (notebook.Cell, tea.Cmd, bool) {
	if cm, ok := msg.(notebook.ClearCopyMsg); ok && cm.CellID == c.id {
		c.copyMsg = ""
		return c, nil, true
	}
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return c, nil, false
	}
	if keyMsg.String() == "c" {
		c.lastCopy = c.Body
		strategy, ok := c.clip(c.Body)
		if ok {
			c.copyMsg = "(copied via " + strategy + ")"
			if c.fallbackClip != nil {
				c.copyMsg += " · press t to save tmp file"
			}
		} else {
			c.copyMsg = "(copy failed — no clipboard provider)"
		}
		return c, notebook.ClearCopyAfter(c.id), true
	}
	if keyMsg.String() == "t" {
		if c.fallbackClip == nil || c.copyMsg == "" || c.lastCopy == "" {
			return c, nil, false
		}
		strategy, ok := c.fallbackClip(c.lastCopy)
		if ok {
			c.copyMsg = "(saved to " + strategy + ")"
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
	}
	return c, nil, false
}

// StatusHint implements notebook.Cell.
func (c *NoteCell) StatusHint(_ notebook.Mode) string { return "c copy" }

func (c *NoteCell) materialize(width int, focused bool) {
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

	titleStyle := lipgloss.NewStyle().Italic(true).Foreground(c.Style.TitleColor)
	bodyStyle := lipgloss.NewStyle().Foreground(c.Style.BodyColor)
	content := titleStyle.Render(c.Title) + "\n\n" + bodyStyle.Render(c.Body)

	boxStyle := lipgloss.NewStyle().
		Border(focusedBorder(focused)).
		BorderForeground(border).
		BorderTop(c.Style.Edges.Top).
		BorderRight(c.Style.Edges.Right).
		BorderBottom(c.Style.Edges.Bottom).
		BorderLeft(c.Style.Edges.Left).
		Padding(0, 1).
		Width(innerWidth(width, c.Style.Edges))

	rendered := boxStyle.Render(content)
	c.cachedWidth = width
	c.cachedFocused = focused
	c.cachedStyle = c.Style
	c.cachedTitle = c.Title
	c.cachedBody = c.Body
	c.cachedLines = strings.Split(rendered, "\n")
	c.cachedHeight = len(c.cachedLines)
}
