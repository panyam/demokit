package notebook

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// Palette holds the colors the notebook UI paints with. Mirrors
// the tui.Palette shape but kept separate so the notebook package
// doesn't depend on tui internals — the two renderers are siblings,
// not parent/child.
//
// Per-cell-type border colors keep the visual hierarchy explicit
// (meta is the step header, output is the result of execution,
// etc.); focused cells override the border with FocusBorder so the
// cursor position is unmistakable.
type Palette struct {
	FocusBorder    color.Color
	MetaBorder     color.Color
	SectionBorder  color.Color
	VerbatimBorder color.Color
	OutputBorder   color.Color
	PromptBorder   color.Color

	Title    color.Color
	Note     color.Color
	Dim      color.Color
	Active   color.Color // active variant label in the tab strip
	Error    color.Color
	Success  color.Color
	Header   color.Color // bottom-row status / banner accent
}

// focusedBorder returns the lipgloss border style to apply for a
// cell at the given focus state. Unfocused cells use a rounded
// single-line border; focused cells switch to a thick (heavy
// single-line) border so the cursor is unmistakable even when the
// color delta is faint (low-contrast terminals, SSH over a
// muted color profile, color-blind users).
//
// Both styles occupy identical character positions — one row of
// border on each side — so HeightHint stays accurate and the
// viewport row math doesn't shift when focus moves between cells.
func focusedBorder(focused bool) lipgloss.Border {
	if focused {
		return lipgloss.ThickBorder()
	}
	return lipgloss.RoundedBorder()
}

// DefaultPalette returns a palette tuned for dark-background
// terminals (the common case). Auto-detection via
// lipgloss.HasDarkBackground is intentionally avoided here:
// HasDarkBackground writes an OSC query to the terminal and reads
// the reply from stdin. Called at construction time (before BT has
// entered alt-screen + raw mode), the reply echoes to the visible
// screen — leaving a `^[[?64;...c` escape sequence stuck above
// the notebook UI until something repaints. Light-terminal users
// can supply their own palette via Renderer.WithPalette.
func DefaultPalette() Palette {
	return Palette{
		FocusBorder:    lipgloss.Color("#FF6B6B"),
		MetaBorder:     lipgloss.Color("#7D56F4"),
		SectionBorder:  lipgloss.Color("#626262"),
		VerbatimBorder: lipgloss.Color("#5BB1FF"),
		OutputBorder:   lipgloss.Color("#04B575"),
		PromptBorder:   lipgloss.Color("#FFD700"),

		Title:   lipgloss.Color("#FAFAFA"),
		Note:    lipgloss.Color("#CCCCCC"),
		Dim:     lipgloss.Color("#888888"),
		Active:  lipgloss.Color("#FF6B6B"),
		Error:   lipgloss.Color("#FF4444"),
		Success: lipgloss.Color("#04B575"),
		Header:  lipgloss.Color("#FF6B6B"),
	}
}
