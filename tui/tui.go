// Package tui provides a Lipgloss-styled renderer for demokit demos.
// Steps, sections, and results are rendered in visually distinct bordered
// boxes with differentiated styling for titles, arrows, notes, and refs.
package tui

import (
	"bufio"
	"fmt"
	"image/color"
	"io"
	"os"
	"strings"
	"time"

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
	Warning       color.Color
	Info          color.Color
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
		Warning:       ld(lipgloss.Color("#B8860B"), lipgloss.Color("#FFD700")),
		Info:          ld(lipgloss.Color("#0070CC"), lipgloss.Color("#00BFFF")),
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
	prompter FormPrompter

	// lastStep is set by RenderStep and consulted by WaitForStep so the
	// line-based copy prompt knows which verbatim blocks the user can
	// reference. Cleared at RenderResult so a between-step Enter pause
	// doesn't pick up the previous step's blocks if the next step has
	// none. v1.1 raw-mode interaction will replace this with a real
	// focus model.
	lastStep *demokit.StepDef
}

// New creates a TUI Renderer with default settings.
func New() *Renderer {
	return &Renderer{
		Palette: DefaultPalette(),
	}
}

// WithPrompter installs a custom FormPrompter for collecting step
// inputs. If unset, the default ReadlinePrompter (sequential readline
// with sticky-on-retry defaults) is used.
func (r *Renderer) WithPrompter(p FormPrompter) *Renderer {
	r.prompter = p
	return r
}

// activePrompter returns the configured FormPrompter, lazily creating
// the default ReadlinePrompter on first access.
func (r *Renderer) activePrompter() FormPrompter {
	if r.prompter == nil {
		r.prompter = &ReadlinePrompter{
			PromptColor: r.Palette.Prompt,
			ErrorColor:  r.Palette.Error,
		}
	}
	return r.prompter
}

// termWidth returns the current terminal width via the shared demokit helper.
func termWidth() int {
	return demokit.TermWidth()
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
	r.lastStep = step
	p := r.Palette
	iw := r.innerWidth()

	// Step number badge
	numStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(p.StepNumber)
	// Once visit count exceeds the declared step total (cyclic graph),
	// the "N/M" denominator becomes misleading — drop it.
	stepLabel := fmt.Sprintf("Step %d/%d", stepNum, totalSteps)
	if stepNum > totalSteps {
		stepLabel = fmt.Sprintf("Step %d", stepNum)
	}
	badge := numStyle.Render(stepLabel)

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

	r.renderVerbatimBlocks(step)
}

// renderVerbatimBlocks emits each verbatim block according to the demo's
// boxing mode + per-block variant count:
//
//   - Single-variant + Demo.IsBoxedVerbatim() unset → today's behavior:
//     printed OUTSIDE the bordered box so lipgloss never soft-wraps long
//     lines into the box border, preserving triple-click copy.
//   - Single-variant + Demo.IsBoxedVerbatim() set → rendered inside a
//     styled box; keyboard copy via the pause prompt.
//   - Multi-variant (always boxed regardless of the flag) → rendered
//     inside a styled box with each variant stacked under its **label**;
//     keyboard copy via `c <label>` at the pause prompt.
func (r *Renderer) renderVerbatimBlocks(step *demokit.StepDef) {
	p := r.Palette
	blocks := step.VerbatimBlocks()
	if len(blocks) == 0 {
		return
	}
	demo := step.Demo()
	boxedDefault := demo != nil && demo.IsBoxedVerbatim()
	labelStyle := lipgloss.NewStyle().Italic(true).Foreground(p.Note)
	variantLabelStyle := lipgloss.NewStyle().Bold(true).Foreground(p.Note)

	for _, v := range blocks {
		fmt.Println()
		multi := len(v.Variants) > 1
		boxed := boxedDefault || multi
		if !boxed {
			if v.Label != "" {
				fmt.Println(labelStyle.Render(v.Label))
			}
			r.smoothPrint(strings.TrimRight(v.Variants[0].Content, "\n"))
			continue
		}

		// Boxed render. Single-variant: snippet inside the box.
		// Multi-variant: stacked variants, each under its bold label.
		var sections []string
		if v.Label != "" {
			sections = append(sections, labelStyle.Render(v.Label))
		}
		for i, va := range v.Variants {
			if multi {
				if i > 0 {
					sections = append(sections, "")
				}
				if va.Label != "" {
					sections = append(sections, variantLabelStyle.Render(va.Label))
				}
			}
			sections = append(sections, strings.TrimRight(va.Content, "\n"))
		}
		content := lipgloss.JoinVertical(lipgloss.Left, sections...)
		box := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(p.Note).
			Padding(0, 1).
			Width(r.width())
		r.smoothPrint(box.Render(content))
	}
	fmt.Println()
}

