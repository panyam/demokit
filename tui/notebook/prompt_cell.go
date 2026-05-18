package notebook

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/panyam/demokit/events"
)

// promptInput is PromptCell's internal projection of an
// events.Input. Keeps a private Parse function (reconstructed
// from Kind + Options) so the cell can validate user typing
// without re-importing demokit at the cell layer.
type promptInput struct {
	Name    string
	Prompt  string
	Default any
	Kind    string
	Options []string
	parse   func(string) (any, error)
}

// promptInputsFromEvents converts events.Input slice into the
// cell-internal form, deriving a Parse closure via type switch.
func promptInputsFromEvents(in []events.Input) []promptInput {
	out := make([]promptInput, len(in))
	for i, e := range in {
		out[i] = promptInputFromEvent(e)
	}
	return out
}

func promptInputFromEvent(e events.Input) promptInput {
	p := promptInput{
		Name:    e.InputName(),
		Prompt:  e.InputPrompt(),
		Default: e.InputDefault(),
	}
	switch v := e.(type) {
	case events.IntInput:
		_ = v
		p.Kind = "int"
		p.parse = func(s string) (any, error) {
			n, err := strconv.Atoi(strings.TrimSpace(s))
			if err != nil {
				return nil, fmt.Errorf("not an integer: %q", s)
			}
			return n, nil
		}
	case events.ChoiceInput:
		p.Kind = "choice"
		p.Options = append([]string(nil), v.Options...)
		opts := p.Options
		p.parse = func(s string) (any, error) {
			got := strings.TrimSpace(s)
			for _, opt := range opts {
				if strings.EqualFold(got, opt) {
					return opt, nil
				}
			}
			return nil, fmt.Errorf("must be one of: %s", strings.Join(opts, ", "))
		}
	default:
		p.Kind = "string"
		p.parse = func(s string) (any, error) { return s, nil }
	}
	return p
}

// PromptCell collects step inputs via a stack of bubbles/textinput
// fields — one per declared events.Input. Tab / Shift+Tab cycle
// the active field; Enter submits when all parse cleanly (focus
// jumps to the first invalid field otherwise); Esc unfocuses
// without submitting so the user can read other cells before
// answering.
//
// On submit, the cell calls queue.Resolve(promptOffset, …) with
// a PromptResolution carrying the typed answer map — closing the
// sync rendezvous with demokit.Execute.
type PromptCell struct {
	id           string
	inputs       []promptInput
	fields       []textinput.Model
	errors       []string
	active       int
	queue        *events.EventQueue
	promptOffset int
	done         bool
	palette      Palette
}

// NewPromptCell builds a cell over the public events.Input list.
// queue + promptOffset identify the PromptOpen event the cell
// resolves on submit. The first field is focused; defaults are
// pre-filled into the textinput placeholders.
func NewPromptCell(id string, inputs []events.Input, queue *events.EventQueue, promptOffset int, palette Palette) *PromptCell {
	pi := promptInputsFromEvents(inputs)
	fields := make([]textinput.Model, len(pi))
	errs := make([]string, len(pi))
	for i, in := range pi {
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
		id: id, inputs: pi, fields: fields, errors: errs,
		queue: queue, promptOffset: promptOffset, palette: palette,
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
	// Blur every textinput so the bubbles cursor stops blinking
	// after submission.
	for i := range c.fields {
		c.fields[i].Blur()
	}
	if c.queue != nil {
		_ = c.queue.Resolve(c.promptOffset, &events.PromptResolution{
			Answers: answers, Source: "user-submitted", Timestamp: time.Now(),
		})
	}
	// Submit succeeded → pop focus and advance.
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
