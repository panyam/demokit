package cells

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/panyam/demokit/notebook"
)

// CommandStyle is CommandCell's per-cell styling. A vim-style
// command bar is single-line, no border — apps that want chrome
// (border, padding) wrap the cell themselves.
type CommandStyle struct {
	PromptColor color.Color
	TextColor   color.Color
	CursorColor color.Color
}

// DarkCommandStyle returns the dark-terminal defaults.
func DarkCommandStyle() CommandStyle {
	return CommandStyle{
		PromptColor: lipgloss.Color("#7D56F4"),
		TextColor:   lipgloss.Color("#FAFAFA"),
		CursorColor: lipgloss.Color("#FF6B6B"),
	}
}

// LightCommandStyle returns the light-terminal defaults.
func LightCommandStyle() CommandStyle {
	return CommandStyle{
		PromptColor: lipgloss.Color("#5A3FCE"),
		TextColor:   lipgloss.Color("#1A1A1A"),
		CursorColor: lipgloss.Color("#D34545"),
	}
}

// DefaultCommandStyle returns the package default — Dark.
func DefaultCommandStyle() CommandStyle { return DarkCommandStyle() }

// CommandCell is a single-line text input designed for vim-style
// command bars at the Bottom dock. It captures every keystroke
// while focused: Enter invokes the onSubmit callback with the
// buffered text and emits notebook.ReleaseFocus; Esc invokes
// onCancel and emits notebook.ReleaseFocus; Backspace edits the
// buffer; printable runes append.
//
// CommandCell is deliberately framework-agnostic about teardown:
// it doesn't ClearDocked itself, doesn't restore the default
// status bar. Apps either do that in onSubmit/onCancel, or use
// the OpenCommandBar convenience which wraps the full lifecycle.
type CommandCell struct {
	Prompt string
	Style  CommandStyle

	id       string
	buf      string
	onSubmit func(string)
	onCancel func()
}

// NewCommandCell builds a CommandCell with the given prompt and
// callbacks. Either callback may be nil — Enter / Esc still
// release focus but no app code fires.
//
// The cell's ID is fixed ("notebook.command") so apps can
// reference it via nb.DockedCell(notebook.Bottom) without
// guessing.
func NewCommandCell(prompt string, onSubmit func(string), onCancel func()) *CommandCell {
	return &CommandCell{
		id:       "notebook.command",
		Prompt:   prompt,
		Style:    DefaultCommandStyle(),
		onSubmit: onSubmit,
		onCancel: onCancel,
	}
}

// ID implements notebook.Cell.
func (c *CommandCell) ID() string { return c.id }

// Text returns the current buffer. Useful for tests and for apps
// that want to peek at the partial input.
func (c *CommandCell) Text() string { return c.buf }

// HeightHint implements notebook.Cell. CommandCell auto-grows as
// the buffer wraps past width: a short ":ls" stays 1 row; a long
// shell pipeline expands until the dock would crowd the body, at
// which point the notebook clamps the dock and the cell scrolls
// to its tail on render.
//
// HeightHint factors in the focused-cursor rune so the height
// doesn't jiggle when focus toggles on/off at a wrap boundary.
func (c *CommandCell) HeightHint(width int) int {
	rows := c.wrappedRows(width, true)
	if len(rows) < 1 {
		return 1
	}
	return len(rows)
}

