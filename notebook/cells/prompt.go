package cells

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/panyam/demokit/notebook"
)

// PromptStyle is PromptCell's per-cell styling.
type PromptStyle struct {
	BorderColor      color.Color
	FocusBorderColor color.Color
	LabelColor       color.Color
	HintColor        color.Color
	ErrorColor       color.Color
	SubmitColor      color.Color
	Edges            BorderEdges
}

// DarkPromptStyle returns the dark-terminal defaults.
func DarkPromptStyle() PromptStyle {
	return PromptStyle{
		BorderColor:      lipgloss.Color("#FFD700"),
		FocusBorderColor: lipgloss.Color("#FF6B6B"),
		LabelColor:       lipgloss.Color("#FAFAFA"),
		HintColor:        lipgloss.Color("#888888"),
		ErrorColor:       lipgloss.Color("#FF4444"),
		SubmitColor:      lipgloss.Color("#CCCCCC"),
		Edges:            AllEdges(),
	}
}

// LightPromptStyle returns the light-terminal defaults.
func LightPromptStyle() PromptStyle {
	return PromptStyle{
		BorderColor:      lipgloss.Color("#C99A00"),
		FocusBorderColor: lipgloss.Color("#D34545"),
		LabelColor:       lipgloss.Color("#1A1A1A"),
		HintColor:        lipgloss.Color("#777777"),
		ErrorColor:       lipgloss.Color("#CC2222"),
		SubmitColor:      lipgloss.Color("#444444"),
		Edges:            AllEdges(),
	}
}

// DefaultPromptStyle returns the package default — Dark.
func DefaultPromptStyle() PromptStyle { return DarkPromptStyle() }

// PromptCell collects answers via a stack of bubbles/textinput
// fields — one per notebook.Input. Tab / Shift+Tab cycle the
// active field; Enter submits when all fields parse cleanly (focus
// jumps to the first invalid field otherwise); Esc is handled by
// the notebook, not the cell.
//
// On a successful submit the cell emits a notebook.PromptSubmittedMsg
// carrying the parsed answer map; the notebook routes it to the
// pending AwaitInput call.
type PromptCell struct {
	Style PromptStyle

	id     string
	inputs []notebook.Input
	fields []textinput.Model
	errors []string
	active int
	done   bool
}

// PromptFactory returns a notebook.PromptFactory that builds the
// built-in PromptCell. Pass it to notebook.WithPromptFactory so
// nb.AwaitInput([]Input) constructs prompt cells automatically.
//
// The notebook package can't import its own cells subpackage
// (would cycle), so wiring like this is a consumer concern.
func PromptFactory() notebook.PromptFactory {
	return func(id notebook.CellID, inputs []notebook.Input) notebook.Cell {
		return NewPrompt(id, inputs)
	}
}

// NewPrompt builds a PromptCell over the given inputs. The first
// field is focused; defaults are pre-filled as textinput
// placeholders.
func NewPrompt(id string, inputs []notebook.Input) *PromptCell {
	fields := make([]textinput.Model, len(inputs))
	errs := make([]string, len(inputs))
	for i, in := range inputs {
		ti := textinput.New()
		ti.Prompt = "› "
		ti.CharLimit = 256
		ti.Width = 40
		if d := in.InputDefault(); d != nil {
			ti.Placeholder = formatDefault(d)
		}
		fields[i] = ti
	}
	// Intentionally NOT calling fields[0].Focus() — textinput's
	// blinking cursor is the cell's "I'm focused" manifestation,
	// and the cell isn't focused at construction. syncFieldFocus
	// in RenderRows turns it on when (focused && CellActiveMode).
	return &PromptCell{
		id:     id,
		inputs: inputs,
		fields: fields,
		errors: errs,
		Style:  DefaultPromptStyle(),
	}
}

// syncFieldFocus drives the textinput Focus state from the
// cell's render-time (focused, mode) so the cursor only blinks
// when the cell itself is focused AND the notebook is in
// CellActiveMode. Idempotent — safe to call every render.
func (c *PromptCell) syncFieldFocus(focused bool, mode notebook.Mode) {
	wantFocused := focused && mode == notebook.CellActiveMode && !c.done
	for i := range c.fields {
		on := wantFocused && i == c.active
		if on && !c.fields[i].Focused() {
			c.fields[i].Focus()
		} else if !on && c.fields[i].Focused() {
			c.fields[i].Blur()
		}
	}
}

// ID implements notebook.Cell.
func (c *PromptCell) ID() string { return c.id }

