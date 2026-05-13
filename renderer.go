package demokit

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/muesli/cancelreader"
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

	// lastStep is set by RenderStep and consulted by WaitForStep to
	// build the digit-to-copy prompt from the step's numbered copyable
	// variants. Cleared at the next RenderStep so a step with no
	// copyables falls through to the plain Enter pause.
	lastStep *StepDef
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
	r.lastStep = step
	w := r.width()
	// totalSteps == 0 means "no denominator" — Demo.ShowStepDenominator
	// defaults off because the count is misleading for cyclic graphs.
	// stepNum > totalSteps is a belt-and-suspenders fallback if a demo
	// opts in but ends up cyclic anyway.
	if totalSteps == 0 || stepNum > totalSteps {
		r.printLine("  Step %d: %s\n", stepNum, step.title)
	} else {
		r.printLine("  Step %d/%d: %s\n", stepNum, totalSteps, step.title)
	}
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

	r.printVerbatim(step)
}

// printVerbatim emits a step's verbatim blocks in plain-text form.
// Layout:
//
//	  <Block label>
//
//	    [N] variant-label (default)
//	        content line 1
//	        content line 2
//
//	    [N+1] other-variant
//	        ...
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
func (r *PlainRenderer) printVerbatim(step *StepDef) {
	blocks := step.VerbatimBlocks()
	if len(blocks) == 0 {
		return
	}
	demo := step.Demo()
	boxedDefault := demo != nil && demo.IsBoxedVerbatim()

	counter := 0
	for _, b := range blocks {
		if len(b.Variants) == 0 {
			continue
		}
		copyable := boxedDefault || len(b.Variants) > 1

		fmt.Println()
		if b.Label != "" {
			r.printLine("  %s\n", b.Label)
		}
		for _, va := range b.Variants {
			fmt.Println()
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
	fmt.Println()
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
		fmt.Print("\n    Press Enter to run this step...")
		bufio.NewReader(os.Stdin).ReadString('\n')
		fmt.Println()
		return
	}

	if !opts.ShowCountdown {
		fmt.Printf("\n    Press Enter to run (auto in %s)...", opts.AutoAcceptAfter.Round(time.Second))
		WaitForEnterOrTimeout(opts.AutoAcceptAfter, nil)
		fmt.Println()
		return
	}

	fmt.Println()
	WaitForEnterOrTimeout(opts.AutoAcceptAfter, func(remaining time.Duration) {
		bar := countdownBar(remaining, opts.AutoAcceptAfter, 20)
		fmt.Printf("\r    %s  %4.1fs  (Enter to accept now)", bar, remaining.Seconds())
	})
	fmt.Printf("\r    %s\n", strings.Repeat(" ", 60))
}

// waitWithCopyPrompt is PlainRenderer's pause when the step exposes
// one or more copyable variants (numbered [1]..[N] in the prior
// RenderStep). Empty Enter advances; a digit in range copies the
// corresponding variant via demokit.Copy and reprints the prompt;
// anything else silently reprints the prompt (the prompt itself
// describes the valid form).
//
// Countdown: any non-empty input cancels the auto-advance (the user
// signaled interest in interacting). Once cancelled, the loop runs
// without the timer until empty Enter.
func (r *PlainRenderer) waitWithCopyPrompt(opts WaitOpts, copyables []NumberedCopyable) {
	hint := promptFromCopyables(copyables)

	// First read may race the countdown if AutoAcceptAfter > 0.
	deadline := opts.AutoAcceptAfter
	for {
		var line string
		var gotInput bool
		if deadline > 0 {
			fmt.Printf("\n    %s (auto in %s): ", hint, deadline.Round(time.Second))
			line, gotInput = WaitForLineOrTimeout(deadline, nil)
			if !gotInput {
				fmt.Println() // newline after the dangling prompt
				return        // timer fired — auto-advance
			}
			deadline = 0 // user signalled intent; subsequent reads are pure line mode
		} else {
			fmt.Printf("\n    %s: ", hint)
			text, _ := bufio.NewReader(os.Stdin).ReadString('\n')
			line = strings.TrimRight(text, "\r\n")
		}

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
					fmt.Println("    (copy failed — no clipboard provider available)")
					continue
				}
				if c.Label != "" {
					fmt.Printf("    (copied %s via %s)\n", c.Label, strategy)
				} else {
					fmt.Printf("    (copied via %s)\n", strategy)
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
