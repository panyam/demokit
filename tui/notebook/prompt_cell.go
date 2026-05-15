package notebook

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// PromptCell collects step inputs via a stack of bubbles/textinput
// fields — one per declared InputDef. Tab / Shift+Tab cycle the
// active field; Enter submits when all parse cleanly (focus jumps
// to the first invalid field otherwise); Esc unfocuses without
// submitting so the user can read other cells before answering.
//
// Each field renders its label, optional Default hint, optional
// Options hint (Choice inputs), the textinput row, and an error
// row (when the most recent parse failed). Submission closes the
// supplied reply channel after sending the typed map.
type PromptCell struct {
	id      string
	inputs  []promptInput
	fields  []textinput.Model
	errors  []string
	active  int
	reply   chan map[string]any
	done    bool
	palette Palette
}

// NewPromptCell builds a cell over the input list + reply channel.
// The first field is focused; defaults are pre-filled into the
// textinput.
func NewPromptCell(id string, inputs []promptInput, reply chan map[string]any, palette Palette) *PromptCell {
	fields := make([]textinput.Model, len(inputs))
	errs := make([]string, len(inputs))
	for i, in := range inputs {
		ti := textinput.New()
		ti.Prompt = "› "
		ti.CharLimit = 256
		ti.Width = 40
		if in.Default != nil {
			ti.Placeholder = fmt.Sprintf("%v", in.Default)
		}
		fields[i] = ti
	}
	if len(fields) > 0 {
		fields[0].Focus()
	}
	return &PromptCell{
		id: id, inputs: inputs, fields: fields, errors: errs, reply: reply, palette: palette,
	}
}

// SetPalette overrides the cell's palette.
func (c *PromptCell) SetPalette(p Palette) { c.palette = p }

// ID implements Cell.
func (c *PromptCell) ID() string { return c.id }

// HeightHint implements Cell. Each field contributes a label row,
// optional default/options hint row, the input row, and an
// optional error row. Plus 2 border rows from the lipgloss box and
// a trailing "Enter to submit" row.
func (c *PromptCell) HeightHint(_ int) int {
	rows := 0
	for i, in := range c.inputs {
		rows++ // label
		if in.Default != nil || len(in.Options) > 0 {
			rows++ // hint
		}
		rows++ // input
		if c.errors[i] != "" {
			rows++ // error
		}
		rows++ // blank gap
	}
	rows++ // submit hint
	rows += 2 // border top + bottom
	return rows
}

// RenderRows implements Cell.
func (c *PromptCell) RenderRows(width, startRow, endRow int, focused bool, _ Mode) []string {
	border := c.palette.PromptBorder
	if focused {
		border = c.palette.FocusBorder
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(c.palette.Title)
	hintStyle := lipgloss.NewStyle().Foreground(c.palette.Dim)
	errStyle := lipgloss.NewStyle().Foreground(c.palette.Error)
	submitStyle := lipgloss.NewStyle().Italic(true).Foreground(c.palette.Note)

	var lines []string
	for i, in := range c.inputs {
		label := in.Prompt
		if label == "" {
			label = in.Name
		}
		marker := "  "
		if focused && i == c.active {
			marker = "▶ "
		}
		lines = append(lines, marker+titleStyle.Render(label))
		if hint := buildHint(in); hint != "" {
			lines = append(lines, "  "+hintStyle.Render(hint))
		}
		lines = append(lines, "  "+c.fields[i].View())
		if c.errors[i] != "" {
			lines = append(lines, "  "+errStyle.Render("✗ "+c.errors[i]))
		}
		lines = append(lines, "")
	}
	if c.done {
		lines = append(lines, submitStyle.Render("✓ submitted"))
	} else {
		lines = append(lines, submitStyle.Render("Enter to submit · Tab cycle · Esc release"))
	}

	boxStyle := lipgloss.NewStyle().
		Border(focusedBorder(focused)).
		BorderForeground(border).
		Padding(0, 1).
		Width(maxBoxWidth(width))
	rendered := boxStyle.Render(strings.Join(lines, "\n"))
	rows := strings.Split(rendered, "\n")

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

// Update implements Cell. Routes keys to the active textinput;
// Enter / Tab / Shift+Tab control navigation and submission.
func (c *PromptCell) Update(msg tea.Msg, mode Mode) (Cell, tea.Cmd) {
	if c.done {
		return c, nil
	}
	if mode != ViewMode {
		return c, nil
	}
	keyMsg, isKey := msg.(tea.KeyMsg)
	if !isKey {
		return c, nil
	}
	switch keyMsg.String() {
	case "tab":
		c.advance(+1)
		return c, nil
	case "shift+tab":
		c.advance(-1)
		return c, nil
	case "enter":
		return c.trySubmit()
	}
	if len(c.fields) == 0 {
		return c, nil
	}
	var cmd tea.Cmd
	c.fields[c.active], cmd = c.fields[c.active].Update(msg)
	return c, cmd
}

// StatusHint implements Cell.
func (c *PromptCell) StatusHint(_ Mode) string {
	return "Tab cycle · Enter submit · Esc release"
}

// advance moves focus to the next/previous field. Wraps at the
// edges. The previously-focused textinput is Blur()'d; the new one
// is Focus()'d so its cursor blinks.
func (c *PromptCell) advance(delta int) {
	if len(c.fields) == 0 {
		return
	}
	c.fields[c.active].Blur()
	n := len(c.fields)
	c.active = (c.active + delta + n) % n
	c.fields[c.active].Focus()
}

// trySubmit parses each field. If any errors, the first invalid
// field becomes active and its error is recorded. If all succeed,
// the answer map is sent via Reply and the channel closed; the
// cell flips to "done" so subsequent keys are ignored.
func (c *PromptCell) trySubmit() (Cell, tea.Cmd) {
	answers := map[string]any{}
	firstInvalid := -1
	for i, in := range c.inputs {
		raw := strings.TrimSpace(c.fields[i].Value())
		if raw == "" && in.Default != nil {
			answers[in.Name] = in.Default
			c.errors[i] = ""
			continue
		}
		val, err := in.parse(raw)
		if err != nil {
			c.errors[i] = err.Error()
			if firstInvalid < 0 {
				firstInvalid = i
			}
			continue
		}
		c.errors[i] = ""
		answers[in.Name] = val
	}
	if firstInvalid >= 0 {
		c.fields[c.active].Blur()
		c.active = firstInvalid
		c.fields[c.active].Focus()
		return c, nil
	}
	c.done = true
	if c.reply != nil {
		c.reply <- answers
		close(c.reply)
	}
	// Submit succeeded → pop focus and advance. The user
	// shouldn't need to Esc-then-Enter after entering valid
	// answers; Enter on a complete form should "just continue."
	return c, cellAdvance
}

// buildHint composes the per-field hint line: shows the default
// value (if any) and the option list (for Choice). Empty hint
// means no hint row is rendered for that field.
func buildHint(in promptInput) string {
	var parts []string
	if len(in.Options) > 0 {
		parts = append(parts, "options: "+strings.Join(in.Options, " · "))
	}
	if in.Default != nil {
		parts = append(parts, fmt.Sprintf("default: %v", in.Default))
	}
	return strings.Join(parts, "  ·  ")
}
