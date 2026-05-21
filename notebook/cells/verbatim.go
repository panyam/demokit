package cells

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/panyam/demokit/notebook"
)

// Variant is one labelled flavor of a VerbatimCell's content —
// e.g. the same command rendered as `curl`, `python`, and `go`.
// IsDefault marks the variant the cell highlights on construction
// (first one wins if multiple are flagged). Lang is a
// syntax-highlighting hint for future chroma adoption.
type Variant struct {
	Label     string
	Lang      string
	Content   string
	IsDefault bool
}

// VerbatimStyle is VerbatimCell's per-cell styling.
type VerbatimStyle struct {
	BorderColor      color.Color
	FocusBorderColor color.Color
	LabelColor       color.Color
	ContentColor     color.Color
	ActiveTabColor   color.Color
	InactiveTabColor color.Color
	Edges            BorderEdges
}

// DarkVerbatimStyle returns the dark-terminal defaults.
func DarkVerbatimStyle() VerbatimStyle {
	return VerbatimStyle{
		BorderColor:      lipgloss.Color("#5BB1FF"),
		FocusBorderColor: lipgloss.Color("#FF6B6B"),
		LabelColor:       lipgloss.Color("#FAFAFA"),
		ContentColor:     lipgloss.Color("#CCCCCC"),
		ActiveTabColor:   lipgloss.Color("#FF6B6B"),
		InactiveTabColor: lipgloss.Color("#888888"),
		Edges:            AllEdges(),
	}
}

// LightVerbatimStyle returns the light-terminal defaults.
func LightVerbatimStyle() VerbatimStyle {
	return VerbatimStyle{
		BorderColor:      lipgloss.Color("#2A77D6"),
		FocusBorderColor: lipgloss.Color("#D34545"),
		LabelColor:       lipgloss.Color("#1A1A1A"),
		ContentColor:     lipgloss.Color("#333333"),
		ActiveTabColor:   lipgloss.Color("#D34545"),
		InactiveTabColor: lipgloss.Color("#777777"),
		Edges:            AllEdges(),
	}
}

// DefaultVerbatimStyle returns the package default — Dark.
func DefaultVerbatimStyle() VerbatimStyle { return DarkVerbatimStyle() }

// VerbatimCell renders one verbatim block — possibly with multiple
// variants shown as a tab strip + a single active variant's body.
//
//   - Tab / Shift+Tab cycle the active variant (wrap)
//   - '1'..'9' jump directly (out-of-range ignored)
//   - 'c' copies the active variant's Content via the injected Clipboard
//
// Single-variant blocks omit the tab strip but still respect 'c'.
type VerbatimCell struct {
	Label    string
	Variants []Variant
	Active   int
	Style    VerbatimStyle

	id            string
	clip          notebook.Clipboard
	cachedWidth   int
	cachedFocused bool
	cachedActive  int
	cachedStyle   VerbatimStyle
	cachedLabel   string
	cachedLines   []string
	cachedHeight  int
	copyMsg       string
}

// NewVerbatim builds a VerbatimCell with DefaultVerbatimStyle.
// Active variant defaults to whichever carries IsDefault (first
// wins), or 0 if none does. Clipboard defaults to
// notebook.NoClipboard.
func NewVerbatim(id, label string, variants []Variant) *VerbatimCell {
	active := 0
	for i, v := range variants {
		if v.IsDefault {
			active = i
			break
		}
	}
	return &VerbatimCell{
		id:       id,
		Label:    label,
		Variants: variants,
		Active:   active,
		Style:    DefaultVerbatimStyle(),
		clip:     notebook.NoClipboard,
	}
}

// SetClipboard wires the clipboard the cell uses on 'c'.
func (c *VerbatimCell) SetClipboard(clip notebook.Clipboard) {
	if clip == nil {
		clip = notebook.NoClipboard
	}
	c.clip = clip
}

// ID implements notebook.Cell.
func (c *VerbatimCell) ID() string { return c.id }

