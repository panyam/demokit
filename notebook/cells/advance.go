package cells

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/panyam/demokit/notebook"
)

// AdvanceStyle is AdvancePromptCell's per-cell styling.
type AdvanceStyle struct {
	BorderColor      color.Color
	FocusBorderColor color.Color
	LabelColor       color.Color
	// BarFilledColor + BarEmptyColor render the countdown
	// progress bar that's drawn when Deadline is set.
	BarFilledColor color.Color
	BarEmptyColor  color.Color
	Edges          BorderEdges
}

// DarkAdvanceStyle returns the dark-terminal defaults.
func DarkAdvanceStyle() AdvanceStyle {
	return AdvanceStyle{
		BorderColor:      lipgloss.Color("#888888"),
		FocusBorderColor: lipgloss.Color("#FF6B6B"),
		LabelColor:       lipgloss.Color("#FAFAFA"),
		BarFilledColor:   lipgloss.Color("#FF6B6B"),
		BarEmptyColor:    lipgloss.Color("#444444"),
		Edges:            AllEdges(),
	}
}

// LightAdvanceStyle returns the light-terminal defaults.
func LightAdvanceStyle() AdvanceStyle {
	return AdvanceStyle{
		BorderColor:      lipgloss.Color("#777777"),
		FocusBorderColor: lipgloss.Color("#D34545"),
		LabelColor:       lipgloss.Color("#1A1A1A"),
		BarFilledColor:   lipgloss.Color("#D34545"),
		BarEmptyColor:    lipgloss.Color("#BBBBBB"),
		Edges:            AllEdges(),
	}
}

// DefaultAdvanceStyle returns the package default — Dark.
func DefaultAdvanceStyle() AdvanceStyle { return DarkAdvanceStyle() }

// AdvancePromptCell is a single-line "press Enter to continue"
// rendezvous cell. Unlike PromptCell it collects no input — Enter
// emits a PromptSubmittedMsg with an empty Answers map so callers
// using AwaitInputBy on it unblock immediately.
//
// Cleaner than reusing PromptCell with no inputs: this cell's
// rendered footer reads "Enter to continue · Esc release" instead
// of PromptCell's "Enter to submit · Tab cycle · Esc release",
// which is misleading when there's nothing to submit.
type AdvancePromptCell struct {
	Label string
	Style AdvanceStyle

	// Deadline (optional) is the absolute time at which the
	// cell auto-submits with Source="auto-advance". Zero means
	// "no deadline — block until the user presses Enter."
	//
	// When set, RenderRows draws a depleting progress bar; the
	// first Update arrival schedules a tea.Tick that emits
	// PromptSubmittedMsg when the timer fires.
	Deadline time.Time

	// total caches the original duration so the bar can render
	// a meaningful "filled vs empty" ratio. Set on first
	// materialize when Deadline is non-zero.
	total time.Duration

	id            string
	cachedWidth   int
	cachedFocused bool
	cachedLines   []string
	cachedHeight  int
	done          bool
	scheduled     bool
}

// NewAdvance builds an AdvancePromptCell. label is the rendered
// text; if empty, defaults to "Press Enter to continue".
func NewAdvance(id, label string) *AdvancePromptCell {
	if label == "" {
		label = "Press Enter to continue"
	}
	return &AdvancePromptCell{
		id:    id,
		Label: label,
		Style: DefaultAdvanceStyle(),
	}
}

// ID implements notebook.Cell.
func (c *AdvancePromptCell) ID() string { return c.id }

// HeightHint implements notebook.Cell.
func (c *AdvancePromptCell) HeightHint(width int) int {
	c.materialize(width, c.cachedFocused)
	return c.cachedHeight
}

// RenderRows implements notebook.Cell.
func (c *AdvancePromptCell) RenderRows(width, startRow, endRow int, focused bool, _ notebook.Mode) []string {
	c.materialize(width, focused)
	if startRow < 0 {
		startRow = 0
	}
	if endRow > c.cachedHeight {
		endRow = c.cachedHeight
	}
	if startRow >= endRow {
		return nil
	}
	out := make([]string, endRow-startRow)
	copy(out, c.cachedLines[startRow:endRow])
	return out
}

