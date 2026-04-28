// Package tui provides a Lipgloss-styled renderer for demokit demos.
// Steps, sections, and results are rendered in visually distinct bordered
// boxes with differentiated styling for titles, arrows, notes, and refs.
package tui

import (
	"bufio"
	"fmt"
	"image/color"
	"os"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/term"
	"github.com/panyam/demokit"
)

// Palette holds the colors used by the TUI renderer.
// Override fields to customize the color scheme.
type Palette struct {
	StepBorder    color.Color
	SectionBorder color.Color
	ResultBorder  color.Color
	StepNumber    color.Color
	Title         color.Color
	Arrow         color.Color
	DashedArrow   color.Color
	Note          color.Color
	Ref           color.Color
	Prompt        color.Color
	Success       color.Color
	Error         color.Color
	Header        color.Color
	Dim           color.Color
}

// DefaultPalette returns a color palette adapted to the terminal's background.
// Uses lipgloss.HasDarkBackground for automatic dark/light detection.
func DefaultPalette() Palette {
	ld := lipgloss.LightDark(lipgloss.HasDarkBackground(os.Stdin, os.Stderr))
	return Palette{
		StepBorder:    ld(lipgloss.Color("#6C3FC7"), lipgloss.Color("#7D56F4")),
		SectionBorder: ld(lipgloss.Color("#999999"), lipgloss.Color("#626262")),
		ResultBorder:  ld(lipgloss.Color("#039960"), lipgloss.Color("#04B575")),
		StepNumber:    ld(lipgloss.Color("#D04040"), lipgloss.Color("#FF6B6B")),
		Title:         ld(lipgloss.Color("#1A1A1A"), lipgloss.Color("#FAFAFA")),
		Arrow:         ld(lipgloss.Color("#0070CC"), lipgloss.Color("#00BFFF")),
		DashedArrow:   ld(lipgloss.Color("#3070A0"), lipgloss.Color("#87CEEB")),
		Note:          ld(lipgloss.Color("#555555"), lipgloss.Color("#CCCCCC")),
		Ref:           ld(lipgloss.Color("#9A7B10"), lipgloss.Color("#D4A017")),
		Prompt:        ld(lipgloss.Color("#888888"), lipgloss.Color("#999999")),
		Success:       ld(lipgloss.Color("#039960"), lipgloss.Color("#04B575")),
		Error:         ld(lipgloss.Color("#CC2222"), lipgloss.Color("#FF4444")),
		Header:        ld(lipgloss.Color("#D04040"), lipgloss.Color("#FF6B6B")),
		Dim:           ld(lipgloss.Color("#999999"), lipgloss.Color("#888888")),
	}
}

// Renderer renders demo output using Lipgloss styled boxes.
type Renderer struct {
	Palette  Palette
	MaxWidth int           // hard cap on box width; 0 means 120
	Fraction float64       // fraction of terminal width to use; 0 means 0.80
	Delay    time.Duration // per-line scroll delay; 0 means 18ms, negative disables
}

// New creates a TUI Renderer with default settings.
func New() *Renderer {
	return &Renderer{
		Palette: DefaultPalette(),
	}
}

// termWidth returns the current terminal width, or 80 as fallback.
func termWidth() int {
	w, _, err := term.GetSize(os.Stdout.Fd())
	if err != nil || w <= 0 {
		return 80
	}
	return w
}

func (r *Renderer) width() int {
	frac := r.Fraction
	if frac <= 0 {
		frac = 0.80
	}
	maxW := r.MaxWidth
	if maxW <= 0 {
		maxW = 120
	}
	w := int(float64(termWidth()) * frac)
	if w > maxW {
		w = maxW
	}
	if w < 40 {
		w = 40
	}
	return w
}

// innerWidth returns the usable content width inside a bordered box.
func (r *Renderer) innerWidth() int {
	// Rounded border: 1 char each side + 1 padding each side = 4
	return r.width() - 4
}

// scrollDelay returns the per-line delay for smooth scrolling.
func (r *Renderer) scrollDelay() time.Duration {
	if r.Delay < 0 {
		return 0
	}
	if r.Delay == 0 {
		return 18 * time.Millisecond
	}
	return r.Delay
}

// smoothPrint writes a rendered block line-by-line with a short delay
// between lines to create a smooth scroll-in effect.
func (r *Renderer) smoothPrint(rendered string) {
	delay := r.scrollDelay()
	if delay == 0 {
		fmt.Println(rendered)
		return
	}
	lines := strings.Split(rendered, "\n")
	for i, line := range lines {
		fmt.Println(line)
		// Skip delay on the last line to avoid trailing pause.
		if i < len(lines)-1 {
			time.Sleep(delay)
		}
	}
}

