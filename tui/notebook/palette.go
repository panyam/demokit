package notebook

import (
	"image/color"
	"os"

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

// DefaultPalette returns a palette adapted to the terminal's
// background. Uses lipgloss.HasDarkBackground for auto detection,
// same as tui.DefaultPalette.
func DefaultPalette() Palette {
	ld := lipgloss.LightDark(lipgloss.HasDarkBackground(os.Stdin, os.Stderr))
	return Palette{
		FocusBorder:    ld(lipgloss.Color("#D04040"), lipgloss.Color("#FF6B6B")),
		MetaBorder:     ld(lipgloss.Color("#6C3FC7"), lipgloss.Color("#7D56F4")),
		SectionBorder:  ld(lipgloss.Color("#999999"), lipgloss.Color("#626262")),
		VerbatimBorder: ld(lipgloss.Color("#0070CC"), lipgloss.Color("#5BB1FF")),
		OutputBorder:   ld(lipgloss.Color("#039960"), lipgloss.Color("#04B575")),
		PromptBorder:   ld(lipgloss.Color("#B8860B"), lipgloss.Color("#FFD700")),

		Title:   ld(lipgloss.Color("#1A1A1A"), lipgloss.Color("#FAFAFA")),
		Note:    ld(lipgloss.Color("#555555"), lipgloss.Color("#CCCCCC")),
		Dim:     ld(lipgloss.Color("#999999"), lipgloss.Color("#888888")),
		Active:  ld(lipgloss.Color("#D04040"), lipgloss.Color("#FF6B6B")),
		Error:   ld(lipgloss.Color("#CC2222"), lipgloss.Color("#FF4444")),
		Success: ld(lipgloss.Color("#039960"), lipgloss.Color("#04B575")),
		Header:  ld(lipgloss.Color("#D04040"), lipgloss.Color("#FF6B6B")),
	}
}
