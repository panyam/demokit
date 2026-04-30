package demokit

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

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

// WaitForEnterOrTimeout blocks until the user presses Enter on stdin or
// the timeout elapses. Returns true if Enter was pressed, false if the
// timer fired. Uses muesli/cancelreader to cancel the pending stdin
// read when the timer wins, so the read goroutine never leaks across
// successive calls — important because a leaked goroutine still
// blocked on os.Stdin can race later prompt reads and steal input.
//
// onTick, if non-nil, is invoked roughly every 100ms with the time
// remaining; renderers use it to redraw a countdown bar.
//
// On platforms where cancelreader is unavailable, the function
// degrades gracefully: it sleeps for the timeout (no Enter shortcut)
// and returns false. The pending-read leak is avoided either way.
func WaitForEnterOrTimeout(timeout time.Duration, onTick func(remaining time.Duration)) bool {
	if timeout <= 0 {
		bufio.NewReader(os.Stdin).ReadString('\n')
		return true
	}
	cr, err := cancelreader.NewReader(os.Stdin)
	if err != nil {
		// Platform unsupported — sleep and give up the Enter shortcut
		// rather than leak a goroutine.
		time.Sleep(timeout)
		return false
	}
	defer cr.Close()

	enter := make(chan struct{}, 1)
	go func() {
		bufio.NewReader(cr).ReadString('\n')
		enter <- struct{}{}
	}()

	deadline := time.Now().Add(timeout)
	if onTick == nil {
		select {
		case <-enter:
			return true
		case <-time.After(timeout):
			cr.Cancel()
			<-enter
			return false
		}
	}

	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			cr.Cancel()
			<-enter
			return false
		}
		onTick(remaining)
		select {
		case <-enter:
			return true
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