// statusColors returns the border and label colors for a given result status.
func (r *Renderer) statusColors(status demokit.ResultStatus) (border, label color.Color) {
	p := r.Palette
	switch status {
	case demokit.StatusError:
		return p.Error, p.Error
	case demokit.StatusWarning:
		return p.Warning, p.Warning
	case demokit.StatusInfo:
		return p.Info, p.Info
	default:
		return p.ResultBorder, p.Success
	}
}

// StreamOutput writes a chunk of step output as the step's Run is
// still executing. demokit's Execute loop dispatches here when the
// renderer implements StreamingRenderer; the resulting RenderResult
// call is then handed an empty output so the body isn't double-printed.
//
// out is the writer to emit chunks to (the user's actual stdout,
// captured before the redirect into demokit's capture pipe).
//
// Phase-1 implementation writes chunks raw between the step header
// and the eventual styled status box. A future enhancement would
// track an in-progress box and redraw it via cursor rewind on each
// chunk for a live-growing-box effect; that's deferred.
func (r *Renderer) StreamOutput(_ int, chunk []byte, out io.Writer) {
	out.Write(chunk)
}

func (r *Renderer) RenderResult(stepNum int, output string, result *demokit.StepResult) {
	output = strings.TrimRight(output, "\n")

	// Nothing to show
	if output == "" && result == nil {
		fmt.Println()
		return
	}

	// Determine status
	status := demokit.StatusSuccess
	if result != nil {
		status = result.Status
	}
	borderColor, labelColor := r.statusColors(status)

	// Label
	displayLabel := "Result"
	if result != nil {
		displayLabel = result.DisplayLabel()
	}
	label := lipgloss.NewStyle().
		Bold(true).
		Foreground(labelColor).
		Render(displayLabel)

	// Build body
	var bodyParts []string

	// Message (error text, warning, info note)
	if result != nil && result.Message != "" {
		msgStyle := lipgloss.NewStyle().Foreground(labelColor)
		bodyParts = append(bodyParts, msgStyle.Render(result.Message))
	}

	// Captured stdout
	if output != "" {
		bodyParts = append(bodyParts, output)
	}

	content := label
	if len(bodyParts) > 0 {
		content += "\n" + strings.Join(bodyParts, "\n")
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
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

func (r *Renderer) WaitForStep(opts demokit.WaitOpts) {
	p := r.Palette
	style := lipgloss.NewStyle().
		Foreground(p.Prompt).
		Italic(true)

	// AutoAccept countdown owns its own input path (cancelreader-backed
	// race against the timer). Wiring copy into it would require a
	// raw-mode key dispatcher, which is deferred to v1.1. Steps that
	// need copy must run without AutoAccept (the common case for
	// interactive demos that show snippets).
	if opts.AutoAcceptAfter > 0 {
		if !opts.ShowCountdown {
			fmt.Println(style.Render(fmt.Sprintf("  Press Enter to run (auto in %s)...",
				opts.AutoAcceptAfter.Round(time.Second))))
			demokit.WaitForEnterOrTimeout(opts.AutoAcceptAfter, nil)
			return
		}
		demokit.WaitForEnterOrTimeout(opts.AutoAcceptAfter, func(remaining time.Duration) {
			bar := plainCountdownBar(remaining, opts.AutoAcceptAfter, 20)
			line := fmt.Sprintf("  %s  %4.1fs  (Enter to accept)", bar, remaining.Seconds())
			fmt.Print("\r" + style.Render(line))
		})
		fmt.Print("\r" + strings.Repeat(" ", 70) + "\r")
		return
	}

	// No countdown — line-based pause loop with optional copy command
	// when the rendered step has boxed/multi-variant verbatim blocks
	// the user can grab via the clipboard primitive.
	copyables := r.copyableBlocks(r.lastStep)
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Println(style.Render(copyPromptHint(copyables)))
		line, _ := reader.ReadString('\n')
		cmd := strings.TrimSpace(line)
		if cmd == "" {
			return
		}
		if msg := r.handleCopyCommand(cmd, copyables); msg != "" {
			noteStyle := lipgloss.NewStyle().Foreground(p.Note).Italic(true)
			fmt.Println(noteStyle.Render("  " + msg))
		}
	}
}

// Prompt delegates to the renderer's FormPrompter (default
// ReadlinePrompter). Customize via Renderer.WithPrompter.
func (r *Renderer) Prompt(stepID string, inputs []demokit.InputDef) map[string]any {
	return r.activePrompter().Prompt(stepID, inputs)
}

func plainCountdownBar(remaining, total time.Duration, width int) string {
	if total <= 0 {
		return strings.Repeat(" ", width)
	}
	filled := int(float64(width) * float64(remaining) / float64(total))
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}
	return "[" + strings.Repeat("█", filled) + strings.Repeat("░", width-filled) + "]"
}
