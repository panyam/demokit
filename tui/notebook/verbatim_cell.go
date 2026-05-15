package notebook

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/panyam/demokit"
)

// VerbatimCell renders one verbatim block — possibly with multiple
// variants shown as a tab strip + a single active variant's body.
// In focused / view mode:
//
//   - Tab / Shift+Tab cycle the active variant (wrap).
//   - '1'..'9' jump directly (out-of-range ignored).
//   - 'c' copies the active variant's Content via demokit.Copy.
//
// Single-variant blocks omit the tab strip but still respect 'c'.
type VerbatimCell struct {
	id       string
	label    string
	variants []demokit.Variant
	active   int
	palette  Palette

	cachedWidth   int
	cachedFocused bool
	cachedActive  int
	cachedLines   []string
	cachedHeight  int

	copyMsg string
}

// NewVerbatimCell builds a cell from a flat slice of demokit.Variants.
// Active variant defaults to whichever carries IsDefault (first wins),
// or 0 if none does.
func NewVerbatimCell(id, label string, variants []demokit.Variant) *VerbatimCell {
	active := 0
	for i, v := range variants {
		if v.IsDefault {
			active = i
			break
		}
	}
	return &VerbatimCell{
		id: id, label: label, variants: variants, active: active,
		palette: DefaultPalette(),
	}
}

// SetPalette overrides the cell's palette.
func (c *VerbatimCell) SetPalette(p Palette) { c.palette = p; c.cachedLines = nil }

// ID implements Cell.
func (c *VerbatimCell) ID() string { return c.id }

// HeightHint implements Cell.
func (c *VerbatimCell) HeightHint(width int) int {
	c.materialize(width, c.cachedFocused)
	h := c.cachedHeight
	if c.copyMsg != "" {
		h++
	}
	return h
}

// RenderRows implements Cell.
func (c *VerbatimCell) RenderRows(width, startRow, endRow int, focused bool, _ Mode) []string {
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

// Update implements Cell.
func (c *VerbatimCell) Update(msg tea.Msg, mode Mode) (Cell, tea.Cmd) {
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
	case "enter":
		// Cell doesn't use Enter — signal release + advance.
		return c, cellAdvance
	case "tab":
		c.cycle(+1)
	case "shift+tab":
		c.cycle(-1)
	case "c":
		v := c.variants[c.active]
		strategy, ok := demokit.Copy(v.Content)
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
		return c, clearCopyMsgAfter(c.id)
	default:
		s := keyMsg.String()
		if len(s) == 1 && s[0] >= '1' && s[0] <= '9' {
			idx := int(s[0] - '1')
			if idx < len(c.variants) {
				c.active = idx
				c.cachedLines = nil
			}
		}
	}
	return c, nil
}

// StatusHint implements Cell.
func (c *VerbatimCell) StatusHint(_ Mode) string {
	if len(c.variants) <= 1 {
		return "c copy"
	}
	return fmt.Sprintf("Tab cycle · 1-%d jump · c copy", len(c.variants))
}

func (c *VerbatimCell) cycle(delta int) {
	n := len(c.variants)
	if n == 0 {
		return
	}
	c.active = (c.active + delta + n) % n
	c.cachedLines = nil
}

func (c *VerbatimCell) materialize(width int, focused bool) {
	if width <= 0 {
		width = 80
	}
	if c.cachedWidth == width &&
		c.cachedFocused == focused &&
		c.cachedActive == c.active &&
		c.cachedLines != nil {
		return
	}
	border := c.palette.VerbatimBorder
	if focused {
		border = c.palette.FocusBorder
	}

	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(c.palette.Title)
	label := c.label
	if label == "" {
		label = "verbatim"
	}
	parts := []string{labelStyle.Render(label)}

	if len(c.variants) > 1 {
		parts = append(parts, c.renderTabs())
	}

	codeStyle := lipgloss.NewStyle().Foreground(c.palette.Note)
	parts = append(parts, codeStyle.Render(c.variants[c.active].Content))

	content := strings.Join(parts, "\n\n")
	boxStyle := lipgloss.NewStyle().
		Border(focusedBorder(focused)).
		BorderForeground(border).
		Padding(0, 1).
		Width(maxBoxWidth(width))
	rendered := boxStyle.Render(content)

	c.cachedWidth = width
	c.cachedFocused = focused
	c.cachedActive = c.active
	c.cachedLines = strings.Split(rendered, "\n")
	c.cachedHeight = len(c.cachedLines)
}

// renderTabs builds the variant tab strip — active variant gets the
// Active color + brackets, others are dim. Single-variant cells skip
// this row.
func (c *VerbatimCell) renderTabs() string {
	active := lipgloss.NewStyle().Bold(true).Foreground(c.palette.Active)
	dim := lipgloss.NewStyle().Foreground(c.palette.Dim)
	var out []string
	for i, v := range c.variants {
		name := v.Label
		if name == "" {
			name = fmt.Sprintf("#%d", i+1)
		}
		if i == c.active {
			out = append(out, active.Render("["+name+"]"))
		} else {
			out = append(out, dim.Render(" "+name+" "))
		}
	}
	return strings.Join(out, " ")
}