// RenderRows implements notebook.Cell. Returns the row window
// [startRow, endRow). When the docked allotment is smaller than
// HeightHint, the notebook passes a startRow > 0 — the cell
// happily yields the head and renders the tail (where the cursor
// lives) so the user always sees what they're typing.
func (c *CommandCell) RenderRows(width, startRow, endRow int, focused bool, _ notebook.Mode) []string {
	rows := c.wrappedRows(width, focused)
	if len(rows) == 0 {
		rows = []string{""}
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

	promptStyle := lipgloss.NewStyle().Foreground(c.Style.PromptColor)
	textStyle := lipgloss.NewStyle().Foreground(c.Style.TextColor)
	cursorStyle := lipgloss.NewStyle().Foreground(c.Style.CursorColor)
	promptRunes := []rune(c.Prompt + " ")
	promptLen := len(promptRunes)
	lastIdx := len(rows) - 1

	out := make([]string, 0, endRow-startRow)
	for i := startRow; i < endRow; i++ {
		runes := []rune(rows[i])
		var sb strings.Builder
		for ri, r := range runes {
			isCursor := focused && i == lastIdx && ri == len(runes)-1 && r == '█'
			isPrompt := i == 0 && ri < promptLen
			switch {
			case isCursor:
				sb.WriteString(cursorStyle.Render(string(r)))
			case isPrompt:
				sb.WriteString(promptStyle.Render(string(r)))
			default:
				sb.WriteString(textStyle.Render(string(r)))
			}
		}
		out = append(out, sb.String())
	}
	return out
}

// wrappedRows returns the buffer (with cursor when focused)
// wrapped to width-rune chunks. Rune-based so multi-byte UTF-8
// doesn't split mid-codepoint — column-aware wrapping (full-width
// CJK, ANSI) is left to a v2 if/when the use case shows up.
func (c *CommandCell) wrappedRows(width int, focused bool) []string {
	text := c.Prompt + " " + c.buf
	if focused {
		text += "█"
	}
	if width <= 0 {
		return []string{text}
	}
	runes := []rune(text)
	var rows []string
	for len(runes) > width {
		rows = append(rows, string(runes[:width]))
		runes = runes[width:]
	}
	if len(runes) > 0 || len(rows) == 0 {
		rows = append(rows, string(runes))
	}
	return rows
}

// Update implements notebook.Cell. Captures every key while
// focused — handled=true on every keystroke so notebook KeyMap
// bindings don't fire underneath it (otherwise typing 'j' in the
// command buffer would also nav-down).
//
// In NavigationMode (cell not focused) the cell ignores keys so
// global bindings like Ctrl+W still work to focus it.
func (c *CommandCell) Update(msg tea.Msg, mode notebook.Mode) (notebook.Cell, tea.Cmd, bool) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return c, nil, false
	}
	if mode != notebook.CellActiveMode {
		return c, nil, false
	}
	switch keyMsg.String() {
	case "enter":
		text := c.buf
		c.buf = ""
		if c.onSubmit != nil {
			c.onSubmit(text)
		}
		return c, notebook.ReleaseFocus, true
	case "esc":
		c.buf = ""
		if c.onCancel != nil {
			c.onCancel()
		}
		return c, notebook.ReleaseFocus, true
	case "backspace":
		if len(c.buf) > 0 {
			// Strip a UTF-8 rune off the end, not a byte.
			r := []rune(c.buf)
			c.buf = string(r[:len(r)-1])
		}
		return c, nil, true
	case "ctrl+u":
		c.buf = ""
		return c, nil, true
	}
	// Printable input — concatenate runes.
	if len(keyMsg.Runes) > 0 {
		c.buf += string(keyMsg.Runes)
		return c, nil, true
	}
	// Modifier-only / unhandled control keys: passthrough so global
	// bindings (Ctrl+C quit) still work.
	return c, nil, false
}

// StatusHint implements notebook.Cell.
func (c *CommandCell) StatusHint(_ notebook.Mode) string {
	return "Enter run · Esc cancel"
}

// OpenCommandBar is the one-liner apps use to wire a vim-style
// command bar. It:
//
//  1. Snapshots whatever Cell is currently at Bottom (typically
//     the default StatusCell or an app's custom status bar).
//  2. Builds a CommandCell with the given prompt and a cleanup
//     wrapper that restores the SAME instance on Enter / Esc —
//     so a stateful status bar keeps its state across the show /
//     hide cycle (apps that want a fresh instance pre-stash one
//     and ClearDocked manually).
//  3. Installs the cell at the Bottom dock and focuses it.
//
// Returns the tea.Cmd that flips the notebook into CellActiveMode.
// Apps using OpenCommandBar from a KeyMap action return the cmd
// directly:
//
//	km := notebook.DefaultKeyMap()
//	km.Modes[notebook.NavigationMode][":"] = func(nb *notebook.Notebook) tea.Cmd {
//	    return cells.OpenCommandBar(nb, ":", onColon)
//	}
//	nb := notebook.New(notebook.WithKeyMap(km), ...)
//
// (Returning the cmd is required for the mode flip to reach the
// BT loop — Sending from inside the action would deadlock.)
//
// onSubmit may be nil to make the bar non-functional (still
// dismisses on Esc / Enter). onSubmit receives the buffer with
// trailing whitespace trimmed; leading whitespace is preserved
// so commands like ":  echo hi" can be inspected as typed.
func OpenCommandBar(nb *notebook.Notebook, prompt string, onSubmit func(string)) tea.Cmd {
	prior, hadPrior := nb.DockedCell(notebook.Bottom)
	restore := func() {
		if hadPrior {
			nb.SetDockedCell(notebook.Bottom, prior)
		} else {
			nb.ClearDocked(notebook.Bottom)
		}
	}
	cell := NewCommandCell(prompt,
		func(text string) {
			if onSubmit != nil {
				onSubmit(strings.TrimRight(text, " "))
			}
			restore()
		},
		restore,
	)
	nb.SetDockedCell(notebook.Bottom, cell)
	if !nb.FocusDock(notebook.Bottom) {
		return nil
	}
	return notebook.ModeCmd(notebook.CellActiveMode)
}
