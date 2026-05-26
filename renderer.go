package demokit

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/muesli/cancelreader"

	"github.com/panyam/demokit/events"
)

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

// StreamingRenderer is implemented by renderers that can show step
// output incrementally while Run is still executing. demokit detects
// this via type assertion: when the renderer implements StreamOutput,
// captureOutput tees each chunk into the renderer in roughly real
// time. RenderResult is then called with output == "" because the
// body has already appeared on screen.
//
// Renderers that don't implement StreamingRenderer get the buffered
// path: the full captured output is passed to RenderResult after Run
// returns.
type StreamingRenderer interface {
	Renderer
	// StreamOutput is called for each byte chunk a step's Run writes
	// to stdout or stderr while it's still executing. May be invoked
	// from a goroutine other than the one driving Render*; renderers
	// must serialize their own state if needed.
	//
	// out is the writer to emit chunks to — the user's actual stdout,
	// captured before captureOutput redirected it. Writing to
	// os.Stdout directly would loop the chunks back into the capture
	// pipe. stepNum is the absolute visit count (matching what
	// RenderStep received); implementations are free to ignore it.
	StreamOutput(stepNum int, chunk []byte, out io.Writer)
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

	// lastStep is set by RenderStep (legacy path) and consulted by
	// WaitForStep to build the digit-to-copy prompt. Cleared at the
	// next RenderStep so a step with no copyables falls through to
	// the plain Enter pause.
	lastStep *StepDef

	// --- event-aware path (demokit.EventAwareRenderer) ---
	//
	// When Execute attaches its queue via AttachEventQueue, plain
	// becomes a queue consumer like the bridge: a goroutine drains
	// the *discrete* events (Header, StepStart, Section, StepEnd,
	// Done, sync waits/prompts) and dispatches to the same printX
	// helpers the legacy methods use. OutputChunk events are
	// deliberately NOT handled here — chunks continue to flow live
	// to the user's terminal via the StreamingRenderer.StreamOutput
	// inline tee in Execute, which already has the real
	// pre-captureOutput stdout writer to print to. Execute skips
	// its dual legacy-method calls for the discrete events when
	// this renderer is attached (see demokit.go).
	queue      *events.EventQueue
	boxedFlag  bool              // mirror of events.Header.BoxedVerbatim
	lastStepEv *events.StepStart // event-side mirror of lastStep, for the copy prompt
	done       bool              // set by Done; tells drainEvents to exit so the goroutine doesn't leak across runs/tests
	drainWG    sync.WaitGroup    // tracks the drain goroutine so Finish can wait for it

	// Snapshots of os.Stdout / os.Stderr taken at AttachEventQueue
	// time, when no captureOutput is in flight. The drain goroutine
	// prints through these instead of reading the live os.Stdout
	// var — otherwise it races with captureOutput's stdout redirect
	// (issue 23 same class), AND nested RLock acquisition (drain
	// RLock + TermWidth RLock) deadlocks Go's RWMutex when a
	// writer's waiting.
	snapOut *os.File
	snapErr *os.File
}

// width returns the usable output width. Uses the renderer's snapshot
// *os.File handles when set (drain path) so the term.GetSize call
// doesn't race with captureOutput's os.Stdout swap.
func (r *PlainRenderer) width() int {
	frac := r.Fraction
	if frac <= 0 {
		frac = 0.90
	}
	maxW := r.MaxWidth
	if maxW <= 0 {
		maxW = 120
	}
	w := int(float64(r.termWidth()) * frac)
	if w > maxW {
		w = maxW
	}
	if w < 40 {
		w = 40
	}
	return w
}

