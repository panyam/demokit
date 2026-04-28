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

	"charm.land/lipgloss/v2"
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

// DefaultPalette returns the default color palette.
func DefaultPalette() Palette {
	return Palette{
		StepBorder:    lipgloss.Color("#7D56F4"),
		SectionBorder: lipgloss.Color("#626262"),
		ResultBorder:  lipgloss.Color("#04B575"),
		StepNumber:    lipgloss.Color("#FF6B6B"),
		Title:         lipgloss.Color("#FAFAFA"),
		Arrow:         lipgloss.Color("#00BFFF"),
		DashedArrow:   lipgloss.Color("#87CEEB"),
		Note:          lipgloss.Color("#A8A8A8"),
		Ref:           lipgloss.Color("#D4A017"),
		Prompt:        lipgloss.Color("#626262"),
		Success:       lipgloss.Color("#04B575"),
		Error:         lipgloss.Color("#FF4444"),
		Header:        lipgloss.Color("#FF6B6B"),
		Dim:           lipgloss.Color("#626262"),
	}
}

// Renderer renders demo output using Lipgloss styled boxes.
type Renderer struct {
	Palette Palette
	Width   int // box width; 0 means auto (72)
}

// New creates a TUI Renderer with default settings.
func New() *Renderer {
	return &Renderer{
		Palette: DefaultPalette(),
		Width:   72,
	}
}

func (r *Renderer) width() int {
	if r.Width <= 0 {
		return 72
	}
	return r.Width
}

// innerWidth returns the usable content width inside a bordered box.
func (r *Renderer) innerWidth() int {
	// Rounded border: 1 char each side + 1 padding each side = 4
	return r.width() - 4
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

	fmt.Println(box.Render(content))
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

	fmt.Println(box.Render(content))
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

	fmt.Println(box.Render(content))
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

	fmt.Println(box.Render(content))
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

	fmt.Println(box.Render(style.Render("Done")))
}

func (r *Renderer) WaitForStep() {
	p := r.Palette
	style := lipgloss.NewStyle().
		Foreground(p.Prompt).
		Italic(true)
	fmt.Println(style.Render("  Press Enter to run this step..."))
	bufio.NewReader(os.Stdin).ReadString('\n')
}