// HeightHint implements notebook.Cell.
func (c *VerbatimCell) HeightHint(width int) int {
	c.materialize(width, c.cachedFocused)
	h := c.cachedHeight
	if c.copyMsg != "" {
		h++
	}
	return h
}

// RenderRows implements notebook.Cell.
func (c *VerbatimCell) RenderRows(width, startRow, endRow int, focused bool, _ notebook.Mode) []string {
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
// in ViewMode Enter advances, Tab / Shift+Tab cycle variants,
// '1'..'9' jump, Esc releases focus. Other keys passthrough.
func (c *VerbatimCell) Update(msg tea.Msg, mode notebook.Mode) (notebook.Cell, tea.Cmd, bool) {
	if cm, ok := msg.(notebook.ClearCopyMsg); ok && cm.CellID == c.id {
		c.copyMsg = ""
		return c, nil, true
	}
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return c, nil, false
	}
	if keyMsg.String() == "c" {
		v := c.Variants[c.Active]
		strategy, ok := c.clip(v.Content)
		if ok {
			label := v.Label
			if label == "" {
				c.copyMsg = "(copied via " + strategy + ")"
			} else {
				c.copyMsg = "(copied " + label + " via " + strategy + ")"
			}
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
	case "tab":
		c.cycle(+1)
		return c, nil, true
	case "shift+tab":
		c.cycle(-1)
		return c, nil, true
	}
	s := keyMsg.String()
	if len(s) == 1 && s[0] >= '1' && s[0] <= '9' {
		idx := int(s[0] - '1')
		if idx < len(c.Variants) {
			c.Active = idx
			c.cachedLines = nil
		}
		return c, nil, true
	}
	return c, nil, false
}

// StatusHint implements notebook.Cell.
func (c *VerbatimCell) StatusHint(_ notebook.Mode) string {
	if len(c.Variants) <= 1 {
		return "c copy"
	}
	return fmt.Sprintf("Tab cycle · 1-%d jump · c copy", len(c.Variants))
}

func (c *VerbatimCell) cycle(delta int) {
	n := len(c.Variants)
	if n == 0 {
		return
	}
	c.Active = (c.Active + delta + n) % n
	c.cachedLines = nil
}

func (c *VerbatimCell) materialize(width int, focused bool) {
	if width <= 0 {
		width = 80
	}
	if c.cachedWidth == width &&
		c.cachedFocused == focused &&
		c.cachedActive == c.Active &&
		c.cachedStyle == c.Style &&
		c.cachedLabel == c.Label &&
		c.cachedLines != nil {
		return
	}
	border := c.Style.BorderColor
	if focused {
		border = c.Style.FocusBorderColor
	}

	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(c.Style.LabelColor)
	label := c.Label
	if label == "" {
		label = "verbatim"
	}
	parts := []string{labelStyle.Render(label)}

	if len(c.Variants) > 1 {
		parts = append(parts, c.renderTabs())
	}

	codeStyle := lipgloss.NewStyle().Foreground(c.Style.ContentColor)
	parts = append(parts, codeStyle.Render(c.Variants[c.Active].Content))

	content := strings.Join(parts, "\n\n")
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
	c.cachedActive = c.Active
	c.cachedStyle = c.Style
	c.cachedLabel = c.Label
	c.cachedLines = strings.Split(rendered, "\n")
	c.cachedHeight = len(c.cachedLines)
}

func (c *VerbatimCell) renderTabs() string {
	active := lipgloss.NewStyle().Bold(true).Foreground(c.Style.ActiveTabColor)
	dim := lipgloss.NewStyle().Foreground(c.Style.InactiveTabColor)
	var out []string
	for i, v := range c.Variants {
		name := v.Label
		if name == "" {
			name = fmt.Sprintf("#%d", i+1)
		}
		if i == c.Active {
			out = append(out, active.Render("["+name+"]"))
		} else {
			out = append(out, dim.Render(" "+name+" "))
		}
	}
	return strings.Join(out, " ")
}