// termWidth queries the terminal width through the renderer's
// snapshot file handles (drain path) or live os.Stdout/Stderr (legacy
// path), 80 fallback. The drain-side path takes no extra lock — its
// snapshots are pinned at AttachEventQueue time.
func (r *PlainRenderer) termWidth() int {
	if r.snapOut != nil || r.snapErr != nil {
		for _, f := range []*os.File{r.stdoutFile(), r.stderrFile()} {
			if w, _, err := term.GetSize(f.Fd()); err == nil && w > 0 {
				return w
			}
		}
		return 80
	}
	// Legacy path: live os.Stdout/Stderr, gated by TermWidth's RLock.
	return TermWidth()
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
// Writes through r.stdoutFor() so the event-aware drain uses its
// snapshotted *os.File (race-free with captureOutput's redirect).
func (r *PlainRenderer) printLine(format string, args ...any) {
	fmt.Fprintf(r.stdoutFor(), format, args...)
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
	fmt.Fprintln(r.stdoutFor())
}

func (r *PlainRenderer) RenderStep(stepNum, totalSteps int, step *StepDef) {
	r.lastStep = step
	demo := step.Demo()
	r.printStepBlock(stepNum, totalSteps, events.StepStart{
		Visit:     stepNum,
		StepID:    step.id,
		Title:     step.title,
		Note:      step.note,
		Declared:  totalSteps,
		Arrows:    arrowsToEvents(step.Arrows()),
		Refs:      refsToEvents(step.Refs()),
		Verbatims: verbatimsToEvents(step.VerbatimBlocks()),
	}, demo != nil && demo.IsBoxedVerbatim())
}

// printStepBlock is the shared formatter: the legacy RenderStep
// extracts fields from *StepDef into the event projection and calls
// here; the event drain calls here directly with the projection it
// already has. boxedDefault is the demo-wide flag (Header carries it
// for the drain; the legacy path pulls it from step.Demo()).
func (r *PlainRenderer) printStepBlock(stepNum, totalSteps int, e events.StepStart, boxedDefault bool) {
	w := r.width()
	// totalSteps == 0 means "no denominator" — Demo.ShowStepDenominator
	// defaults off because the count is misleading for cyclic graphs.
	// stepNum > totalSteps is a belt-and-suspenders fallback if a demo
	// opts in but ends up cyclic anyway.
	if totalSteps == 0 || stepNum > totalSteps {
		r.printLine("  Step %d: %s\n", stepNum, e.Title)
	} else {
		r.printLine("  Step %d/%d: %s\n", stepNum, totalSteps, e.Title)
	}
	r.printLine("  %s\n", strings.Repeat("-", w-2))

	if len(e.Refs) > 0 {
		refs := "    Refs: "
		for i, ref := range e.Refs {
			if i > 0 {
				refs += ", "
			}
			refs += ref.Name
		}
		r.printLine("%s\n", refs)
	}

	for _, a := range e.Arrows {
		arrow := "->>"
		if a.Dashed {
			arrow = "-->>"
		}
		r.printLine("    %s %s %s: %s\n", a.From, arrow, a.To, a.Label)
	}

	if e.Note != "" {
		r.printLine("\n%s\n", wrapText(e.Note, w, "    "))
	}

	r.printVerbatimBlocks(e.Verbatims, boxedDefault)
}

// printVerbatim emits a step's verbatim blocks in plain-text form.
// Layout:
//
//	<Block label>
//
//	  [N] variant-label (default)
//	      content line 1
//	      content line 2
//
//	  [N+1] other-variant
//	      ...
//
// Copyable variants are numbered globally across the step — the same N
// a user types at the pause prompt to copy the variant. Non-copyable
// single-variant blocks render their content under the block label
// with no [N] prefix (mouse-select copy stays the natural affordance
// in unboxed demos).
//
// Numbering walks the same iteration order as
// StepDef.NumberedCopyables, so the rendered N matches the prompt's
// accepted digit one-for-one.
//
// --variant filtering is intentionally NOT applied here: interactive
// users need every variant visible so they can pick by number. The
// filter is for documentation output (markdown / HTML / JSON), where
// reference docs do want trimmable output.
// printVerbatimBlocks is the shared formatter: takes the
// already-projected event vocabulary so the drain can call it
// without reconstructing a *StepDef.
func (r *PlainRenderer) printVerbatimBlocks(blocks []events.Verbatim, boxedDefault bool) {
	if len(blocks) == 0 {
		return
	}
	counter := 0
	for _, b := range blocks {
		if len(b.Variants) == 0 {
			continue
		}
		copyable := boxedDefault || len(b.Variants) > 1

		fmt.Fprintln(r.stdoutFor())
		if b.Label != "" {
			r.printLine("  %s\n", b.Label)
		}
		for _, va := range b.Variants {
			fmt.Fprintln(r.stdoutFor())
			if copyable {
				counter++
				label := va.Label
				if label == "" {
					label = b.Label
				}
				if va.IsDefault {
					label += " (default)"
				}
				if label != "" {
					r.printLine("    [%d] %s\n", counter, label)
				} else {
					r.printLine("    [%d]\n", counter)
				}
			}
			for _, line := range strings.Split(strings.TrimRight(va.Content, "\n"), "\n") {
				r.printLine("        %s\n", line)
			}
		}
	}
	fmt.Fprintln(r.stdoutFor())
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
	fmt.Fprintln(r.stdoutFor())
}

func (r *PlainRenderer) RenderSection(section *SectionDef) {
	r.printSection(section.title, section.body)
}

// printSection is the shared formatter the event drain also calls.
func (r *PlainRenderer) printSection(title, body string) {
	w := r.width()
	r.printLine("  --- %s ---\n", title)
	r.printLine("%s\n", wrapText(body, w, "    "))
	fmt.Fprintln(r.stdoutFor())
}

func (r *PlainRenderer) RenderDone() {
	r.printLine("=== Done ===\n")
}

// StreamOutput writes a chunk of step output as it's produced. The
// out writer is the user's actual stdout (captured by Execute before
// captureOutput redirected os.Stdout into the capture pipe); writing
// to os.Stdout directly here would loop straight back into the pipe.
// PlainRenderer doesn't style or buffer per-chunk — this is a
// passthrough that lets long-running steps print live.
func (r *PlainRenderer) StreamOutput(_ int, chunk []byte, out io.Writer) {
	out.Write(chunk)
}

func (r *PlainRenderer) WaitForStep(opts WaitOpts) {
	copyables := r.lastStep.NumberedCopyables()
	if len(copyables) > 0 {
		r.waitWithCopyPrompt(opts, copyables)
		return
	}

	if opts.AutoAcceptAfter <= 0 {
		fmt.Fprint(r.stdoutFor(), "\n    Press Enter to run this step...")
		bufio.NewReader(os.Stdin).ReadString('\n')
		fmt.Fprintln(r.stdoutFor())
		return
	}

	var key byte
	var gotKey bool
	if !opts.ShowCountdown {
		fmt.Fprintf(r.stdoutFor(), "\n    Press Enter to run (auto in %s · any key to hold)...", opts.AutoAcceptAfter.Round(time.Second))
		key, gotKey = WaitForKeyOrTimeout(opts.AutoAcceptAfter, nil)
		fmt.Fprintln(r.stdoutFor())
	} else {
		fmt.Fprintln(r.stdoutFor())
		key, gotKey = WaitForKeyOrTimeout(opts.AutoAcceptAfter, func(remaining time.Duration) {
			bar := countdownBar(remaining, opts.AutoAcceptAfter, 20)
			fmt.Fprintf(r.stdoutFor(), "\r    %s  %4.1fs  (Enter to accept · any key to hold)", bar, remaining.Seconds())
		})
		fmt.Fprintf(r.stdoutFor(), "\r    %s\n", strings.Repeat(" ", 60))
	}

	if !gotKey || key == KeyEnter || key == '\n' {
		return // timer fired or Enter — advance
	}
	// Any other key cancels the countdown. Drop into a cooked-mode
	// "press Enter to continue" hold so the user can read the screen
	// before advancing.
	fmt.Fprint(r.stdoutFor(), "    (countdown stopped — press Enter to continue) ")
	bufio.NewReader(os.Stdin).ReadString('\n')
}

// waitWithCopyPrompt is PlainRenderer's pause when the step exposes
// one or more copyable variants (numbered [1]..[N] in the prior
// RenderStep). Empty Enter advances; a digit in range copies the
// corresponding variant via demokit.Copy and reprints the prompt;
// anything else silently reprints the prompt (the prompt itself
// describes the valid form).
//
// Countdown: the first read is a single-keypress raw-mode race against
// the timer. Enter or timer expiry → advance. Any other key cancels
// the countdown and drops into the cooked-mode loop below; the user
// then types their actual command (digit / Enter) with Enter as
// usual. Matches the TUI's "any key holds" mental model.
func (r *PlainRenderer) waitWithCopyPrompt(opts WaitOpts, copyables []NumberedCopyable) {
	hint := promptFromCopyables(copyables)

	if opts.AutoAcceptAfter > 0 {
		fmt.Fprintf(r.stdoutFor(), "\n    %s · any key holds (auto in %s): ", hint, opts.AutoAcceptAfter.Round(time.Second))
		key, gotKey := WaitForKeyOrTimeout(opts.AutoAcceptAfter, nil)
		fmt.Fprintln(r.stdoutFor()) // newline after the dangling prompt
		if !gotKey {
			return // timer fired — auto-advance
		}
		if key == KeyEnter || key == '\n' {
			return // user accepted with Enter
		}
		// Any other key — fall through to the cooked-mode loop below.
	}

	for {
		fmt.Fprintf(r.stdoutFor(), "\n    %s: ", hint)
		text, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		line := strings.TrimRight(text, "\r\n")

		cmd := strings.TrimSpace(line)
		if cmd == "" {
			return // user accepted with Enter
		}
		// Single digit (or short int) → copy that variant.
		if n, err := strconv.Atoi(cmd); err == nil {
			if n >= 1 && n <= len(copyables) {
				c := copyables[n-1]
				strategy, ok := Copy(c.Content)
				if !ok {
					fmt.Fprintln(r.stdoutFor(), "    (copy failed — no clipboard provider available)")
					continue
				}
				if c.Label != "" {
					fmt.Fprintf(r.stdoutFor(), "    (copied %s via %s)\n", c.Label, strategy)
				} else {
					fmt.Fprintf(r.stdoutFor(), "    (copied via %s)\n", strategy)
				}
				continue
			}
		}
		// Anything else: silently reprompt. The prompt line already
		// shows the valid form; verbose errors would just add noise.
	}
}

// promptFromCopyables builds the one-line digit-to-copy hint shown at
// the pause. Adapts to the count: "[1]" for a single copyable,
// "[1-N]" for multiple.
func promptFromCopyables(copyables []NumberedCopyable) string {
	switch len(copyables) {
	case 0:
		return "Press Enter to run this step"
	case 1:
		return "Enter to run · [1] to copy"
	default:
		return fmt.Sprintf("Enter to run · [1-%d] to copy", len(copyables))
	}
}

// WaitForEnterOrTimeout blocks until the user presses Enter on stdin or
// the timeout elapses. Returns true if Enter was pressed, false if the
// timer fired. Discards the line content — callers that need to inspect
// what the user typed should use WaitForLineOrTimeout instead.
//
// Uses muesli/cancelreader to cancel the pending stdin read when the
// timer wins, so the read goroutine never leaks across successive
// calls — important because a leaked goroutine still blocked on
// os.Stdin can race later prompt reads and steal input.
//
// onTick, if non-nil, is invoked roughly every 100ms with the time
// remaining; renderers use it to redraw a countdown bar.
//
// On platforms where cancelreader is unavailable, the function
// degrades gracefully: it sleeps for the timeout (no Enter shortcut)
// and returns false.
func WaitForEnterOrTimeout(timeout time.Duration, onTick func(remaining time.Duration)) bool {
	_, ok := WaitForLineOrTimeout(timeout, onTick)
	return ok
}

// KeyEnter is the byte returned by WaitForKeyOrTimeout when the user
// presses Enter. On most terminals the literal byte read in raw mode
// is '\r' (carriage return); the constant lets callers compare
// against a stable name. Some terminals (rare) deliver '\n' — the
// public callers should accept both.
const KeyEnter = '\r'

// WaitForKeyOrTimeout puts the terminal in raw mode and reads a single
// byte from stdin racing against the timer. Used by interactive
// countdown prompts that want any-key (not just Enter) to interrupt.
// Returns:
//
//   - (key, true) on the first byte typed. Enter typically arrives as
//     '\r' (KeyEnter); some terminals send '\n'. Callers wanting "any
//     key but Enter" should match both.
//   - (0, false) when the timer fires before any input.
//
// The terminal is restored to its prior cooked-mode state before the
// function returns so a caller can drop into line-based input
// immediately. On platforms where raw mode is unavailable (or stdin
// is not a terminal), falls back to WaitForLineOrTimeout — the user
// has to press Enter to interrupt, but the function still works.
//
// onTick, if non-nil, fires roughly every 100ms with remaining time
// — renderers use it to redraw a countdown bar.
func WaitForKeyOrTimeout(timeout time.Duration, onTick func(remaining time.Duration)) (byte, bool) {
	fd := os.Stdin.Fd()
	if !term.IsTerminal(fd) {
		return fallbackToLineRead(timeout, onTick)
	}
	state, err := term.MakeRaw(fd)
	if err != nil {
		return fallbackToLineRead(timeout, onTick)
	}
	defer term.Restore(fd, state)

	if timeout <= 0 {
		buf := make([]byte, 1)
		n, err := os.Stdin.Read(buf)
		if err != nil || n == 0 {
			return 0, false
		}
		return buf[0], true
	}

	cr, err := cancelreader.NewReader(os.Stdin)
	if err != nil {
		return fallbackToLineRead(timeout, onTick)
	}
	defer cr.Close()

	type keyMsg struct {
		key byte
		ok  bool
	}
	got := make(chan keyMsg, 1)
	go func() {
		buf := make([]byte, 1)
		n, err := cr.Read(buf)
		if err != nil || n == 0 {
			got <- keyMsg{ok: false}
			return
		}
		got <- keyMsg{key: buf[0], ok: true}
	}()

	deadline := time.Now().Add(timeout)
	if onTick == nil {
		select {
		case msg := <-got:
			return msg.key, msg.ok
		case <-time.After(timeout):
			cr.Cancel()
			<-got
			return 0, false
		}
	}

	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			cr.Cancel()
			<-got
			return 0, false
		}
		onTick(remaining)
		select {
		case msg := <-got:
			return msg.key, msg.ok
		case <-tick.C:
		}
	}
}

