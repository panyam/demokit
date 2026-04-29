// Package demokit provides an interactive step-through framework for runnable
// examples. Each example defines a sequence of steps (with mermaid diagram
// arrows) and optional sections (explanatory text). The same definitions
// drive both the interactive CLI and the generated README.
//
// Single source of truth: step titles, arrows, and notes are defined in Go
// code. The README's mermaid diagram and step documentation are generated
// from these definitions — never maintained by hand.
//
// Usage:
//
//	demo := demokit.New("01: Client Credentials Flow").
//	    Description("Non-UI | No infrastructure needed").
//	    Actors(
//	        demokit.Actor("App", "Client App"),
//	        demokit.Actor("AS", "Auth Server"),
//	    )
//	demo.Step("Register a client").
//	    Arrow("App", "AS", "POST /apps/register").
//	    Arrow("AS", "App", "{client_id, client_secret}").
//	    Note("The client gets credentials for later use.").
//	    Run(func() { fmt.Println("registered!") })
//	demo.Execute()
package demokit

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"
)

// ActorDef defines a participant in the sequence diagram.
type ActorDef struct {
	ID    string // Short identifier used in arrows (e.g., "AS")
	Label string // Display label (e.g., "Auth Server")
}

// Actor creates an ActorDef.
func Actor(id, label string) ActorDef {
	return ActorDef{ID: id, Label: label}
}

// item is a union type for the ordered sequence of steps and sections.
type item interface {
	isItem()
}

// Ref is a named reference (RFC, CVE, blog post, spec section, etc.).
type Ref struct {
	Name string // e.g., "RFC 7519 (JWT)" or "CVE-2015-9235"
	URL  string // e.g., "https://www.rfc-editor.org/rfc/rfc7519"
}

// ResultStatus controls how a step's result is rendered.
type ResultStatus int

const (
	StatusSuccess ResultStatus = iota // green / "Result"
	StatusError                       // red / "Error"
	StatusWarning                     // yellow / "Warning"
	StatusInfo                        // blue / "Info"
)

// DefaultLabel returns the default display label for a status.
func (s ResultStatus) DefaultLabel() string {
	switch s {
	case StatusError:
		return "Error"
	case StatusWarning:
		return "Warning"
	case StatusInfo:
		return "Info"
	default:
		return "Result"
	}
}

// StepResult carries the outcome of a step's run function.
// A nil *StepResult from Run means success with no message.
// The zero value is also success.
type StepResult struct {
	Status  ResultStatus // controls color/border styling
	Label   string       // custom title; empty = auto from Status
	Message string       // shown prominently (error text, info note, etc.)
	Err     error        // underlying error, if any
	Next    string       // step ID to jump to; empty = fall through to declaration order
}

// StepContext is passed to a step's run function. It carries the resolved
// input payload and visit count. Future fields may include trace metadata.
type StepContext struct {
	// Inputs holds the raw map payload collected from the renderer.
	// Values are typed (e.g. int, string) according to each InputDef's Parse.
	// Empty map if the step declared no inputs.
	Inputs map[string]any
	// Input is the coalesced typed payload returned by the step's Coalesce
	// function. If Coalesce was not set, Input == Inputs.
	Input any
	// Visits is the number of times this step has been entered, including
	// the current visit (so the first visit is 1).
	Visits int
}

// WaitOpts configures the renderer's WaitForStep prompt.
type WaitOpts struct {
	// AutoAcceptAfter, if > 0, advances the demo automatically after this
	// duration even without user input. Zero means wait indefinitely.
	AutoAcceptAfter time.Duration
	// ShowCountdown, when true, asks the renderer to display a visible
	// countdown / progress indicator while AutoAcceptAfter is in effect.
	ShowCountdown bool
}

// DisplayLabel returns Label if set, otherwise the default for the Status.
func (r *StepResult) DisplayLabel() string {
	if r.Label != "" {
		return r.Label
	}
	return r.Status.DefaultLabel()
}

// Convenience constructors for common result types.

// Err creates an error result from an error.
func Err(err error) *StepResult {
	return &StepResult{Status: StatusError, Message: err.Error(), Err: err}
}

// Errf creates an error result from a formatted string.
func Errf(format string, args ...any) *StepResult {
	msg := fmt.Sprintf(format, args...)
	return &StepResult{Status: StatusError, Message: msg, Err: fmt.Errorf("%s", msg)}
}