// HeightHint implements notebook.Cell.
func (c *PromptCell) HeightHint(_ int) int {
	rows := 0
	for i, in := range c.inputs {
		rows++ // label
		if buildHint(in) != "" {
			rows++ // hint
		}
		rows++ // input
		if c.errors[i] != "" {
			rows++ // error
		}
		rows++ // blank gap
	}
	rows++ // submit hint
	rows += chromeRows(c.Style.Edges)
	return rows
}

// RenderRows implements notebook.Cell.
func (c *PromptCell) RenderRows(width, startRow, endRow int, focused bool, mode notebook.Mode) []string {
	c.syncFieldFocus(focused, mode)

	border := c.Style.BorderColor
	if focused {
		border = c.Style.FocusBorderColor
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(c.Style.LabelColor)
	hintStyle := lipgloss.NewStyle().Foreground(c.Style.HintColor)
	errStyle := lipgloss.NewStyle().Foreground(c.Style.ErrorColor)
	submitStyle := lipgloss.NewStyle().Italic(true).Foreground(c.Style.SubmitColor)

	var lines []string
	for i, in := range c.inputs {
		label := in.InputPrompt()
		if label == "" {
			label = in.InputName()
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
		BorderTop(c.Style.Edges.Top).
		BorderRight(c.Style.Edges.Right).
		BorderBottom(c.Style.Edges.Bottom).
		BorderLeft(c.Style.Edges.Left).
		Padding(0, 1).
		Width(innerWidth(width, c.Style.Edges))
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

// Update implements notebook.Cell. Routes keys to the active
// textinput; Enter / Tab / Shift+Tab control submission /
// navigation; Esc releases focus without submitting. In CellActiveMode
// the cell claims every key (typing into the textinput counts);
// in other modes or once submitted, keys passthrough.
func (c *PromptCell) Update(msg tea.Msg, mode notebook.Mode) (notebook.Cell, tea.Cmd, bool) {
	if c.done {
		return c, nil, false
	}
	if mode != notebook.CellActiveMode {
		return c, nil, false
	}
	keyMsg, isKey := msg.(tea.KeyMsg)
	if !isKey {
		return c, nil, false
	}
	switch keyMsg.String() {
	case "tab":
		c.advance(+1)
		return c, nil, true
	case "shift+tab":
		c.advance(-1)
		return c, nil, true
	case "esc":
		return c, notebook.ReleaseFocus, true
	case "enter":
		cell, cmd := c.trySubmit()
		return cell, cmd, true
	}
	if len(c.fields) == 0 {
		return c, nil, true
	}
	var cmd tea.Cmd
	c.fields[c.active], cmd = c.fields[c.active].Update(msg)
	return c, cmd, true
}

// StatusHint implements notebook.Cell.
func (c *PromptCell) StatusHint(_ notebook.Mode) string {
	return "Tab cycle · Enter submit · Esc release"
}

// advance moves focus to the next/previous field, wrapping at the
// edges.
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
// field becomes active. If all succeed, the cell flips to done and
// emits a PromptSubmittedMsg with the answer map.
func (c *PromptCell) trySubmit() (notebook.Cell, tea.Cmd) {
	answers := map[string]any{}
	firstInvalid := -1
	for i, in := range c.inputs {
		raw := strings.TrimSpace(c.fields[i].Value())
		if raw == "" && in.InputDefault() != nil {
			answers[in.InputName()] = in.InputDefault()
			c.errors[i] = ""
			continue
		}
		val, err := in.Parse(raw)
		if err != nil {
			c.errors[i] = err.Error()
			if firstInvalid < 0 {
				firstInvalid = i
			}
			continue
		}
		c.errors[i] = ""
		answers[in.InputName()] = val
	}
	if firstInvalid >= 0 {
		c.fields[c.active].Blur()
		c.active = firstInvalid
		c.fields[c.active].Focus()
		return c, nil
	}
	c.done = true
	for i := range c.fields {
		c.fields[i].Blur()
	}
	id := c.id
	return c, func() tea.Msg {
		return notebook.PromptSubmittedMsg{CellID: id, Answers: answers}
	}
}

// buildHint composes the per-field hint line: the input's
// type-specific hint plus a "default: …" suffix when a default is
// set. Empty result means no hint row.
func buildHint(in notebook.Input) string {
	var parts []string
	if h := in.Hint(); h != "" {
		parts = append(parts, h)
	}
	if d := in.InputDefault(); d != nil {
		parts = append(parts, "default: "+formatDefault(d))
	}
	return strings.Join(parts, "  ·  ")
}

// formatDefault renders an input default for display.
func formatDefault(d any) string {
	if s, ok := d.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", d)
}
