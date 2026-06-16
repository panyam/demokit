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
	"sync"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/term"
	"github.com/panyam/demokit"
	"github.com/panyam/demokit/events"
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
	Palette     Palette
	MaxWidth    int                 // hard cap on box width; 0 means 120
	Fraction    float64             // fraction of terminal width to use; 0 means 0.80
	Delay       time.Duration       // per-line scroll delay; 0 means 18ms, negative disables
	borderStyle demokit.BorderStyle // per-side toggles for verbatim + result boxes; see WithBorderStyle
	borderChars demokit.BorderChars // glyphs for the border; zero value = lipgloss.RoundedBorder(); see WithBorderChars
	prompter    FormPrompter

	// activeVariant maps a block's index within the current step to
	// the currently-active variant index within that block. Initial
	// values seed from each block's Default-marked variant (or 0 if
	// none is marked). The line-based pause loop mutates this when
	// the user types a switch command; bare `c` then copies whichever
	// variant is active. Reset at each StepStart so a fresh step's
	// defaults take over.
	activeVariant map[int]int

	// --- event drain state ---
	//
	// When Execute attaches its queue via AttachEventQueue, a
	// drain goroutine dispatches every event (Header, StepStart,
	// OutputChunk, StepEnd, Section, Done, sync waits/prompts) into
	// the printX helpers + the stdout snapshot below. Snapshots of
	// os.Stdout/Stderr are pinned at attach time so the drain
	// never reads the live globals (race-free against
	// captureOutput's redirect, per issue 23).
	queue      *events.EventQueue
	drainDone  bool
	drainWG    sync.WaitGroup
	boxedFlag  bool              // mirror of events.Header.BoxedVerbatim
	lastStepEv *events.StepStart // most recent StepStart, for the copy prompt
	snapOut    *os.File
	snapErr    *os.File
}

// stdoutFor returns the writer the drain should write to. Uses
// the snapshot taken at AttachEventQueue when present, falling
// back to live os.Stdout for tests that drive printX helpers
// directly without going through Execute. Same pattern as
// PlainRenderer's snap helpers.
func (r *Renderer) stdoutFor() io.Writer {
	if r.snapOut != nil {
		return r.snapOut
	}
	return os.Stdout
}

func (r *Renderer) stdoutFile() *os.File {
	if r.snapOut != nil {
		return r.snapOut
	}
	return os.Stdout
}

