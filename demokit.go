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

// StepDef defines one executable step in the demo.
type StepDef struct {
	title  string
	arrows []arrowDef
	refs   []Ref
	note   string
	runFn  func()
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

// Run sets the function to execute for this step.
func (s *StepDef) Run(fn func()) *StepDef {
	s.runFn = fn
	return s
}

// --- Read accessors for renderers ---

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
	title       string
	description string
	dir         string // directory name for run commands in generated README
	runPrefix   string // path prefix for run commands (default: "examples")
	actors      []ActorDef
	items       []item
	stepCount   int
	renderer    Renderer // nil means PlainRenderer
}

// New creates a new Demo with the given title.
func New(title string) *Demo {
	return &Demo{title: title, runPrefix: "examples"}
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
	// output is empty if the step had no runFn or produced no output.
	// err is non-nil if the output capture failed (the step still ran).
	RenderResult(stepNum int, output string, err error)
	// RenderSection displays a non-executable explanatory block.
	RenderSection(section *SectionDef)
	// RenderDone displays the demo completion message.
	RenderDone()
	// WaitForStep blocks until the user is ready to run the next step.
	// Called only in interactive mode.
	WaitForStep()
}

// --- PlainRenderer: default text-only renderer (zero dependencies) ---

// PlainRenderer renders demo output as plain text to stdout.
// This preserves the original demokit output style.
type PlainRenderer struct {
	// Delay is the per-line smooth scroll delay. Zero means no delay (instant).
	// Set to e.g. 18ms for a smooth scroll effect.
	Delay time.Duration
}

// printLine prints a line and optionally sleeps for the smooth scroll effect.
func (r *PlainRenderer) printLine(format string, args ...any) {
	fmt.Printf(format, args...)
	if r.Delay > 0 {
		time.Sleep(r.Delay)
	}
}

func (r *PlainRenderer) RenderHeader(title, description string, stepCount int) {
	r.printLine("=== %s ===\n", title)
	if description != "" {
		r.printLine("    %s\n", description)
	}
	r.printLine("    %d steps\n", stepCount)
	fmt.Println()
}

func (r *PlainRenderer) RenderStep(stepNum, totalSteps int, step *StepDef) {
	r.printLine("  Step %d/%d: %s  [README → Step %d]\n", stepNum, totalSteps, step.title, stepNum)

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
		r.printLine("\n    %s\n", step.note)
	}
}

func (r *PlainRenderer) RenderResult(_ int, output string, _ error) {
	if output != "" {
		for _, line := range strings.Split(output, "\n") {
			r.printLine("%s\n", line)
		}
	} else {
		fmt.Println()
	}
}

func (r *PlainRenderer) RenderSection(section *SectionDef) {
	r.printLine("  --- %s ---\n", section.title)
	for _, line := range strings.Split(section.body, "\n") {
		r.printLine("    %s\n", line)
	}
	fmt.Println()
}

func (r *PlainRenderer) RenderDone() {
	r.printLine("=== Done ===\n")
}

func (r *PlainRenderer) WaitForStep() {
	fmt.Print("\n    Press Enter to run this step...")
	bufio.NewReader(os.Stdin).ReadString('\n')
	fmt.Println()
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
func (d *Demo) Execute() {
	interactive := isTerminal()
	for _, arg := range os.Args[1:] {
		if arg == "--non-interactive" {
			interactive = false
		}
		if arg == "--readme" {
			fmt.Print(d.Markdown())
			return
		}
	}

	r := d.renderer
	if r == nil {
		r = &PlainRenderer{}
	}

	r.RenderHeader(d.title, d.description, d.stepCount)

	stepNum := 0
	for _, it := range d.items {
		switch v := it.(type) {
		case *StepDef:
			stepNum++
			r.RenderStep(stepNum, d.stepCount, v)

			if interactive {
				r.WaitForStep()
			}

			// Capture output from runFn
			var output string
			var captureErr error
			if v.runFn != nil {
				output, captureErr = captureOutput(v.runFn)
			}
			r.RenderResult(stepNum, output, captureErr)

		case *SectionDef:
			r.RenderSection(v)
		}
	}

	r.RenderDone()
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
func captureOutput(fn func()) (string, error) {
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		// If pipe fails, just run normally — don't block the demo.
		fn()
		return "", err
	}
	os.Stdout = w

	done := make(chan string)
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		done <- buf.String()
	}()

	fn()

	w.Close()
	os.Stdout = old
	return <-done, nil
}

// isTerminal returns true if stdin appears to be an interactive terminal.
func isTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