// Warn creates a warning result.
func Warn(msg string) *StepResult {
	return &StepResult{Status: StatusWarning, Message: msg}
}

// Info creates an informational result.
func Info(msg string) *StepResult {
	return &StepResult{Status: StatusInfo, Message: msg}
}

// StepDef defines one executable step in the demo.
type StepDef struct {
	id       string
	title    string
	arrows   []arrowDef
	refs     []Ref
	note     string
	inputs   []InputDef
	coalesce func(map[string]any) any
	runFn    func(StepContext) *StepResult
}

type arrowDef struct {
	from, to, label string
	dashed          bool // -->> vs ->>
}

// ArrowView is a read-only view of an arrow for use by renderers.
type ArrowView struct {
	From, To, Label string
	Dashed          bool
}

func (s *StepDef) isItem() {}

// ID assigns a stable identifier to this step. IDs are used as jump
// targets by StepResult.Next and by recordings. If unset, the demo
// auto-assigns "step-N" based on declaration order at Execute time.
func (s *StepDef) ID(id string) *StepDef {
	s.id = id
	return s
}

// Arrow adds a solid arrow (request) to the step's sequence diagram.
func (s *StepDef) Arrow(from, to, label string) *StepDef {
	s.arrows = append(s.arrows, arrowDef{from: from, to: to, label: label})
	return s
}

// DashedArrow adds a dashed arrow (response) to the step's sequence diagram.
func (s *StepDef) DashedArrow(from, to, label string) *StepDef {
	s.arrows = append(s.arrows, arrowDef{from: from, to: to, label: label, dashed: true})
	return s
}

// Ref adds a reference (RFC, CVE, spec section, blog post, etc.) to this step.
func (s *StepDef) Ref(ref Ref) *StepDef {
	s.refs = append(s.refs, ref)
	return s
}

// Note adds explanatory text shown in both CLI and README.
func (s *StepDef) Note(text string) *StepDef {
	s.note = text
	return s
}

// Input declares an input the renderer should collect before this step's
// Run executes. Inputs prompt in declaration order and the parsed values
// are placed into StepContext.Inputs keyed by InputDef.Name.
func (s *StepDef) Input(d InputDef) *StepDef {
	s.inputs = append(s.inputs, d)
	return s
}

// Coalesce attaches a function that converts the raw inputs map into a
// single typed payload, available to the step as StepContext.Input. If
// not set, ctx.Input == ctx.Inputs (the map itself).
func (s *StepDef) Coalesce(fn func(map[string]any) any) *StepDef {
	s.coalesce = fn
	return s
}

// Inputs returns a read-only view of this step's declared inputs.
func (s *StepDef) Inputs() []InputDef {
	out := make([]InputDef, len(s.inputs))
	copy(out, s.inputs)
	return out
}

// Run sets the function to execute for this step. The function receives a
// StepContext carrying inputs and visit count, and returns a *StepResult
// (or nil for success with no message). Use named return for brevity:
//
//	step.Run(func(ctx demokit.StepContext) (result *demokit.StepResult) {
//	    fmt.Println("did the thing")
//	    return // nil = success
//	})
//
// Set result.Next to a step ID to jump in DAG mode. Empty Next falls
// through to the next item in declaration order.
func (s *StepDef) Run(fn func(StepContext) *StepResult) *StepDef {
	s.runFn = fn
	return s
}

// --- Read accessors for renderers ---

// StepID returns the step's identifier (auto-assigned at Execute time if unset).
func (s *StepDef) StepID() string { return s.id }

// Title returns the step title.
func (s *StepDef) Title() string { return s.title }

// Note returns the step's explanatory note (may be empty).
func (s *StepDef) NoteText() string { return s.note }

// Refs returns the step's references.
func (s *StepDef) Refs() []Ref { return s.refs }

// Arrows returns a read-only view of the step's sequence diagram arrows.
func (s *StepDef) Arrows() []ArrowView {
	out := make([]ArrowView, len(s.arrows))
	for i, a := range s.arrows {
		out[i] = ArrowView{From: a.from, To: a.to, Label: a.label, Dashed: a.dashed}
	}
	return out
}

// SectionDef is a non-executable block of explanatory content.
type SectionDef struct {
	title string
	body  string
}

func (s *SectionDef) isItem() {}

// Title returns the section title.
func (s *SectionDef) Title() string { return s.title }

// Body returns the section body text.
func (s *SectionDef) Body() string { return s.body }