// fallbackToLineRead is the cooked-mode degradation for
// WaitForKeyOrTimeout on platforms where raw mode is unavailable.
// User must press Enter to interrupt (but the function still works).
func fallbackToLineRead(timeout time.Duration, onTick func(remaining time.Duration)) (byte, bool) {
	line, ok := WaitForLineOrTimeout(timeout, onTick)
	if !ok {
		return 0, false
	}
	if line == "" {
		return KeyEnter, true
	}
	return line[0], true
}

// WaitForLineOrTimeout is the line-returning sibling of
// WaitForEnterOrTimeout. Reads one line from stdin racing against the
// timer; returns ("", false) when the timer fires, (line, true) when
// the user submitted a line (line excludes the trailing newline,
// preserving any other whitespace the user typed).
//
// Used by renderers that combine an auto-advance countdown with an
// interactive command prompt — the returned line lets the caller
// dispatch (e.g. "c" → copy) when the input was non-empty, advance
// when the input was empty, or auto-advance when the timer won.
func WaitForLineOrTimeout(timeout time.Duration, onTick func(remaining time.Duration)) (string, bool) {
	if timeout <= 0 {
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		return strings.TrimRight(line, "\r\n"), true
	}
	cr, err := cancelreader.NewReader(os.Stdin)
	if err != nil {
		time.Sleep(timeout)
		return "", false
	}
	defer cr.Close()

	type lineMsg struct {
		text string
		ok   bool
	}
	got := make(chan lineMsg, 1)
	go func() {
		text, err := bufio.NewReader(cr).ReadString('\n')
		if err != nil {
			got <- lineMsg{ok: false}
			return
		}
		got <- lineMsg{text: strings.TrimRight(text, "\r\n"), ok: true}
	}()

	deadline := time.Now().Add(timeout)
	if onTick == nil {
		select {
		case msg := <-got:
			return msg.text, msg.ok
		case <-time.After(timeout):
			cr.Cancel()
			<-got
			return "", false
		}
	}

	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			cr.Cancel()
			<-got
			return "", false
		}
		onTick(remaining)
		select {
		case msg := <-got:
			return msg.text, msg.ok
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
			fmt.Fprintln(r.stdoutFor(), "    Re-enter values (Enter to keep [bracketed]):")
		}
		result := map[string]any{}
		errored := false
		for i, in := range pending {
			label := in.Prompt
			if label == "" {
				label = in.Name
			}
			if in.Default != nil {
				fmt.Fprintf(r.stdoutFor(), "    %s [%v]: ", label, in.Default)
			} else {
				fmt.Fprintf(r.stdoutFor(), "    %s: ", label)
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
				fmt.Fprintf(r.stdoutFor(), "    [error] %v\n", err)
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

// --- EventAwareRenderer (demokit.EventAwareRenderer) ---

// AttachEventQueue wires the demokit event queue and spawns the
// drain goroutine. Execute calls this once per run; once attached,
// PlainRenderer drains discrete events on its own goroutine and
// Execute stops dual-calling the legacy Render* methods. Chunks
// still flow live via StreamOutput (see struct doc).
//
// Resets per-run state so a renderer reused across Execute calls
// (tests do this) starts each run with a fresh drain rather than
// inheriting `done` from a previous Done event.
func (r *PlainRenderer) AttachEventQueue(q *events.EventQueue) {
	r.queue = q
	r.done = false
	r.lastStepEv = nil
	r.boxedFlag = false
	// Snapshot the real terminal stdout/stderr under stdoutMu.RLock
	// (per #23) so the drain has stable writers that don't race with
	// captureOutput's later swaps.
	stdoutMu.RLock()
	r.snapOut = os.Stdout
	r.snapErr = os.Stderr
	stdoutMu.RUnlock()
	r.drainWG.Add(1)
	go func() {
		defer r.drainWG.Done()
		r.drainEvents()
	}()
}

// stdoutFor returns the writer the drain (and legacy path) should
// write to. Drain uses the snapshot; legacy falls back to live
// os.Stdout. Same shape for stderr-side queries via stderrFile.
func (r *PlainRenderer) stdoutFor() io.Writer {
	if r.snapOut != nil {
		return r.snapOut
	}
	return os.Stdout
}

func (r *PlainRenderer) stdoutFile() *os.File {
	if r.snapOut != nil {
		return r.snapOut
	}
	return os.Stdout
}

func (r *PlainRenderer) stderrFile() *os.File {
	if r.snapErr != nil {
		return r.snapErr
	}
	return os.Stderr
}

// Finish waits for the drain goroutine to exit. Execute calls this
// (via the FinishableRenderer interface) after emitting Done so the
// renderer's writes are sequenced before Execute returns — needed
// for the race detector and for test hygiene.
func (r *PlainRenderer) Finish() {
	r.drainWG.Wait()
}

// drainEvents subscribes to the queue and dispatches events.
// Catch-up drain before blocking on Notify — same lesson as the
// notebookbridge (issue 40): the first events are usually appended
// before the subscribe goroutine wakes. The drain exits when
// handleEvent processes a Done event (sets r.done), so the goroutine
// doesn't leak across Execute calls or test runs.
func (r *PlainRenderer) drainEvents() {
	sub := r.queue.Subscribe()
	defer sub.Close()
	offset := r.drainFrom(0)
	for !r.done {
		<-sub.Notify()
		offset = r.drainFrom(offset)
	}
}

func (r *PlainRenderer) drainFrom(offset int) int {
	evs, newOff := r.queue.ReadFrom(offset)
	for i, ev := range evs {
		r.handleEvent(offset+i, ev)
	}
	return newOff
}

// handleEvent dispatches one event. OutputChunk + StepReadyToRun
// are intentionally no-ops here — chunks tee live via StreamOutput.
// No outer stdoutMu lock needed: the drain writes through
// snapshotted *os.File handles (r.snapOut / r.snapErr) taken at
// AttachEventQueue time, so it never touches the live os.Stdout
// variable that captureOutput mutates.
func (r *PlainRenderer) handleEvent(off int, ev events.Event) {
	switch e := ev.(type) {
	case events.Header:
		r.boxedFlag = e.BoxedVerbatim
		r.RenderHeader(e.Title, e.Description, e.StepCount)
	case events.Section:
		r.printSection(e.Title, e.Body)
	case events.StepStart:
		stepCopy := e
		r.lastStepEv = &stepCopy
		r.printStepBlock(e.Visit, e.Declared, e, r.boxedFlag)
	case events.StepEnd:
		// Output already streamed via StreamOutput's inline tee in
		// Execute, so RenderResult receives "" (matches today's
		// displayOutput="" path for streaming/event-aware).
		r.RenderResult(e.Visit, "", StepResultFromEvent(e))
	case events.Done:
		r.RenderDone()
		r.done = true
	case events.WaitForAdvance:
		// Skip if already resolved (e.g. non-interactive paths where
		// demokit Resolves the offset itself before the drain reaches
		// it). Otherwise the drain would block on stdin for a wait
		// nobody is waiting on.
		if _, resolved := r.queue.Resolution(off); resolved {
			return
		}
		opts := WaitOpts{}
		if !e.Deadline.IsZero() {
			opts.AutoAcceptAfter = time.Until(e.Deadline)
		}
		r.handleWaitForAdvance(opts)
		_ = r.queue.Resolve(off, &events.AdvanceResolution{
			Source: "user-submitted", Timestamp: time.Now(),
		})
	case events.PromptOpen:
		// Same as WaitForAdvance: demokit pre-resolves with defaults in
		// non-interactive mode. If already resolved, don't try to read
		// stdin — there's nothing to collect for.
		if _, resolved := r.queue.Resolution(off); resolved {
			return
		}
		answers := r.handlePromptOpen(e.Inputs)
		_ = r.queue.Resolve(off, &events.PromptResolution{
			Answers: answers, Source: "user-submitted", Timestamp: time.Now(),
		})
	}
}

// handleWaitForAdvance is the event-side analogue of WaitForStep.
// Builds copyables from the cached StepStart event so the digit-to-
// copy prompt matches the legacy path's numbering.
func (r *PlainRenderer) handleWaitForAdvance(opts WaitOpts) {
	var copyables []NumberedCopyable
	if r.lastStepEv != nil {
		copyables = numberedCopyablesFromVerbatims(r.lastStepEv.Verbatims, r.boxedFlag)
	}
	if len(copyables) > 0 {
		r.waitWithCopyPrompt(opts, copyables)
		return
	}
	// Fall through to the plain Enter pause — same shape as the
	// non-copyable branch of WaitForStep.
	if opts.AutoAcceptAfter <= 0 {
		fmt.Fprint(r.stdoutFor(), "\n    Press Enter to run this step...")
		bufio.NewReader(os.Stdin).ReadString('\n')
		fmt.Fprintln(r.stdoutFor())
		return
	}
	var key byte
	var gotKey bool
	if !opts.ShowCountdown {
		fmt.Fprintf(r.stdoutFor(), "\n    Press Enter to run (auto in %s · any key to hold)...", opts.AutoAcceptAfter.Round(time.Second))
		key, gotKey = WaitForKeyOrTimeout(opts.AutoAcceptAfter, nil)
		fmt.Fprintln(r.stdoutFor())
	} else {
		fmt.Fprintln(r.stdoutFor())
		key, gotKey = WaitForKeyOrTimeout(opts.AutoAcceptAfter, func(remaining time.Duration) {
			bar := countdownBar(remaining, opts.AutoAcceptAfter, 20)
			fmt.Fprintf(r.stdoutFor(), "\r    %s  %4.1fs  (Enter to accept · any key to hold)", bar, remaining.Seconds())
		})
		fmt.Fprintf(r.stdoutFor(), "\r    %s\n", strings.Repeat(" ", 60))
	}
	if !gotKey || key == KeyEnter || key == '\n' {
		return
	}
	fmt.Fprint(r.stdoutFor(), "    (countdown stopped — press Enter to continue) ")
	bufio.NewReader(os.Stdin).ReadString('\n')
}

// handlePromptOpen runs the line-mode prompt loop on event-side
// inputs, returning the collected answers. Mirrors PlainRenderer.Prompt
// but reads from the events.Input projection (which carries the same
// name/prompt/default/Parse surface via the typed events.IntInput /
// ChoiceInput / StringInput types).
func (r *PlainRenderer) handlePromptOpen(inputs []events.Input) map[string]any {
	out := make(map[string]any, len(inputs))
	reader := bufio.NewReader(os.Stdin)
	for _, in := range inputs {
		for {
			prompt := in.InputPrompt()
			if def := in.InputDefault(); def != nil {
				fmt.Fprintf(r.stdoutFor(), "    %s [%v]: ", prompt, def)
			} else {
				fmt.Fprintf(r.stdoutFor(), "    %s: ", prompt)
			}
			raw, err := reader.ReadString('\n')
			if err != nil {
				out[in.InputName()] = in.InputDefault()
				break
			}
			raw = strings.TrimRight(raw, "\r\n")
			if raw == "" && in.InputDefault() != nil {
				out[in.InputName()] = in.InputDefault()
				break
			}
			val, err := parseEventInput(in, raw)
			if err != nil {
				fmt.Fprintf(r.stdoutFor(), "    %v\n", err)
				continue
			}
			out[in.InputName()] = val
			break
		}
	}
	return out
}

// numberedCopyablesFromVerbatims rebuilds NumberedCopyable entries
// from the events.Verbatim projection. Mirrors StepDef.NumberedCopyables
// so the event-aware copy prompt numbers match the legacy path exactly.
func numberedCopyablesFromVerbatims(blocks []events.Verbatim, boxedDefault bool) []NumberedCopyable {
	var out []NumberedCopyable
	for _, b := range blocks {
		if len(b.Variants) == 0 {
			continue
		}
		if !(boxedDefault || len(b.Variants) > 1) {
			continue
		}
		for _, va := range b.Variants {
			label := va.Label
			if label == "" {
				label = b.Label
			}
			out = append(out, NumberedCopyable{
				N:       len(out) + 1,
				Label:   label,
				Lang:    va.Lang,
				Content: va.Content,
			})
		}
	}
	return out
}

// StepResultFromEvent rebuilds a *StepResult from a StepEnd event.
// Returns nil for an "ok" terminal with no message/error, matching
// the legacy "no result was returned by Run" sentinel that
// RenderResult interprets as a plain success. Exported so any
// event-aware renderer (plain, web bridge, future tui) can
// reconstruct the legacy result shape without duplicating the
// status-string parse.
func StepResultFromEvent(e events.StepEnd) *StepResult {
	if e.Status == "ok" && e.Message == "" && e.ErrorText == "" {
		return nil
	}
	res := &StepResult{
		Status:  parseStatus(e.Status),
		Message: e.Message,
	}
	if e.ErrorText != "" {
		res.Err = fmt.Errorf("%s", e.ErrorText)
	}
	return res
}

func parseStatus(s string) ResultStatus {
	switch s {
	case "error":
		return StatusError
	case "warning":
		return StatusWarning
	case "info":
		return StatusInfo
	default:
		return StatusSuccess
	}
}

// parseEventInput interprets the user's raw line for one events.Input
// projection, mirroring the InputDef.Parse semantics for the three
// built-in shapes. Unknown types fall through to a string.
func parseEventInput(in events.Input, raw string) (any, error) {
	switch v := in.(type) {
	case events.IntInput:
		_ = v
		n, err := strconv.Atoi(raw)
		if err != nil {
			return nil, fmt.Errorf("not an integer: %v", err)
		}
		return n, nil
	case events.ChoiceInput:
		for _, opt := range v.Options {
			if opt == raw {
				return raw, nil
			}
		}
		return nil, fmt.Errorf("must be one of %v", v.Options)
	default:
		return raw, nil
	}
}
