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
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

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

	// CLI flag state. Populated either by RegisterFlags + the user's own
	// FlagSet.Parse, or — if RegisterFlags is never called — by Execute's
	// internal os.Args scan.
	flagsRegistered     bool
	flagNonInteractive  bool
	flagReadme          bool
	flagReadmeFromPath  string
	flagReadmeHTMLPath  string
	flagRecordPath      string
	flagReplayPath      string
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

// --- Execute ---

// WithRenderer sets a custom renderer for the demo.
// If not called, Execute uses PlainRenderer.
func (d *Demo) WithRenderer(r Renderer) *Demo {
	d.renderer = r
	return d
}

// RegisterFlags registers demokit's CLI flags onto fs. Use this when
// your demo has its own flags and you want to manage parsing centrally:
//
//	flag.StringVar(&myFlag, "my-flag", "", "...")
//	demo.RegisterFlags(flag.CommandLine)
//	flag.Parse()
//	demo.Execute()
//
// When RegisterFlags is called, Execute trusts the FlagSet's Parse to
// have populated the flag values and skips its own os.Args scan. If
// RegisterFlags is never called, Execute scans os.Args itself for its
// own flags only — leaving any unknown args alone, so demos with
// foreign flags (e.g. examples/basic/main.go's --tui / --smooth)
// continue to work without coordination.
//
// All values configured programmatically (WithRecorder, WithReplay,
// AutoAcceptAfter, etc.) take precedence over their flag equivalents.
func (d *Demo) RegisterFlags(fs *flag.FlagSet) {
	d.flagsRegistered = true
	fs.BoolVar(&d.flagNonInteractive, "non-interactive", false,
		"skip pauses between steps")
	fs.BoolVar(&d.flagReadme, "readme", false,
		"emit static markdown for the demo to stdout and exit")
	fs.StringVar(&d.flagReadmeFromPath, "readme-from", "",
		"emit markdown rendered from the given trace file and exit")
	fs.StringVar(&d.flagReadmeHTMLPath, "readme-html-from", "",
		"emit HTML rendered from the given trace file and exit")
	fs.StringVar(&d.flagRecordPath, "record", "",
		"record this run to the given trace file")
	fs.StringVar(&d.flagReplayPath, "replay", "",
		"replay from the given trace file (forces deterministic Next)")
}

// scanOwnArgs is the default flag scanner used when RegisterFlags is
// not called. It only consumes demokit's own flags, ignoring everything
// else so example demos can layer their own flags via plain os.Args
// inspection without conflict.
func (d *Demo) scanOwnArgs(args []string) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--non-interactive":
			d.flagNonInteractive = true
		case arg == "--readme":
			d.flagReadme = true
		case arg == "--readme-from" && i+1 < len(args):
			d.flagReadmeFromPath = args[i+1]
			i++
		case strings.HasPrefix(arg, "--readme-from="):
			d.flagReadmeFromPath = strings.TrimPrefix(arg, "--readme-from=")
		case arg == "--readme-html-from" && i+1 < len(args):
			d.flagReadmeHTMLPath = args[i+1]
			i++
		case strings.HasPrefix(arg, "--readme-html-from="):
			d.flagReadmeHTMLPath = strings.TrimPrefix(arg, "--readme-html-from=")
		case arg == "--record" && i+1 < len(args):
			d.flagRecordPath = args[i+1]
			i++
		case strings.HasPrefix(arg, "--record="):
			d.flagRecordPath = strings.TrimPrefix(arg, "--record=")
		case arg == "--replay" && i+1 < len(args):
			d.flagReplayPath = args[i+1]
			i++
		case strings.HasPrefix(arg, "--replay="):
			d.flagReplayPath = strings.TrimPrefix(arg, "--replay=")
		}
	}
}

// Execute runs the demo interactively — pausing between steps for Enter.
// If --non-interactive is passed (or stdin is not a terminal), runs without pausing.
//
// Steps execute in declaration order by default. Any StepResult with a
// non-empty Next jumps to the matching step ID instead of advancing
// linearly. A safety guard (Demo.MaxSteps, default 200) bounds total
// step visits to prevent infinite loops.
func (d *Demo) Execute() {
	if !d.flagsRegistered {
		d.scanOwnArgs(os.Args[1:])
	}

	// Doc-emit shortcuts exit before doing anything else.
	if d.flagReadme {
		fmt.Print(d.Markdown())
		return
	}
	if d.flagReadmeFromPath != "" {
		entries, err := LoadTrace(d.flagReadmeFromPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "demokit: --readme-from %s: %v\n", d.flagReadmeFromPath, err)
			return
		}
		fmt.Print(RenderDocumentMD(RenderContext{Demo: d, Trace: entries}))
		return
	}
	if d.flagReadmeHTMLPath != "" {
		entries, err := LoadTrace(d.flagReadmeHTMLPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "demokit: --readme-html-from %s: %v\n", d.flagReadmeHTMLPath, err)
			return
		}
		fmt.Print(RenderDocumentHTML(RenderContext{Demo: d, Trace: entries}))
		return
	}

	// Programmatic config wins; flags only fill in when not already set.
	if d.flagRecordPath != "" && d.recorder == nil {
		d.recorder = NewJSONFileRecorder(d.flagRecordPath)
	}
	if d.flagReplayPath != "" && d.replay == nil {
		if entries, err := LoadTrace(d.flagReplayPath); err == nil {
			d.replay = entries
		} else {
			fmt.Fprintf(os.Stderr, "demokit: --replay %s: %v\n", d.flagReplayPath, err)
		}
	}

	interactive := isTerminal() && !d.flagNonInteractive

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