// Demo is the top-level container for an interactive example.
type Demo struct {
	title           string
	description     string
	dir             string // directory name for run commands in generated README
	runPrefix       string // path prefix for run commands (default: "examples")
	actors          []ActorDef
	items           []item
	stepCount       int
	renderer        Renderer // nil means PlainRenderer
	maxSteps        int      // upper bound on total step visits per Execute (0 = default)
	maxVisits       int      // upper bound on visits to any single step (0 = unlimited)
	autoAcceptAfter time.Duration
	showCountdown   bool
	recorder        Recorder
	replay          []TraceEntry
	replayCursor    int
}

// New creates a new Demo with the given title.
func New(title string) *Demo {
	return &Demo{title: title, runPrefix: "examples"}
}

// MaxSteps caps the total number of step visits per Execute, preventing
// runaway loops in DAGs. Default is 200 if unset.
func (d *Demo) MaxSteps(n int) *Demo {
	d.maxSteps = n
	return d
}

// MaxVisits caps how many times any single step may be entered. Zero
// means unlimited. Useful as a per-node guard in cyclic DAGs.
func (d *Demo) MaxVisits(n int) *Demo {
	d.maxVisits = n
	return d
}

// AutoAcceptAfter configures the duration after which WaitForStep auto-
// advances even without user input. Zero (default) waits indefinitely.
func (d *Demo) AutoAcceptAfter(dur time.Duration) *Demo {
	d.autoAcceptAfter = dur
	return d
}

// ShowCountdown asks the renderer to display a visible countdown while
// AutoAcceptAfter is in effect. Has no effect if AutoAcceptAfter is zero.
func (d *Demo) ShowCountdown(show bool) *Demo {
	d.showCountdown = show
	return d
}

// WithRecorder attaches a Recorder that observes step and section visits
// during Execute. Use NewJSONFileRecorder to persist a trace to disk.
func (d *Demo) WithRecorder(r Recorder) *Demo {
	d.recorder = r
	return d
}

// WithReplay sets a recorded trace to replay. While replay is active,
// the renderer's Prompt is skipped (recorded inputs are used directly)
// and each step's StepResult.Next is overridden by the recorded value
// so the demo follows the same path it took at record time. Steps whose
// IDs do not match the next recorded entry fall back to default inputs
// and the user's own Next.
func (d *Demo) WithReplay(entries []TraceEntry) *Demo {
	d.replay = entries
	d.replayCursor = 0
	return d
}

// nextReplayStep peeks the next step-kind trace entry. If it matches
// stepID, it is consumed and returned. Section entries are skipped over
// (they are reconstructed live by walking the demo's items).
func (d *Demo) nextReplayStep(stepID string) (TraceEntry, bool) {
	for d.replayCursor < len(d.replay) && d.replay[d.replayCursor].Kind != KindStep {
		d.replayCursor++
	}
	if d.replayCursor >= len(d.replay) {
		return TraceEntry{}, false
	}
	e := d.replay[d.replayCursor]
	if e.StepID != stepID {
		return TraceEntry{}, false // do not consume on mismatch
	}
	d.replayCursor++
	return e, true
}

// Description sets the one-line description shown in the CLI header.
func (d *Demo) Description(desc string) *Demo {
	d.description = desc
	return d
}

// Dir sets the directory name used in generated README run commands.
// e.g., Dir("01-client-credentials") produces "go run ./{RunPrefix}/01-client-credentials/"
func (d *Demo) Dir(name string) *Demo {
	d.dir = name
	return d
}

// RunPrefix sets the path prefix for run commands in the generated README.
// Default is "examples". Set to "" for root-level examples.
func (d *Demo) RunPrefix(prefix string) *Demo {
	d.runPrefix = prefix
	return d
}

// Actors sets the sequence diagram participants.
func (d *Demo) Actors(actors ...ActorDef) *Demo {
	d.actors = actors
	return d
}

// Step adds an executable step to the demo. Returns the StepDef for chaining.
func (d *Demo) Step(title string) *StepDef {
	s := &StepDef{title: title}
	d.items = append(d.items, s)
	d.stepCount++
	return s
}

// Section adds a non-executable explanatory block. Lines are joined with
// newlines, so you can write multi-paragraph markdown naturally:
//
//	demo.Section("How it works",
//	    "The auth server signs tokens with HS256.",
//	    "",
//	    "**Key insight:** Both servers share the same KeyStore.",
//	)
func (d *Demo) Section(title string, lines ...string) *Demo {
	d.items = append(d.items, &SectionDef{title: title, body: strings.Join(lines, "\n")})
	return d
}

