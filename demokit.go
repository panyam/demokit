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
	"context"
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
	autoAcceptAfter      time.Duration
	showCountdown        bool
	showStepDenominator  bool
	recorder        Recorder
	replay          []TraceEntry
	replayCursor    int

	// CLI flag state. Populated either by RegisterFlags + the user's own
	// FlagSet.Parse, or — if RegisterFlags is never called — by Execute's
	// internal os.Args scan.
	flagsRegistered    bool
	flagNonInteractive bool
	flagRecordPath     string
	flagReplayPath     string
	flagDoc            string // md|html|json|bundle (empty = not requested)
	flagFrom           string // optional trace path used with --doc
	flagOut            string // output file for --doc bundle (else stdout)

	// Sidecar-markdown loader state. Errors are stored rather than
	// returned so FromMarkdown stays chainable; surfaced at Execute.
	loadError    error
	loadWarnings []string
	bindErrors   []string
}

// New creates a new Demo with the given title.
func New(title string) *Demo {
	return &Demo{title: title, runPrefix: "examples"}
}

// Title returns the demo's title (set via New).
func (d *Demo) Title() string { return d.title }

// DocHandler renders a documentation format. demokit core comes with
// md/html/json built in; additional formats register themselves at
// init time so demos opt in by blank-importing the package.
//
// out is the path passed via --out (empty string = stdout).
type DocHandler func(d *Demo, entries []TraceEntry, out string) error

var docHandlers = map[string]DocHandler{}