func (r *Renderer) stderrFile() *os.File {
	if r.snapErr != nil {
		return r.snapErr
	}
	return os.Stderr
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

// WithBorderStyle configures which sides of the verbatim and result
// boxes draw border lines. Header, step, section, and done boxes
// always use their default rounded/double borders — they have no
// copy-relevant content, so per-side toggling there is noise.
//
// BorderDefault preserves today's all-sides rounded look.
// BorderHorizontalOnly is the copy-paste-friendly choice for
// walkthroughs whose readers mouse-select snippets out of multi-
// variant verbatim blocks.
func (r *Renderer) WithBorderStyle(s demokit.BorderStyle) *Renderer {
	r.borderStyle = s
	return r
}

// WithBorderChars configures which characters the verbatim and
// result borders use. The zero BorderChars{} value (the default)
// means "use lipgloss.RoundedBorder()" — today's look. Apply a
// preset (demokit.BorderCharsDouble, .Thick, .ASCII, .Normal) or
// supply a struct literal with custom glyphs.
//
// Composes with WithBorderStyle: chars say what glyphs to use,
// style says which sides those glyphs draw on.
func (r *Renderer) WithBorderChars(bc demokit.BorderChars) *Renderer {
	r.borderChars = bc
	return r
}

// verbatimBorder returns the lipgloss.Border the verbatim/result
// boxes should draw with. Honors r.borderChars when non-zero;
// otherwise falls back to lipgloss.RoundedBorder() so today's
// look is preserved for callers who don't opt in.
func (r *Renderer) verbatimBorder() lipgloss.Border {
	if r.borderChars.IsZero() {
		return lipgloss.RoundedBorder()
	}
	return lipgloss.Border{
		Top:         r.borderChars.Top,
		Bottom:      r.borderChars.Bottom,
		Left:        r.borderChars.Left,
		Right:       r.borderChars.Right,
		TopLeft:     r.borderChars.TopLeft,
		TopRight:    r.borderChars.TopRight,
		BottomLeft:  r.borderChars.BottomLeft,
		BottomRight: r.borderChars.BottomRight,
	}
}

// applyBorderStyle takes a base lipgloss.Style and returns it with
// per-side toggles applied. Resolution order:
//
//  1. An explicit r.borderStyle (Full / HorizontalOnly / None)
//     wins outright — the chars don't get a say.
//  2. r.borderStyle == BorderDefault AND r.borderChars is non-zero:
//     infer sides from the chars. Empty char fields → that side off.
//     Lets one-call use of `WithBorderChars(BorderCharsRoundedH)`
//     produce a horizontal-only box without a companion
//     WithBorderStyle.
//  3. Neither set: pass through unchanged. The base style's
//     Border(...) (set by the caller) decides.
//
// Called at the verbatim and result draw sites only — header,
// step, section, done boxes skip this and keep their defaults.
func (r *Renderer) applyBorderStyle(s lipgloss.Style) lipgloss.Style {
	switch r.borderStyle {
	case demokit.BorderFull:
		return s.BorderTop(true).BorderRight(true).BorderBottom(true).BorderLeft(true)
	case demokit.BorderHorizontalOnly:
		return s.BorderTop(true).BorderRight(false).BorderBottom(true).BorderLeft(false)
	case demokit.BorderNone:
		return s.BorderTop(false).BorderRight(false).BorderBottom(false).BorderLeft(false)
	}
	if r.borderChars.IsZero() {
		return s
	}
	return s.
		BorderTop(r.borderChars.Top != "").
		BorderRight(r.borderChars.Right != "").
		BorderBottom(r.borderChars.Bottom != "").
		BorderLeft(r.borderChars.Left != "")
}

// borderSides is the resolved per-side draw decision for a verbatim
// or result box. Computed once via effectiveSides() so the box vs.
// raw render branching uses the same logic that applyBorderStyle
// uses to drive lipgloss.
type borderSides struct{ top, right, bottom, left bool }

// hasSide reports whether either left or right side is drawn. When
// false, the box has no visible frame columns and content can be
// emitted raw (no lipgloss.Width()/Padding()) so long lines stay
// byte-exact for copy-paste.
func (s borderSides) hasSide() bool { return s.left || s.right }

// effectiveSides resolves r.borderStyle + r.borderChars into the
// per-side draw decision, mirroring applyBorderStyle's resolution
// order. Used by renderBoxOrRaw to decide between the lipgloss box
// path (any side drawn) and the raw-content-plus-rules path (no
// sides drawn — copy-paste friendly).
func (r *Renderer) effectiveSides() borderSides {
	switch r.borderStyle {
	case demokit.BorderFull:
		return borderSides{true, true, true, true}
	case demokit.BorderHorizontalOnly:
		return borderSides{top: true, bottom: true}
	case demokit.BorderNone:
		return borderSides{}
	}
	if r.borderChars.IsZero() {
		return borderSides{true, true, true, true}
	}
	return borderSides{
		top:    r.borderChars.Top != "",
		right:  r.borderChars.Right != "",
		bottom: r.borderChars.Bottom != "",
		left:   r.borderChars.Left != "",
	}
}

// horizontalRule returns a single-line rule string for the raw
// (no-side) render path, in the requested border color. `side` is
// "top" or "bottom" and selects the glyph from r.borderChars when
// set; otherwise falls back to `─` (matching the default rounded
// border's Top/Bottom char). Width is r.width() so the rule visually
// aligns with the renderer's other framing.
func (r *Renderer) horizontalRule(side string, fg color.Color) string {
	ch := ""
	if !r.borderChars.IsZero() {
		switch side {
		case "top":
			ch = r.borderChars.Top
		case "bottom":
			ch = r.borderChars.Bottom
		}
	}
	if ch == "" {
		ch = "─"
	}
	return lipgloss.NewStyle().Foreground(fg).Render(strings.Repeat(ch, r.width()))
}

// renderBoxOrRaw is the shared draw path for verbatim and result
// content. When the active border has any side (left/right) drawn,
// it renders inside a lipgloss box of width r.width() with the
// standard Padding(0, 1). When neither side is drawn, content is
// emitted raw via smoothPrint — no Width(), no Padding — flanked by
// optional top/bottom horizontal rules. The raw path preserves
// byte-exact content so long lines (curl payloads, JSON blobs)
// survive copy-paste; explicit \n in `content` still produces line
// breaks. fg is the border color (Foreground on the rules,
// BorderForeground on the lipgloss box).
func (r *Renderer) renderBoxOrRaw(content string, fg color.Color) {
	content = strings.TrimRight(content, "\n")
	sides := r.effectiveSides()
	if sides.hasSide() {
		box := r.applyBorderStyle(lipgloss.NewStyle().
			Border(r.verbatimBorder()).
			BorderForeground(fg).
			Padding(0, 1).
			Width(r.width()))
		r.smoothPrint(box.Render(content))
		return
	}
	if sides.top {
		r.smoothPrint(r.horizontalRule("top", fg))
	}
	r.smoothPrint(content)
	if sides.bottom {
		r.smoothPrint(r.horizontalRule("bottom", fg))
	}
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

// terminalWidth queries the terminal width through the renderer's
// pinned snapshot file handles, 80 fallback. The snapshots are
// taken at AttachEventQueue time so no extra lock is needed.
// Before the snapshot is set (tests that construct a Renderer
// without attaching to a queue) falls back to live os.Stdout via
// demokit.TermWidth.
func (r *Renderer) terminalWidth() int {
	if r.snapOut != nil || r.snapErr != nil {
		for _, f := range []*os.File{r.stdoutFile(), r.stderrFile()} {
			if w, _, err := term.GetSize(f.Fd()); err == nil && w > 0 {
				return w
			}
		}
		return 80
	}
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
	w := int(float64(r.terminalWidth()) * frac)
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
		fmt.Fprintln(r.stdoutFor(), rendered)
		return
	}
	lines := strings.Split(rendered, "\n")
	for i, line := range lines {
		fmt.Fprintln(r.stdoutFor(), line)
		// Skip delay on the last line to avoid trailing pause.
		if i < len(lines)-1 {
			time.Sleep(delay)
		}
	}
}

// printHeaderBlock formats and emits the demo header. Called by
// the event drain on Header.
func (r *Renderer) printHeaderBlock(title, description string, stepCount int) {
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
	fmt.Fprintln(r.stdoutFor())
}

// printStepBlock is the shared formatter the event drain calls.
// boxedDefault comes from the Header.BoxedVerbatim flag the drain
// caches on the renderer.
func (r *Renderer) printStepBlock(stepNum, totalSteps int, e events.StepStart, boxedDefault bool) {
	r.activeVariant = initialActiveVariantsFromVerbatims(e.Verbatims)
	p := r.Palette
	iw := r.innerWidth()

	numStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(p.StepNumber)
	stepLabel := fmt.Sprintf("Step %d/%d", stepNum, totalSteps)
	if stepNum > totalSteps {
		stepLabel = fmt.Sprintf("Step %d", stepNum)
	}
	badge := numStyle.Render(stepLabel)

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(p.Title)
	title := titleStyle.Render(e.Title)

	header := badge + "  " + title

	var sections []string
	sections = append(sections, header)

	if len(e.Refs) > 0 {
		refStyle := lipgloss.NewStyle().Foreground(p.Ref)
		refParts := make([]string, 0, len(e.Refs))
		for _, ref := range e.Refs {
			refParts = append(refParts, ref.Name)
		}
		sections = append(sections, refStyle.Render("Refs: "+strings.Join(refParts, ", ")))
	}

	arrowStyle := lipgloss.NewStyle().Foreground(p.Arrow)
	dashedStyle := lipgloss.NewStyle().Foreground(p.DashedArrow)
	for _, a := range e.Arrows {
		sym := "──>>"
		style := arrowStyle
		if a.Dashed {
			sym = "- ->>"
			style = dashedStyle
		}
		line := fmt.Sprintf("  %s %s %s : %s", a.From, sym, a.To, a.Label)
		sections = append(sections, style.Render(line))
	}

	if e.Note != "" {
		noteStyle := lipgloss.NewStyle().
			Italic(true).
			Foreground(p.Note).
			Width(iw)
		sections = append(sections, "")
		sections = append(sections, noteStyle.Render(e.Note))
	}

	content := lipgloss.JoinVertical(lipgloss.Left, sections...)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(p.StepBorder).
		Padding(0, 1).
		Width(r.width())

	r.smoothPrint(box.Render(content))

	r.printVerbatimBlocks(e.Verbatims, boxedDefault)
}

// renderVerbatimBlocks emits each verbatim block according to the
// demo's boxing mode + per-block variant count:
//
//   - Single-variant + Demo.IsBoxedVerbatim() unset → today's behavior:
//     printed OUTSIDE the bordered box so lipgloss never soft-wraps
//     long lines into the box border, preserving triple-click copy.
//   - Single-variant + Demo.IsBoxedVerbatim() set → rendered inside a
//     styled box; keyboard copy via the pause prompt.
//   - Multi-variant (always boxed regardless of the flag) → tab strip
//     above (`<active>  other  other`), box below showing only the
//     active variant. Default-marked variant starts active; user
//     switches via line-input command at the pause; the new active
//     variant is echoed inline.
//
// printVerbatimBlocks is the shared formatter the event drain also
// calls. Takes the projected event vocabulary so the drain doesn't
// need to reconstruct *StepDef. Body matches renderVerbatimBlocks
// modulo the projection types (events.Verbatim has identical shape
// to demokit.VerbatimView; renderUnboxedVariant / renderBoxedBlock
// still take the legacy view types so we convert at the block edge).
func (r *Renderer) printVerbatimBlocks(blocks []events.Verbatim, boxedDefault bool) {
	if len(blocks) == 0 {
		return
	}
	for idx, v := range blocks {
		fmt.Fprintln(r.stdoutFor())
		multi := len(v.Variants) > 1
		boxed := boxedDefault || multi
		view := verbatimEventToView(v)
		if !boxed {
			r.renderUnboxedVariant(view)
			continue
		}
		r.renderBoxedBlock(idx, view, multi)
	}
	fmt.Fprintln(r.stdoutFor())
}

// renderUnboxedVariant emits a single-variant block in today's
// outside-the-box style — italic label line then the raw content.
// Preserves triple-click copy semantics for users on terminals
// without OSC 52.
func (r *Renderer) renderUnboxedVariant(v demokit.VerbatimView) {
	labelStyle := lipgloss.NewStyle().Italic(true).Foreground(r.Palette.Note)
	if v.Label != "" {
		fmt.Fprintln(r.stdoutFor(), labelStyle.Render(v.Label))
	}
	r.smoothPrint(strings.TrimRight(v.Variants[0].Content, "\n"))
}

// renderBoxedBlock emits a verbatim block inside a styled box. For
// multi-variant blocks a tab strip is rendered above the box and only
// the currently-active variant's content appears inside. blockIdx is
// the block's index within the step; the renderer reads
// r.activeVariant[blockIdx] to decide which variant is active.
//
// The outer block label is bold + Title color so it stands out from
// the dim variant tabs (the previous italic Note color was too close
// to the inactive-tab Dim color). A blank line after the label gives
// the tab strip / box breathing room.
func (r *Renderer) renderBoxedBlock(blockIdx int, v demokit.VerbatimView, multi bool) {
	p := r.Palette
	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(p.Title)
	if v.Label != "" {
		fmt.Fprintln(r.stdoutFor(), labelStyle.Render(v.Label))
		fmt.Fprintln(r.stdoutFor())
	}
	if multi {
		fmt.Fprintln(r.stdoutFor(), r.renderTabStrip(v.Variants, r.activeIndex(blockIdx)))
	}
	active := v.Variants[r.activeIndex(blockIdx)]
	r.renderBoxOrRaw(active.Content, p.Note)
}

// renderTabStrip formats the per-variant tabs above a multi-variant
// box. The active variant is wrapped in angle brackets and bolded;
// others are dim. Spacing: two spaces between entries. Default-marked
// variant gets a "(default)" trailing tag so the user knows what bare
// `c` will copy when the active is reset.
//
// Each tab is prefixed with its 1-based index ("1. curl") so the
// `<n> to switch` affordance shown in the pause prompt has a visual
// anchor on the tab itself.
func (r *Renderer) renderTabStrip(variants []demokit.VariantView, activeIdx int) string {
	active := lipgloss.NewStyle().Bold(true).Foreground(r.Palette.Header)
	dim := lipgloss.NewStyle().Foreground(r.Palette.Dim)
	parts := make([]string, len(variants))
	for i, v := range variants {
		label := v.Label
		if label == "" {
			label = fmt.Sprintf("variant %d", i+1)
		}
		if v.IsDefault {
			label += " (default)"
		}
		label = fmt.Sprintf("%d. %s", i+1, label)
		if i == activeIdx {
			parts[i] = active.Render("<" + label + ">")
		} else {
			parts[i] = dim.Render(" " + label + " ")
		}
	}
	return strings.Join(parts, "  ")
}

// activeIndex returns the currently-active variant index for the
// block at blockIdx within the current step. Falls back to 0 when
// the state map hasn't been seeded (e.g. tests that construct a
// Renderer without going through RenderStep).
func (r *Renderer) activeIndex(blockIdx int) int {
	if r.activeVariant == nil {
		return 0
	}
	return r.activeVariant[blockIdx]
}

// initialActiveVariantsFromVerbatims computes the starting
// active-variant index for each block on a step: the Default-marked
// variant if any, otherwise the first. Returns a fresh map so the
// previous step's state doesn't leak. Called by the drain on
// StepStart and by tests that prep a Renderer with seeded state.
func initialActiveVariantsFromVerbatims(blocks []events.Verbatim) map[int]int {
	out := map[int]int{}
	for i, v := range blocks {
		for j, va := range v.Variants {
			if va.IsDefault {
				out[i] = j
				break
			}
		}
	}
	return out
}

// --- event projection <-> demokit-view converters ---
//
// events.Ref/Arrow/Variant/Verbatim have identical field shapes to
// demokit.Ref/ArrowView/VariantView/VerbatimView. These tiny copy
// helpers let the drain hand block-rendering helpers (which still
// take the legacy view types) the projected event data, and let
// tests project a *StepDef into a StepStart event for printStepBlock.

func refsToEvents(in []demokit.Ref) []events.Ref {
	if len(in) == 0 {
		return nil
	}
	out := make([]events.Ref, len(in))
	for i, r := range in {
		out[i] = events.Ref{Name: r.Name, URL: r.URL}
	}
	return out
}

func arrowsToEvents(in []demokit.ArrowView) []events.Arrow {
	if len(in) == 0 {
		return nil
	}
	out := make([]events.Arrow, len(in))
	for i, a := range in {
		out[i] = events.Arrow{From: a.From, To: a.To, Label: a.Label, Dashed: a.Dashed}
	}
	return out
}

func verbatimsToEventsTUI(in []demokit.VerbatimView) []events.Verbatim {
	if len(in) == 0 {
		return nil
	}
	out := make([]events.Verbatim, len(in))
	for i, vb := range in {
		variants := make([]events.Variant, len(vb.Variants))
		for j, v := range vb.Variants {
			variants[j] = events.Variant{
				Label: v.Label, Lang: v.Lang, Content: v.Content, IsDefault: v.IsDefault,
			}
		}
		out[i] = events.Verbatim{Label: vb.Label, Variants: variants}
	}
	return out
}

func verbatimEventToView(e events.Verbatim) demokit.VerbatimView {
	variants := make([]demokit.VariantView, len(e.Variants))
	for i, v := range e.Variants {
		variants[i] = demokit.VariantView{
			Label: v.Label, Lang: v.Lang, Content: v.Content, IsDefault: v.IsDefault,
		}
	}
	return demokit.VerbatimView{Label: e.Label, Variants: variants}
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

// printResultBlock formats the post-Run result box. Called by the
// event drain on StepEnd; the output argument is always "" along
// the event-driven path (chunks arrived via OutputChunk).
func (r *Renderer) printResultBlock(_ int, output string, result *demokit.StepResult) {
	output = strings.TrimRight(output, "\n")

	// Nothing to show
	if output == "" && result == nil {
		fmt.Fprintln(r.stdoutFor())
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

	r.renderBoxOrRaw(content, borderColor)
	fmt.Fprintln(r.stdoutFor())
}

// printSectionBlock is the shared formatter the event drain calls.
func (r *Renderer) printSectionBlock(title, body string) {
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
	parts = append(parts, titleStyle.Render(title))
	if body != "" {
		parts = append(parts, bodyStyle.Render(body))
	}

	content := lipgloss.JoinVertical(lipgloss.Left, parts...)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(p.SectionBorder).
		Padding(0, 1).
		Width(r.width())

	r.smoothPrint(box.Render(content))
	fmt.Fprintln(r.stdoutFor())
}

// printDoneBlock prints the completion marker. Called by the
// event drain on Done.
func (r *Renderer) printDoneBlock() {
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

// waitForAdvanceUI is the interactive-pause loop the event drain
// calls (with copyables computed from r.lastStepEv).
func (r *Renderer) waitForAdvanceUI(opts demokit.WaitOpts, copyables []copyableBlock) {
	p := r.Palette
	style := lipgloss.NewStyle().
		Foreground(p.Prompt).
		Italic(true)

	if opts.AutoAcceptAfter > 0 {
		r.waitWithCountdown(opts, copyables, style)
		return
	}
	if len(copyables) > 0 {
		r.copyPromptLoop(copyables, style)
		return
	}
	fmt.Fprintln(r.stdoutFor(), style.Render("  Press Enter to run this step..."))
	bufio.NewReader(os.Stdin).ReadString('\n')
}

// waitWithCountdown runs the auto-accept countdown with a universal
// "any key to review" escape. The countdown read is in raw mode so a
// single keypress (not just Enter) interrupts:
//
//   - Enter (KeyEnter or '\n') → accept and advance.
//   - Any other key            → drop into the line-based interactive
//     hold (copy loop for copyable steps,
//     plain Enter wait otherwise).
//   - Timer fires              → auto-advance.
//
// On terminals where raw mode is unavailable, WaitForKeyOrTimeout
// falls back to line mode and the user has to press Enter to
// interrupt — degraded but still functional.
func (r *Renderer) waitWithCountdown(opts demokit.WaitOpts, copyables []copyableBlock, promptStyle lipgloss.Style) {
	p := r.Palette
	hint := "Enter accept · any key to review"

	var key byte
	var gotKey bool
	if !opts.ShowCountdown {
		fmt.Fprintln(r.stdoutFor(), promptStyle.Render(fmt.Sprintf("  Press Enter to run (auto in %s) · any key to review",
			opts.AutoAcceptAfter.Round(time.Second))))
		key, gotKey = demokit.WaitForKeyOrTimeout(opts.AutoAcceptAfter, nil)
	} else {
		key, gotKey = demokit.WaitForKeyOrTimeout(opts.AutoAcceptAfter, func(remaining time.Duration) {
			bar := plainCountdownBar(remaining, opts.AutoAcceptAfter, 20)
			row := fmt.Sprintf("  %s  %4.1fs  (%s)", bar, remaining.Seconds(), hint)
			fmt.Fprint(r.stdoutFor(), "\r"+promptStyle.Render(row))
		})
		fmt.Fprint(r.stdoutFor(), "\r"+strings.Repeat(" ", 80)+"\r")
	}

	if !gotKey {
		return // timer fired — auto-advance
	}
	if key == demokit.KeyEnter || key == '\n' {
		return // Enter — accept and advance
	}

	// Any other key cancels the countdown. Drop into the interactive
	// hold appropriate for the step. Raw-mode terminal is already
	// restored by WaitForKeyOrTimeout, so cooked-mode line input
	// (bufio + ReadString) works correctly below.
	noteStyle := lipgloss.NewStyle().Foreground(p.Note).Italic(true)
	if len(copyables) > 0 {
		r.copyPromptLoop(copyables, promptStyle)
		return
	}
	fmt.Fprintln(r.stdoutFor(), noteStyle.Render("  (countdown stopped — press Enter to continue)"))
	bufio.NewReader(os.Stdin).ReadString('\n')
}

// copyPromptLoop runs the line-based pause for steps that have
// copyable verbatim blocks. Empty input continues; `c` / `c <label>`
// copy; `<label>` or `<n>` switches the active variant (and echoes
// the new active inline so the user can see what they're about to
// copy). Loops until empty Enter so users can stack multiple actions.
func (r *Renderer) copyPromptLoop(copyables []copyableBlock, promptStyle lipgloss.Style) {
	p := r.Palette
	reader := bufio.NewReader(os.Stdin)
	noteStyle := lipgloss.NewStyle().Foreground(p.Note).Italic(true)
	for {
		fmt.Fprintln(r.stdoutFor(), promptStyle.Render(copyPromptHint(copyables)))
		line, _ := reader.ReadString('\n')
		cmd := strings.TrimSpace(line)
		if cmd == "" {
			return
		}
		msg, switched := r.handleCopyCommand(cmd, copyables)
		if msg != "" {
			fmt.Fprintln(r.stdoutFor(), noteStyle.Render("  "+msg))
		}
		if switched {
			r.echoActiveVariant(copyables)
		}
	}
}

// echoActiveVariant re-emits the current active variant of the first
// multi-variant block inline, so the user can see what they switched
// to before deciding to copy. In raw-mode v1.1 this becomes an
// in-place redraw; v1 line mode appends to scrollback (cheap and
// works without cursor positioning).
func (r *Renderer) echoActiveVariant(copyables []copyableBlock) {
	target := r.firstMultiVariantBlock(copyables)
	if target == nil {
		return
	}
	fmt.Fprintln(r.stdoutFor())
	fmt.Fprintln(r.stdoutFor(), r.renderTabStrip(target.view.Variants, r.activeIndex(target.index)))
	active := target.view.Variants[r.activeIndex(target.index)]
	r.renderBoxOrRaw(active.Content, r.Palette.Note)
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

// Compile-time assertions: TUI is the demokit Renderer (drains the
// queue and dispatches into printX helpers) and finishable (waits
// for the drain at Done).
var (
	_ demokit.Renderer           = (*Renderer)(nil)
	_ demokit.FinishableRenderer = (*Renderer)(nil)
)

// AttachEventQueue wires the demokit event queue and spawns the
// drain goroutine. Execute calls this once per run; the drain
// dispatches every event into the printX helpers + the stdout
// snapshot taken below. Snapshots os.Stdout / Stderr so the drain
// writes to stable file handles, race-free against captureOutput's
// redirect (issue 23).
func (r *Renderer) AttachEventQueue(q *events.EventQueue) {
	r.queue = q
	r.drainDone = false
	r.lastStepEv = nil
	r.boxedFlag = false
	r.snapOut = os.Stdout
	r.snapErr = os.Stderr
	r.drainWG.Add(1)
	go func() {
		defer r.drainWG.Done()
		r.drainEvents()
	}()
}

// Finish waits for the drain goroutine to exit. Demokit calls this
// after emitting Done so writes are sequenced before the test/process
// moves on (race-detector hygiene).
func (r *Renderer) Finish() {
	r.drainWG.Wait()
}

// drainEvents subscribes + catch-up drains (events appended before
// Subscribe ran won't fire Notify on their own; see issue 40) +
// loops on Notify until Done sets drainDone.
func (r *Renderer) drainEvents() {
	sub := r.queue.Subscribe()
	defer sub.Close()
	offset := r.drainFrom(0)
	for !r.drainDone {
		<-sub.Notify()
		offset = r.drainFrom(offset)
	}
}

func (r *Renderer) drainFrom(offset int) int {
	evs, newOff := r.queue.ReadFrom(offset)
	for i, ev := range evs {
		r.handleEvent(offset+i, ev)
	}
	return newOff
}

// handleEvent dispatches one event into the shared printX helpers.
// StepReadyToRun stays a no-op here — TUI has no pre-allocated
// output widget to place. No outer stdoutMu lock needed: the drain
// writes through snapshotted *os.File handles taken at
// AttachEventQueue time, so it never touches the live os.Stdout
// variable that captureOutput mutates.
func (r *Renderer) handleEvent(off int, ev events.Event) {
	switch e := ev.(type) {
	case events.Header:
		r.boxedFlag = e.BoxedVerbatim
		r.printHeaderBlock(e.Title, e.Description, e.StepCount)
	case events.Section:
		r.printSectionBlock(e.Title, e.Body)
	case events.StepStart:
		stepCopy := e
		r.lastStepEv = &stepCopy
		r.printStepBlock(e.Visit, e.Declared, e, r.boxedFlag)
	case events.OutputChunk:
		// Chunks flow through the drain. r.stdoutFor() returns the
		// snapshot pinned at AttachEventQueue, race-free against
		// captureOutput's os.Stdout swap (issue 23).
		r.stdoutFor().Write(e.Chunk)
	case events.StepEnd:
		// Output already streamed via the OutputChunk drain above;
		// printResultBlock gets "" so the result label/message
		// don't re-emit the body.
		r.printResultBlock(e.Visit, "", demokit.StepResultFromEvent(e))
	case events.Done:
		r.printDoneBlock()
		r.drainDone = true
	case events.WaitForAdvance:
		// Already resolved (non-interactive defaults) — skip.
		if _, resolved := r.queue.Resolution(off); resolved {
			return
		}
		opts := demokit.WaitOpts{}
		if !e.Deadline.IsZero() {
			opts.AutoAcceptAfter = time.Until(e.Deadline)
		}
		var copyables []copyableBlock
		if r.lastStepEv != nil {
			copyables = copyableBlocksFromVerbatims(verbatimEventsToViews(r.lastStepEv.Verbatims), r.boxedFlag)
		}
		r.waitForAdvanceUI(opts, copyables)
		_ = r.queue.Resolve(off, &events.AdvanceResolution{
			Source: "user-submitted", Timestamp: time.Now(),
		})
	case events.PromptOpen:
		if _, resolved := r.queue.Resolution(off); resolved {
			return
		}
		stepID := ""
		if r.lastStepEv != nil {
			stepID = r.lastStepEv.StepID
		}
		answers := r.activePrompter().Prompt(stepID, inputDefsFromEvents(e.Inputs))
		_ = r.queue.Resolve(off, &events.PromptResolution{
			Answers: answers, Source: "user-submitted", Timestamp: time.Now(),
		})
	}
}

// inputDefsFromEvents projects events.Input back into demokit.InputDef
// so the existing FormPrompter (line-mode readline UI) can consume
// it. The prompter doesn't care about the runtime Parse closure here
// — it re-derives validation from Kind, same as the legacy path.
func inputDefsFromEvents(in []events.Input) []demokit.InputDef {
	out := make([]demokit.InputDef, 0, len(in))
	for _, ev := range in {
		def := demokit.InputDef{
			Name:    ev.InputName(),
			Prompt:  ev.InputPrompt(),
			Default: ev.InputDefault(),
		}
		switch v := ev.(type) {
		case events.IntInput:
			def.Kind = "int"
		case events.ChoiceInput:
			def.Kind = "choice"
			def.Options = v.Options
		default:
			def.Kind = "string"
		}
		out = append(out, def)
	}
	return out
}

// verbatimEventsToViews converts the event projection slice back to
// the legacy view slice — used by waitForAdvanceUI in the drain so
// the existing copyableBlocksFromVerbatims helper can do its work.
func verbatimEventsToViews(in []events.Verbatim) []demokit.VerbatimView {
	out := make([]demokit.VerbatimView, len(in))
	for i, v := range in {
		out[i] = verbatimEventToView(v)
	}
	return out
}