// Renderer controls how demo output is displayed. Implement this interface
// to provide custom visual styles (e.g., TUI with Lipgloss boxes).
type Renderer interface {
	// RenderHeader displays the demo title, description, and step count.
	RenderHeader(title, description string, stepCount int)
	// RenderStep displays a step's metadata (number, title, arrows, refs, note)
	// before it is executed.
	RenderStep(stepNum, totalSteps int, step *StepDef)
	// RenderResult displays the captured output from a step's run function.
	// output is the captured stdout. result is nil for success with no message.
	RenderResult(stepNum int, output string, result *StepResult)
	// RenderSection displays a non-executable explanatory block.
	RenderSection(section *SectionDef)
	// RenderDone displays the demo completion message.
	RenderDone()
	// WaitForStep blocks until the user is ready to run the next step.
	// Called only in interactive mode. If opts.AutoAcceptAfter > 0 the
	// renderer should advance automatically after that duration.
	WaitForStep(opts WaitOpts)
	// Prompt collects the declared inputs from the user and returns a
	// typed map keyed by InputDef.Name. Only called in interactive mode
	// when the step has at least one input. Implementations should
	// re-prompt on Parse error and respect each input's Default.
	Prompt(stepID string, inputs []InputDef) map[string]any
}

// --- PlainRenderer: default text-only renderer (zero dependencies) ---

// PlainRenderer renders demo output as plain text to stdout.
// This preserves the original demokit output style.
type PlainRenderer struct {
	// Delay is the per-line smooth scroll delay. Zero means no delay (instant).
	// Set to e.g. 18ms for a smooth scroll effect.
	Delay time.Duration
	// MaxWidth caps the output width. 0 means 120.
	MaxWidth int
	// Fraction of terminal width to use. 0 means 0.90.
	Fraction float64
}

// width returns the usable output width.
func (r *PlainRenderer) width() int {
	frac := r.Fraction
	if frac <= 0 {
		frac = 0.90
	}
	maxW := r.MaxWidth
	if maxW <= 0 {
		maxW = 120
	}
	w := int(float64(TermWidth()) * frac)
	if w > maxW {
		w = maxW
	}
	if w < 40 {
		w = 40
	}
	return w
}

// wrapText wraps a string to fit within width, respecting an indent prefix.
// Each output line is prefixed with indent.
func wrapText(s string, width int, indent string) string {
	maxLen := width - len(indent)
	if maxLen <= 10 {
		maxLen = 10
	}
	var result []string
	for _, para := range strings.Split(s, "\n") {
		if para == "" {
			result = append(result, indent)
			continue
		}
		words := strings.Fields(para)
		if len(words) == 0 {
			result = append(result, indent)
			continue
		}
		line := words[0]
		for _, w := range words[1:] {
			if len(line)+1+len(w) > maxLen {
				result = append(result, indent+line)
				line = w
			} else {
				line += " " + w
			}
		}
		result = append(result, indent+line)
	}
	return strings.Join(result, "\n")
}

// printLine prints a line and optionally sleeps for the smooth scroll effect.
func (r *PlainRenderer) printLine(format string, args ...any) {
	fmt.Printf(format, args...)
	if r.Delay > 0 {
		time.Sleep(r.Delay)
	}
}

func (r *PlainRenderer) RenderHeader(title, description string, stepCount int) {
	w := r.width()
	sep := strings.Repeat("=", w)
	r.printLine("%s\n", sep)
	r.printLine("  %s\n", title)
	if description != "" {
		r.printLine("  %s\n", description)
	}
	r.printLine("  %d steps\n", stepCount)
	r.printLine("%s\n", sep)
	fmt.Println()
}

func (r *PlainRenderer) RenderStep(stepNum, totalSteps int, step *StepDef) {
	w := r.width()
	r.printLine("  Step %d/%d: %s\n", stepNum, totalSteps, step.title)
	r.printLine("  %s\n", strings.Repeat("-", w-2))

	if len(step.refs) > 0 {
		refs := "    Refs: "
		for i, ref := range step.refs {
			if i > 0 {
				refs += ", "
			}
			refs += ref.Name
		}
		r.printLine("%s\n", refs)
	}

	for _, a := range step.arrows {
		arrow := "->>"
		if a.dashed {
			arrow = "-->>"
		}
		r.printLine("    %s %s %s: %s\n", a.from, arrow, a.to, a.label)
	}

	if step.note != "" {
		r.printLine("\n%s\n", wrapText(step.note, w, "    "))
	}
}