// Update implements notebook.Cell. Enter in ViewMode emits
// PromptSubmittedMsg with Source="user-submitted". Esc in
// ViewMode releases focus without submitting. Other keys
// passthrough.
//
// When Deadline is set, the first Update arrival schedules a
// tea.Tick that fires at the deadline and emits
// PromptSubmittedMsg with Source="auto-advance". Once
// auto-submitted (or user-submitted) the cell is done; further
// updates passthrough.
func (c *AdvancePromptCell) Update(msg tea.Msg, mode notebook.Mode) (notebook.Cell, tea.Cmd, bool) {
	if c.done {
		return c, nil, false
	}
	var schedCmd tea.Cmd
	if !c.scheduled && !c.Deadline.IsZero() {
		c.scheduled = true
		id := c.id
		remaining := time.Until(c.Deadline)
		if remaining < 0 {
			remaining = 0
		}
		schedCmd = tea.Tick(remaining, func(_ time.Time) tea.Msg {
			return notebook.PromptSubmittedMsg{
				CellID: id, Answers: nil, Source: "auto-advance",
			}
		})
	}
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return c, schedCmd, false
	}
	if mode != notebook.ViewMode {
		return c, schedCmd, false
	}
	switch keyMsg.String() {
	case "enter":
		c.done = true
		id := c.id
		userCmd := func() tea.Msg {
			return notebook.PromptSubmittedMsg{
				CellID: id, Answers: nil, Source: "user-submitted",
			}
		}
		if schedCmd != nil {
			return c, tea.Batch(schedCmd, userCmd), true
		}
		return c, userCmd, true
	case "esc":
		if schedCmd != nil {
			return c, tea.Batch(schedCmd, notebook.ReleaseFocus), true
		}
		return c, notebook.ReleaseFocus, true
	}
	return c, schedCmd, false
}

// StatusHint implements notebook.Cell.
func (c *AdvancePromptCell) StatusHint(_ notebook.Mode) string {
	return "Enter continue · Esc release"
}

func (c *AdvancePromptCell) materialize(width int, focused bool) {
	// When Deadline is active, every frame's output depends on
	// time.Now() — skip the cache entirely. The 16ms tick
	// drives smooth animation.
	hasDeadline := !c.Deadline.IsZero() && !c.done
	if !hasDeadline {
		if c.cachedWidth == width && c.cachedFocused == focused && c.cachedLines != nil {
			return
		}
	}
	border := c.Style.BorderColor
	if focused {
		border = c.Style.FocusBorderColor
	}
	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(c.Style.LabelColor)
	label := c.Label
	if c.done {
		label = "✓ " + label
	} else {
		label = "▸ " + label
	}
	content := labelStyle.Render(label)
	if hasDeadline {
		content += "\n" + c.renderCountdown()
	}
	box := lipgloss.NewStyle().
		Border(focusedBorder(focused)).
		BorderForeground(border).
		BorderTop(c.Style.Edges.Top).
		BorderRight(c.Style.Edges.Right).
		BorderBottom(c.Style.Edges.Bottom).
		BorderLeft(c.Style.Edges.Left).
		Padding(0, 1).
		Width(innerWidth(width, c.Style.Edges)).
		Render(content)
	c.cachedWidth = width
	c.cachedFocused = focused
	c.cachedLines = strings.Split(box, "\n")
	c.cachedHeight = len(c.cachedLines)
}

// renderCountdown returns a single line containing the
// depleting progress bar and the remaining seconds. Called from
// materialize when Deadline is set and the cell isn't done yet.
func (c *AdvancePromptCell) renderCountdown() string {
	remaining := time.Until(c.Deadline)
	if remaining < 0 {
		remaining = 0
	}
	if c.total == 0 {
		// First render snapshots the duration so the bar always
		// shows progress relative to "the full wait", not just
		// "however long is left."
		c.total = remaining
		if c.total <= 0 {
			c.total = 1
		}
	}
	const barWidth = 20
	frac := float64(remaining) / float64(c.total)
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	filled := int(frac * float64(barWidth))
	if remaining > 0 && filled == 0 {
		filled = 1 // never collapse to zero while the timer is alive
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)

	filledStyle := lipgloss.NewStyle().Foreground(c.Style.BarFilledColor)
	emptyStyle := lipgloss.NewStyle().Foreground(c.Style.BarEmptyColor)
	rendered := filledStyle.Render(bar[:filled*len("█")]) +
		emptyStyle.Render(bar[filled*len("█"):])
	return fmt.Sprintf("%s  %4.1fs", rendered, remaining.Seconds())
}