func (r *Renderer) RenderHeader(title, description string, stepCount int) {
	p := r.Palette

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(p.Header).
		Align(lipgloss.Center).
		Width(r.innerWidth())

	descStyle := lipgloss.NewStyle().
		Foreground(p.Dim).
		Align(lipgloss.Center).
		Width(r.innerWidth())

	countStyle := lipgloss.NewStyle().
		Foreground(p.Dim).
		Align(lipgloss.Center).
		Width(r.innerWidth())

	var parts []string
	parts = append(parts, titleStyle.Render(title))
	if description != "" {
		parts = append(parts, descStyle.Render(description))
	}
	parts = append(parts, countStyle.Render(fmt.Sprintf("%d steps", stepCount)))

	content := lipgloss.JoinVertical(lipgloss.Center, parts...)

	box := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(p.Header).
		Padding(0, 1).
		Width(r.width())

	r.smoothPrint(box.Render(content))
	fmt.Println()
}

func (r *Renderer) RenderStep(stepNum, totalSteps int, step *demokit.StepDef) {
	p := r.Palette
	iw := r.innerWidth()

	// Step number badge
	numStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(p.StepNumber)
	badge := numStyle.Render(fmt.Sprintf("Step %d/%d", stepNum, totalSteps))

	// Title
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(p.Title)
	title := titleStyle.Render(step.Title())

	header := badge + "  " + title

	var sections []string
	sections = append(sections, header)

	// Refs
	if refs := step.Refs(); len(refs) > 0 {
		refStyle := lipgloss.NewStyle().Foreground(p.Ref)
		var refParts []string
		for _, ref := range refs {
			refParts = append(refParts, ref.Name)
		}
		sections = append(sections, refStyle.Render("Refs: "+strings.Join(refParts, ", ")))
	}

	// Arrows
	arrowStyle := lipgloss.NewStyle().Foreground(p.Arrow)
	dashedStyle := lipgloss.NewStyle().Foreground(p.DashedArrow)
	for _, a := range step.Arrows() {
		sym := "──>>"
		style := arrowStyle
		if a.Dashed {
			sym = "- ->>"
			style = dashedStyle
		}
		line := fmt.Sprintf("  %s %s %s : %s", a.From, sym, a.To, a.Label)
		sections = append(sections, style.Render(line))
	}

	// Note
	if note := step.NoteText(); note != "" {
		noteStyle := lipgloss.NewStyle().
			Italic(true).
			Foreground(p.Note).
			Width(iw)
		sections = append(sections, "")
		sections = append(sections, noteStyle.Render(note))
	}

	content := lipgloss.JoinVertical(lipgloss.Left, sections...)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(p.StepBorder).
		Padding(0, 1).
		Width(r.width())

	r.smoothPrint(box.Render(content))
}

func (r *Renderer) RenderResult(stepNum int, output string, err error) {
	p := r.Palette
	output = strings.TrimRight(output, "\n")

	if output == "" && err == nil {
		fmt.Println()
		return
	}

	label := lipgloss.NewStyle().
		Bold(true).
		Foreground(p.Success).
		Render("Result")

	var body string
	if err != nil {
		errStyle := lipgloss.NewStyle().Foreground(p.Error)
		body = errStyle.Render(fmt.Sprintf("(capture error: %v)", err))
		if output != "" {
			body += "\n" + output
		}
	} else {
		body = output
	}

	content := label + "\n" + body

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(p.ResultBorder).
		Padding(0, 1).
		Width(r.width())

	r.smoothPrint(box.Render(content))
	fmt.Println()
}

func (r *Renderer) RenderSection(section *demokit.SectionDef) {
	p := r.Palette
	iw := r.innerWidth()

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(p.Title).
		Width(iw)

	bodyStyle := lipgloss.NewStyle().
		Foreground(p.Note).
		Width(iw)

	var parts []string
	parts = append(parts, titleStyle.Render(section.Title()))
	if body := section.Body(); body != "" {
		parts = append(parts, bodyStyle.Render(body))
	}

	content := lipgloss.JoinVertical(lipgloss.Left, parts...)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(p.SectionBorder).
		Padding(0, 1).
		Width(r.width())

	r.smoothPrint(box.Render(content))
	fmt.Println()
}

func (r *Renderer) RenderDone() {
	p := r.Palette
	style := lipgloss.NewStyle().
		Bold(true).
		Foreground(p.Success).
		Align(lipgloss.Center).
		Width(r.innerWidth())

	box := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(p.Success).
		Padding(0, 1).
		Width(r.width())

	r.smoothPrint(box.Render(style.Render("Done")))
}

func (r *Renderer) WaitForStep() {
	p := r.Palette
	style := lipgloss.NewStyle().
		Foreground(p.Prompt).
		Italic(true)
	fmt.Println(style.Render("  Press Enter to run this step..."))
	bufio.NewReader(os.Stdin).ReadString('\n')
}