func (r *PlainRenderer) RenderResult(_ int, output string, result *StepResult) {
	w := r.width()

	// Determine label
	label := "Result"
	if result != nil {
		label = result.DisplayLabel()
	}

	// Print label for non-success or when there's a message
	if result != nil && result.Status != StatusSuccess {
		r.printLine("  [%s]", label)
		if result.Message != "" {
			r.printLine(" %s", result.Message)
		}
		r.printLine("\n")
	}

	if output != "" {
		wrapped := wrapText(output, w, "")
		for _, line := range strings.Split(wrapped, "\n") {
			r.printLine("%s\n", line)
		}
	}
	fmt.Println()
}

func (r *PlainRenderer) RenderSection(section *SectionDef) {
	w := r.width()
	r.printLine("  --- %s ---\n", section.title)
	r.printLine("%s\n", wrapText(section.body, w, "    "))
	fmt.Println()
}

func (r *PlainRenderer) RenderDone() {
	r.printLine("=== Done ===\n")
}

func (r *PlainRenderer) WaitForStep(opts WaitOpts) {
	if opts.AutoAcceptAfter <= 0 {
		fmt.Print("\n    Press Enter to run this step...")
		bufio.NewReader(os.Stdin).ReadString('\n')
		fmt.Println()
		return
	}

	// Auto-advance with optional countdown. Whichever arrives first —
	// stdin newline or timer expiry — wins. The reader goroutine outlives
	// this call if the user never presses Enter; that's acceptable since
	// the next WaitForStep allocates its own pipe and the leaked reader
	// is reaped at process exit.
	enter := make(chan struct{}, 1)
	go func() {
		bufio.NewReader(os.Stdin).ReadString('\n')
		enter <- struct{}{}
	}()

	deadline := time.Now().Add(opts.AutoAcceptAfter)
	if !opts.ShowCountdown {
		fmt.Printf("\n    Press Enter to run (auto in %s)...", opts.AutoAcceptAfter.Round(time.Second))
		select {
		case <-enter:
		case <-time.After(opts.AutoAcceptAfter):
		}
		fmt.Println()
		return
	}

	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	fmt.Println()
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			fmt.Printf("\r    %s\n", strings.Repeat(" ", 60))
			return
		}
		bar := countdownBar(remaining, opts.AutoAcceptAfter, 20)
		fmt.Printf("\r    %s  %4.1fs  (Enter to accept now)", bar, remaining.Seconds())
		select {
		case <-enter:
			fmt.Printf("\r    %s\n", strings.Repeat(" ", 60))
			return
		case <-tick.C:
		}
	}
}

// Prompt collects inputs sequentially via stdin readline. On any Parse
// error the renderer collects the rest of the fields, prints errors, and
// re-prompts everything — using each just-typed valid value as the new
// default so the user only retypes the invalid one (Enter to accept).
func (r *PlainRenderer) Prompt(stepID string, inputs []InputDef) map[string]any {
	if len(inputs) == 0 {
		return map[string]any{}
	}
	pending := make([]InputDef, len(inputs))
	copy(pending, inputs)
	stdin := bufio.NewReader(os.Stdin)

	for attempt := 0; ; attempt++ {
		if attempt > 0 {
			fmt.Println("    Re-enter values (Enter to keep [bracketed]):")
		}
		result := map[string]any{}
		errored := false
		for i, in := range pending {
			label := in.Prompt
			if label == "" {
				label = in.Name
			}
			if in.Default != nil {
				fmt.Printf("    %s [%v]: ", label, in.Default)
			} else {
				fmt.Printf("    %s: ", label)
			}
			line, _ := stdin.ReadString('\n')
			line = strings.TrimRight(line, "\r\n")

			if line == "" && in.Default != nil {
				result[in.Name] = in.Default
				continue
			}

			parser := in.Parse
			if parser == nil {
				parser = func(s string) (any, error) { return s, nil }
			}
			val, err := parser(line)
			if err != nil {
				fmt.Printf("    [error] %v\n", err)
				errored = true
				continue
			}
			result[in.Name] = val
			// Sticky: a valid value becomes the next attempt's default.
			pending[i].Default = val
		}
		if !errored {
			return result
		}
	}
}