// RegisterDocFormat installs a handler for a --doc <name> format.
// Typical pattern — a separate package registers itself in init():
//
//	func init() {
//	    demokit.RegisterDocFormat("bundle", func(d *demokit.Demo, entries []demokit.TraceEntry, out string) error {
//	        return WriteBundle(d, entries, out)
//	    })
//	}
//
// Names "md", "html", "json" are reserved by core; reusing them panics.
func RegisterDocFormat(name string, h DocHandler) {
	switch name {
	case "md", "html", "json":
		panic("demokit: cannot override built-in doc format " + name)
	}
	docHandlers[name] = h
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

// ShowStepDenominator controls whether step headings render as
// "Step N/M" (denominator on) or just "Step N" (off, default). The
// denominator is misleading for cyclic graphs where the visit count
// can exceed the declared step count, so it's opt-in.
//
// Linear demos with no Next jumps benefit from "Step N/M" — set this
// to true on those. For cyclic / branching demos, leave it off.
func (d *Demo) ShowStepDenominator(show bool) *Demo {
	d.showStepDenominator = show
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
	fs.StringVar(&d.flagRecordPath, "record", "",
		"record this run to the given trace file")
	fs.StringVar(&d.flagReplayPath, "replay", "",
		"replay from the given trace file (forces deterministic Next)")
	fs.StringVar(&d.flagDoc, "doc", "",
		"emit documentation in the given format (md|html|json|bundle) and exit")
	fs.StringVar(&d.flagFrom, "from", "",
		"trace file to render with --doc (omit for static-definition output)")
	fs.StringVar(&d.flagOut, "out", "",
		"output file for --doc bundle (else writes to stdout)")
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
		case arg == "--doc" && i+1 < len(args):
			d.flagDoc = args[i+1]
			i++
		case strings.HasPrefix(arg, "--doc="):
			d.flagDoc = strings.TrimPrefix(arg, "--doc=")
		case arg == "--from" && i+1 < len(args):
			d.flagFrom = args[i+1]
			i++
		case strings.HasPrefix(arg, "--from="):
			d.flagFrom = strings.TrimPrefix(arg, "--from=")
		case arg == "--out" && i+1 < len(args):
			d.flagOut = args[i+1]
			i++
		case strings.HasPrefix(arg, "--out="):
			d.flagOut = strings.TrimPrefix(arg, "--out=")
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

	// Sidecar-markdown errors are fatal — surface them once and abort
	// before any flag dispatch, so users see the real cause rather
	// than a downstream symptom (empty doc output, missing step).
	if d.loadError != nil {
		fmt.Fprintf(os.Stderr, "demokit: %v\n", d.loadError)
		return
	}
	if len(d.bindErrors) > 0 {
		fmt.Fprintf(os.Stderr, "demokit: Bind to unknown step id(s): %s\n",
			strings.Join(d.bindErrors, ", "))
		return
	}
	for _, w := range d.loadWarnings {
		fmt.Fprintf(os.Stderr, "demokit: %s\n", w)
	}

	// --doc <format> [--from <trace>] short-circuits before any
	// run-mode setup so document generation never executes Run.
	if d.flagDoc != "" {
		d.emitDoc(d.flagDoc, d.flagFrom)
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
				msg := fmt.Sprintf("max visits (%d) exceeded for step %q", d.maxVisits, v.id)
				// Record the abort as the trace's final entry so doc
				// renders and embed players show the error rather
				// than silently truncating mid-loop.
				if d.recorder != nil {
					d.recorder.Record(TraceEntry{
						Kind:    KindStep,
						Title:   v.title,
						StepID:  v.id,
						Visit:   visits[v.id],
						Status:  StatusError,
						Message: msg,
					})
				}
				r.RenderResult(totalVisits, "", Errf("%s", msg))
				break walk
			}

			// totalSteps == 0 means "no denominator". Demos opt in
			// via ShowStepDenominator for linear walkthroughs.
			declared := 0
			if d.showStepDenominator {
				declared = d.stepCount
			}
			r.RenderStep(totalVisits, declared, v)

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
			// Build a per-step context for cancellation. Steps without
			// Timeout or Cancellable get a never-cancelled background
			// context (always safe to read). Cancellable + interactive
			// starts a stdin watcher that fires cancel on Enter.
			runCtx, cancelRun := context.WithCancel(context.Background())
			if v.timeout > 0 {
				var stopTimeout context.CancelFunc
				runCtx, stopTimeout = context.WithTimeout(runCtx, v.timeout)
				defer stopTimeout()
			}
			var stopWatcher func()
			if v.cancellable && interactive && !replaying {
				stopWatcher = watchCancelKey(cancelRun)
			}

			ctx := StepContext{
				Inputs: inputs,
				Visits: visits[v.id],
				Ctx:    runCtx,
			}
			if v.coalesce != nil {
				ctx.Input = v.coalesce(inputs)
			} else {
				ctx.Input = inputs
			}

			var output string
			var result *StepResult
			// Tee output chunks into the renderer in real time when it
			// supports streaming. The trace and recorder still receive
			// the full captured string regardless. Snapshot os.Stdout
			// before captureOutput redirects it so the chunk callback
			// can write to the user's actual terminal — writing to
			// os.Stdout from the drain goroutine would loop the bytes
			// back into the capture pipe.
			var onChunk func([]byte)
			streaming := false
			if sr, ok := r.(StreamingRenderer); ok {
				streaming = true
				stepNum := totalVisits
				originalStdout := os.Stdout
				onChunk = func(chunk []byte) {
					sr.StreamOutput(stepNum, chunk, originalStdout)
				}
			}
			if v.runFn != nil {
				output, result = captureOutput(v.runFn, ctx, onChunk)
			}
			// Capture whether the context fired during Run BEFORE the
			// cleanup-cancel below, otherwise we can't distinguish
			// "Run ran to completion" from "Run was externally
			// cancelled" — both end up with runCtx.Err() != nil.
			ctxFiredDuringRun := runCtx.Err() != nil
			ctxErr := runCtx.Err()
			if stopWatcher != nil {
				stopWatcher()
			}
			cancelRun()

			// If the context fired before Run returned a result of its
			// own, surface that as an Info so the user sees why the
			// step ended. A user-supplied result wins (Run noticed the
			// cancel and returned its own message).
			if result == nil && ctxFiredDuringRun {
				result = Info(cancelReason(ctxErr))
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

			// When the body has already been streamed, hand RenderResult
			// an empty output so it doesn't double-print. The trace
			// entry below still carries the full captured string.
			displayOutput := output
			if streaming {
				displayOutput = ""
			}
			r.RenderResult(totalVisits, displayOutput, result)

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

	// Record the MaxSteps abort to the trace BEFORE closing the
	// recorder so doc renders / embed players show the error as the
	// final entry rather than truncating silently. The synthesized
	// step uses a sentinel id so it doesn't collide with author-
	// defined steps.
	if totalVisits >= maxSteps && cur < len(d.items) {
		msg := fmt.Sprintf("max steps (%d) reached; aborting demo", maxSteps)
		if d.recorder != nil {
			d.recorder.Record(TraceEntry{
				Kind:    KindStep,
				Title:   "Aborted",
				StepID:  "__demokit_aborted__",
				Visit:   1,
				Status:  StatusError,
				Message: msg,
			})
		}
		r.RenderResult(totalVisits, "", Errf("%s", msg))
	}

	if d.recorder != nil {
		_ = closeRecorder(d.recorder)
	}

	r.RenderDone()
}

// emitDoc writes one documentation render to stdout (or, for the
// bundle format, to --out if set). The format argument selects the
// renderer (md, html, json, bundle); from selects the source —
// empty means static (definition only) and a non-empty path is
// loaded as a trace. Errors are written to stderr; a malformed
// trace or unknown format produces no stdout output.
func (d *Demo) emitDoc(format, from string) {
	var entries []TraceEntry
	if from != "" {
		loaded, err := LoadTrace(from)
		if err != nil {
			fmt.Fprintf(os.Stderr, "demokit: --from %s: %v\n", from, err)
			return
		}
		entries = loaded
	}

	switch format {
	case "md":
		// Static md routes to the rich Demo.Markdown() visitor; trace
		// md routes to the per-entry layered renderer. They walk
		// different sources (declarations vs recorded entries) and
		// produce intentionally different shapes.
		if entries == nil {
			fmt.Print(d.Markdown())
		} else {
			fmt.Print(RenderDocumentMD(RenderContext{Demo: d, Trace: entries}))
		}
	case "html":
		fmt.Print(RenderDocumentHTML(RenderContext{Demo: d, Trace: entries}))
	case "json":
		fmt.Print(RenderDocumentJSON(RenderContext{Demo: d, Trace: entries}))
	default:
		// Registered formats (e.g. "bundle" via demokit/web). Hint
		// at the most common case if it's missing.
		if h, ok := docHandlers[format]; ok {
			if err := h(d, entries, d.flagOut); err != nil {
				fmt.Fprintf(os.Stderr, "demokit: --doc %s: %v\n", format, err)
			}
			return
		}
		if format == "bundle" {
			fmt.Fprintln(os.Stderr,
				"demokit: --doc bundle is not enabled. Add `_ \"github.com/panyam/demokit/web\"` to your imports.")
			return
		}
		fmt.Fprintf(os.Stderr, "demokit: unknown --doc format %q (want md|html|json or registered format)\n", format)
	}
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