// countdownBar renders a left-to-right depleting progress bar.
func countdownBar(remaining, total time.Duration, width int) string {
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
	return "[" + strings.Repeat("#", filled) + strings.Repeat(" ", width-filled) + "]"
}

// --- Execute ---

// WithRenderer sets a custom renderer for the demo.
// If not called, Execute uses PlainRenderer.
func (d *Demo) WithRenderer(r Renderer) *Demo {
	d.renderer = r
	return d
}

// Execute runs the demo interactively — pausing between steps for Enter.
// If --non-interactive is passed (or stdin is not a terminal), runs without pausing.
//
// Steps execute in declaration order by default. Any StepResult with a
// non-empty Next jumps to the matching step ID instead of advancing
// linearly. A safety guard (Demo.MaxSteps, default 200) bounds total
// step visits to prevent infinite loops.
func (d *Demo) Execute() {
	interactive := isTerminal()
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--non-interactive":
			interactive = false
		case arg == "--readme":
			fmt.Print(d.Markdown())
			return
		case arg == "--readme-from" && i+1 < len(args):
			entries, err := LoadTrace(args[i+1])
			if err != nil {
				fmt.Fprintf(os.Stderr, "demokit: --readme-from %s: %v\n", args[i+1], err)
				return
			}
			fmt.Print(MarkdownFromTrace(d, entries))
			return
		case arg == "--readme-html-from" && i+1 < len(args):
			entries, err := LoadTrace(args[i+1])
			if err != nil {
				fmt.Fprintf(os.Stderr, "demokit: --readme-html-from %s: %v\n", args[i+1], err)
				return
			}
			fmt.Print(HTMLFromTrace(d, entries))
			return
		case arg == "--record" && i+1 < len(args):
			if d.recorder == nil {
				d.recorder = NewJSONFileRecorder(args[i+1])
			}
			i++
		case strings.HasPrefix(arg, "--record="):
			if d.recorder == nil {
				d.recorder = NewJSONFileRecorder(strings.TrimPrefix(arg, "--record="))
			}
		case arg == "--replay" && i+1 < len(args):
			if d.replay == nil {
				if entries, err := LoadTrace(args[i+1]); err == nil {
					d.replay = entries
				} else {
					fmt.Fprintf(os.Stderr, "demokit: --replay %s: %v\n", args[i+1], err)
				}
			}
			i++
		case strings.HasPrefix(arg, "--replay="):
			if d.replay == nil {
				p := strings.TrimPrefix(arg, "--replay=")
				if entries, err := LoadTrace(p); err == nil {
					d.replay = entries
				} else {
					fmt.Fprintf(os.Stderr, "demokit: --replay %s: %v\n", p, err)
				}
			}
		}
	}

	// Replay mode is non-interactive by default.
	if d.replay != nil {
		interactive = false
	}

	r := d.renderer
	if r == nil {
		r = &PlainRenderer{}
	}

	d.assignStepIDs()
	idxByID := make(map[string]int)
	for i, it := range d.items {
		if s, ok := it.(*StepDef); ok {
			idxByID[s.id] = i
		}
	}

	maxSteps := d.maxSteps
	if maxSteps <= 0 {
		maxSteps = 200
	}
	waitOpts := WaitOpts{
		AutoAcceptAfter: d.autoAcceptAfter,
		ShowCountdown:   d.showCountdown,
	}

	r.RenderHeader(d.title, d.description, d.stepCount)

	visits := make(map[string]int)
	totalVisits := 0
	cur := 0
walk:
	for cur < len(d.items) && totalVisits < maxSteps {
		switch v := d.items[cur].(type) {
		case *StepDef:
			totalVisits++
			visits[v.id]++
			if d.maxVisits > 0 && visits[v.id] > d.maxVisits {
				r.RenderResult(totalVisits, "",
					Errf("max visits (%d) exceeded for step %q", d.maxVisits, v.id))
				break walk
			}

			r.RenderStep(totalVisits, d.stepCount, v)

			// Replay mode pulls inputs and the next-step path from the
			// recorded trace. A mismatched step ID falls through to the
			// normal interactive/default flow.
			var replayEntry TraceEntry
			replaying := false
			if d.replay != nil {
				replayEntry, replaying = d.nextReplayStep(v.id)
			}

			// Steps with declared inputs take their pause from the prompt
			// itself; steps without inputs get the conventional Enter-pause
			// (or auto-accept countdown).
			if interactive && len(v.inputs) == 0 {
				r.WaitForStep(waitOpts)
			}

			var inputs map[string]any
			if replaying {
				inputs = replayEntry.Inputs
				if inputs == nil {
					inputs = map[string]any{}
				}
			} else {
				inputs = collectInputs(r, v, interactive)
			}
			ctx := StepContext{
				Inputs: inputs,
				Visits: visits[v.id],
			}
			if v.coalesce != nil {
				ctx.Input = v.coalesce(inputs)
			} else {
				ctx.Input = inputs
			}

			var output string
			var result *StepResult
			if v.runFn != nil {
				output, result = captureOutput(v.runFn, ctx)
			}

			// In replay mode, the recorded path wins over what the user's
			// Run returned. This keeps replays deterministic even if the
			// step's logic has been refactored since recording.
			if replaying {
				if result == nil {
					result = &StepResult{}
				}
				result.Next = replayEntry.Next
			}

			r.RenderResult(totalVisits, output, result)

			entry := TraceEntry{
				Kind:   KindStep,
				Title:  v.title,
				StepID: v.id,
				Visit:  visits[v.id],
				Inputs: inputs,
				Output: output,
			}
			if result != nil {
				entry.Status = result.Status
				entry.Label = result.Label
				entry.Message = result.Message
			}

			if result != nil && result.Next != "" {
				next, ok := idxByID[result.Next]
				if !ok {
					if d.recorder != nil {
						entry.Status = StatusError
						entry.Message = fmt.Sprintf("unknown step id %q", result.Next)
						d.recorder.Record(entry)
					}
					r.RenderResult(totalVisits, "",
						Errf("unknown step id %q in Next from %q", result.Next, v.id))
					break walk
				}
				entry.Next = result.Next
				if d.recorder != nil {
					d.recorder.Record(entry)
				}
				cur = next
				continue
			}
			if d.recorder != nil {
				d.recorder.Record(entry)
			}
			cur++

		case *SectionDef:
			r.RenderSection(v)
			if d.recorder != nil {
				d.recorder.Record(TraceEntry{
					Kind:  KindSection,
					Title: v.title,
					Body:  v.body,
				})
			}
			cur++
		}
	}

	if d.recorder != nil {
		_ = closeRecorder(d.recorder)
	}

	if totalVisits >= maxSteps && cur < len(d.items) {
		r.RenderResult(totalVisits, "",
			Errf("max steps (%d) reached; aborting demo", maxSteps))
	}

	r.RenderDone()
}

// collectInputs gathers a step's input payload. In interactive mode the
// renderer prompts; in non-interactive mode (or when no inputs are
// declared) defaults are filled in directly.
func collectInputs(r Renderer, s *StepDef, interactive bool) map[string]any {
	if len(s.inputs) == 0 {
		return map[string]any{}
	}
	if interactive {
		return r.Prompt(s.id, s.inputs)
	}
	out := make(map[string]any, len(s.inputs))
	for _, in := range s.inputs {
		if in.Default != nil {
			out[in.Name] = in.Default
		}
	}
	return out
}

// assignStepIDs fills in any unset step IDs with auto-generated ones
// ("step-1", "step-2", …) based on declaration order. User-supplied IDs
// are preserved. Collisions between an explicit ID and an auto ID for a
// later step are resolved by skipping any taken slot.
func (d *Demo) assignStepIDs() {
	used := make(map[string]bool)
	for _, it := range d.items {
		if s, ok := it.(*StepDef); ok && s.id != "" {
			used[s.id] = true
		}
	}
	n := 0
	for _, it := range d.items {
		s, ok := it.(*StepDef)
		if !ok || s.id != "" {
			continue
		}
		for {
			n++
			candidate := fmt.Sprintf("step-%d", n)
			if !used[candidate] {
				s.id = candidate
				used[candidate] = true
				break
			}
		}
	}
}

// Markdown generates the full README content from the demo definition.
// This is the single source of truth — run with --readme to regenerate.
func (d *Demo) Markdown() string {
	var b strings.Builder

	// Title and description
	fmt.Fprintf(&b, "# %s\n\n", d.title)
	if d.description != "" {
		fmt.Fprintf(&b, "%s\n\n", d.description)
	}

	// Collect steps for the summary
	var steps []*StepDef
	for _, it := range d.items {
		if s, ok := it.(*StepDef); ok {
			steps = append(steps, s)
		}
	}

	// What you'll learn (from step notes)
	hasNotes := false
	for _, s := range steps {
		if s.note != "" {
			hasNotes = true
			break
		}
	}
	if hasNotes {
		b.WriteString("## What you'll learn\n\n")
		for _, s := range steps {
			if s.note != "" {
				fmt.Fprintf(&b, "- **%s** — %s\n", s.title, s.note)
			}
		}
		b.WriteString("\n")
	}

	// Sequence diagram
	b.WriteString("## Flow\n\n```mermaid\nsequenceDiagram\n")
	for _, a := range d.actors {
		if a.ID != a.Label {
			fmt.Fprintf(&b, "    participant %s as %s\n", a.ID, a.Label)
		} else {
			fmt.Fprintf(&b, "    participant %s\n", a.ID)
		}
	}
	stepNum := 0
	for _, it := range d.items {
		switch v := it.(type) {
		case *StepDef:
			stepNum++
			fmt.Fprintf(&b, "\n    Note over %s,%s: Step %d: %s\n",
				d.actors[0].ID, d.actors[len(d.actors)-1].ID, stepNum, v.title)
			for _, a := range v.arrows {
				if a.dashed {
					fmt.Fprintf(&b, "    %s-->>%s: %s\n", a.from, a.to, a.label)
				} else {
					fmt.Fprintf(&b, "    %s->>%s: %s\n", a.from, a.to, a.label)
				}
			}
		}
	}
	b.WriteString("```\n\n")

	// Steps detail
	b.WriteString("## Steps\n\n")
	stepNum = 0
	allRefs := make(map[string]Ref) // dedup by URL
	for _, it := range d.items {
		switch v := it.(type) {
		case *StepDef:
			stepNum++
			fmt.Fprintf(&b, "### Step %d: %s\n\n", stepNum, v.title)
			if len(v.refs) > 0 {
				b.WriteString("> **References:** ")
				for i, ref := range v.refs {
					if i > 0 {
						b.WriteString(", ")
					}
					fmt.Fprintf(&b, "[%s](%s)", ref.Name, ref.URL)
					allRefs[ref.URL] = ref
				}
				b.WriteString("\n\n")
			}
			if v.note != "" {
				fmt.Fprintf(&b, "%s\n\n", v.note)
			}
		case *SectionDef:
			fmt.Fprintf(&b, "### %s\n\n%s\n\n", v.title, v.body)
		}
	}

	// Collected references (deduped)
	if len(allRefs) > 0 {
		b.WriteString("## References\n\n")
		for _, ref := range allRefs {
			fmt.Fprintf(&b, "- [%s](%s)\n", ref.Name, ref.URL)
		}
		b.WriteString("\n")
	}

	// Run command
	dir := d.dir
	if dir == "" {
		dir = "<this-directory>"
	}
	runPath := dir
	if d.runPrefix != "" {
		runPath = d.runPrefix + "/" + dir
	}
	b.WriteString("## Run it\n\n")
	fmt.Fprintf(&b, "```bash\ngo run ./%s/\n```\n\n", runPath)
	b.WriteString("Pass `--non-interactive` to skip pauses:\n\n")
	fmt.Fprintf(&b, "```bash\ngo run ./%s/ --non-interactive\n```\n", runPath)

	return b.String()
}

// captureOutput redirects stdout while fn runs and returns what was written.
// Panics are recovered and converted to a StepResult with StatusError.
func captureOutput(fn func(StepContext) *StepResult, ctx StepContext) (output string, result *StepResult) {
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		// If pipe fails, just run normally — don't block the demo.
		result = fn(ctx)
		return "", result
	}
	os.Stdout = w

	done := make(chan string)
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		done <- buf.String()
	}()

	func() {
		defer func() {
			if rec := recover(); rec != nil {
				result = &StepResult{
					Status:  StatusError,
					Message: fmt.Sprintf("panic: %v", rec),
				}
			}
		}()
		result = fn(ctx)
	}()

	w.Close()
	os.Stdout = old
	output = <-done
	return output, result
}

// TermWidth returns the current terminal width, or 80 as fallback.
// Tries stdout, then stderr (which stays connected even when stdout is piped).
func TermWidth() int {
	for _, f := range []*os.File{os.Stdout, os.Stderr} {
		if w, _, err := term.GetSize(f.Fd()); err == nil && w > 0 {
			return w
		}
	}
	return 80
}

// isTerminal returns true if stdin appears to be an interactive terminal.
func isTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
