package main

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/panyam/demokit/notebook"
	"github.com/panyam/demokit/notebook/cells"
)

// seriesRe parses `series <expr> from <a> to <b>` plus optional
// trailing `step <s>` / `rate <ms>` clauses in either order. <a>
// / <b> / <s> may themselves be expressions (`pi/4`) — they're
// evaluated through the same Env as the body. `series` shares
// plot's grammar shape (`<expr> from <a> to <b>`) so users only
// need to learn one form.
var seriesRe = regexp.MustCompile(`^\s*series\s+(.+?)\s+from\s+(.+?)\s+to\s+(\S+(?:\s+\S+)*?)((?:\s+(?:step|rate)\s+\S+)*)\s*$`)

// optionRe extracts trailing `key value` pairs from the regex's
// optional suffix group. Keys are `step` and `rate`; everything
// else is rejected by the outer parse before this runs.
var optionRe = regexp.MustCompile(`(step|rate)\s+(\S+)`)

// defaultSeriesTickInterval is the pause between successive
// evaluations when the user doesn't specify a `rate`. Long enough
// for the user to register each new line as it arrives; short
// enough that a 50-point series finishes in a few seconds. The
// notebook's repaint tick is 16ms so any cadence >= one tick is
// renderable.
const defaultSeriesTickInterval = 60 * time.Millisecond

// seriesController tracks the in-flight `series` runs so a
// keybinding (Ctrl+C) can cancel them as a group without quitting
// the notebook. Each registered run has a context the goroutine
// selects on between iterations.
//
// Multi-series cancellation is all-or-nothing in v1 — there's no
// "cancel just this one" UX yet. Apps that need per-cell cancel
// can extend by keying off the focused cell.
type seriesController struct {
	mu      sync.Mutex
	running map[notebook.CellID]context.CancelFunc
}

func newSeriesController() *seriesController {
	return &seriesController{running: map[notebook.CellID]context.CancelFunc{}}
}

func (s *seriesController) register(id notebook.CellID, cancel context.CancelFunc) {
	s.mu.Lock()
	s.running[id] = cancel
	s.mu.Unlock()
}

func (s *seriesController) finish(id notebook.CellID) {
	s.mu.Lock()
	delete(s.running, id)
	s.mu.Unlock()
}

// cancelAll cancels every running series and returns true if any
// were cancelled. Used by the Ctrl+C binding to consume the key
// when there's work to interrupt.
func (s *seriesController) cancelAll() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.running) == 0 {
		return false
	}
	for id, cancel := range s.running {
		cancel()
		delete(s.running, id)
	}
	return true
}

// seriesOptions carries the parsed trailing `step` / `rate`
// clauses. Both are optional; defaults come from defaultSeriesStep
// (count ≈ 40) and defaultSeriesTickInterval (60ms).
type seriesOptions struct {
	stepRaw string
	rateRaw string
}

// parseSeriesSrc parses src into its (expr, from, to, options)
// pieces. Returns an error with the syntax line on mismatch so
// the caller can surface it as a ResultCell.
func parseSeriesSrc(src string) (expr, fromRaw, toRaw string, opts seriesOptions, err error) {
	m := seriesRe.FindStringSubmatch(src)
	if m == nil {
		err = fmt.Errorf("series syntax: series <expr> from <a> to <b> [step <s>] [rate <ms>]")
		return
	}
	expr = strings.TrimSpace(m[1])
	fromRaw = strings.TrimSpace(m[2])
	toRaw = strings.TrimSpace(m[3])
	for _, kv := range optionRe.FindAllStringSubmatch(m[4], -1) {
		switch kv[1] {
		case "step":
			opts.stepRaw = kv[2]
		case "rate":
			opts.rateRaw = kv[2]
		}
	}
	return
}

// runSeries appends an OutputCell sized to the full point count
// (so the cell grows row-by-row without ever capping into in-cell
// scroll), then evaluates expr at successive x values from a to
// b in its own goroutine, streaming one line per point with a
// short sleep so the user sees the series assemble live.
//
// The "x" name is bound in env on every iteration; tests / callers
// that want a different free variable can extend this to take a
// name argument. expr referencing other variables (constants like
// pi, prior bindings) works as-is.
//
// ctl is optional. When non-nil, the spawned goroutine registers
// its cancel func with it so a key binding can cancel running
// series as a group (see seriesController.cancelAll).
//
// Returns nil on success; on parse / eval errors returns the
// error so the caller can surface it as a ResultCell (matching
// plot's failure UX).
func runSeries(nb *notebook.Notebook, ctl *seriesController, n int, src string, env *Env) error {
	expr, fromRaw, toRaw, opts, err := parseSeriesSrc(src)
	if err != nil {
		return err
	}
	aVal, err := env.Eval(fromRaw)
	if err != nil {
		return fmt.Errorf("series 'from': %s", err)
	}
	bVal, err := env.Eval(toRaw)
	if err != nil {
		return fmt.Errorf("series 'to': %s", err)
	}
	a := AsFloat(aVal)
	b := AsFloat(bVal)
	if math.IsNaN(a) || math.IsNaN(b) {
		return fmt.Errorf("series bounds must be numeric")
	}
	step := defaultSeriesStep(a, b)
	if opts.stepRaw != "" {
		stepVal, err := env.Eval(opts.stepRaw)
		if err != nil {
			return fmt.Errorf("series 'step': %s", err)
		}
		step = AsFloat(stepVal)
		if math.IsNaN(step) || step == 0 {
			return fmt.Errorf("series step must be a non-zero number")
		}
	}
	if (b > a && step < 0) || (b < a && step > 0) {
		return fmt.Errorf("series step direction disagrees with from / to (%g .. %g step %g)", a, b, step)
	}

	rate := defaultSeriesTickInterval
	if opts.rateRaw != "" {
		ms, err := strconv.Atoi(strings.TrimSpace(opts.rateRaw))
		if err != nil || ms < 0 {
			return fmt.Errorf("series rate must be a non-negative integer (milliseconds)")
		}
		rate = time.Duration(ms) * time.Millisecond
	}

	count := stepCount(a, b, step)
	if count <= 0 {
		return fmt.Errorf("series range produces no points (a=%g b=%g step=%g)", a, b, step)
	}

	id := notebook.CellID(fmt.Sprintf("series-%d", n))
	// Sizing maxBody to count + 2 (header + N value lines + room
	// for a (cancelled) tail line) keeps the cell fully expanded
	// as it grows — no in-cell scroll, the notebook viewport
	// handles overflow between cells.
	oc := cells.NewOutput(string(id), count+2)
	if _, err := nb.Append(oc); err != nil {
		return err
	}
	w := nb.Stream(id)
	fmt.Fprintf(w, "%s  (x from %g to %g step %g, %d points, rate %s)\n",
		expr, a, b, step, count, rate)

	ctx, cancel := context.WithCancel(context.Background())
	if ctl != nil {
		ctl.register(id, cancel)
	}
	go func() {
		streamSeries(ctx, nb, id, w, expr, a, b, step, count, rate, env)
		if ctl != nil {
			ctl.finish(id)
		}
		cancel()
	}()
	return nil
}

// streamSeries runs the eval loop on its own goroutine so the
// notebook stays interactive while the series fills in. Honors
// ctx cancellation: on Done, it emits a "(cancelled)" tail line
// and returns.
func streamSeries(ctx context.Context, nb *notebook.Notebook, id notebook.CellID, w interface {
	Write([]byte) (int, error)
}, expr string, a, b, step float64, count int, rate time.Duration, env *Env) {
	for i := 0; i < count; i++ {
		select {
		case <-ctx.Done():
			fmt.Fprintln(w, "  (cancelled)")
			markSeriesDone(nb, id)
			return
		default:
		}
		x := a + float64(i)*step
		// Guard against floating-point drift on the last point so
		// `series x from 0 to 10 step 1` ends exactly at 10.
		if (step > 0 && x > b) || (step < 0 && x < b) {
			x = b
		}
		env.SetVar("x", x)
		val, err := env.Eval(expr)
		if err != nil {
			fmt.Fprintf(w, "  x=%-8g  err: %s\n", x, err)
		} else {
			fmt.Fprintf(w, "  x=%-8g  -> %s\n", x, formatValue(val))
		}
		if rate <= 0 || i == count-1 {
			continue
		}
		select {
		case <-time.After(rate):
		case <-ctx.Done():
			fmt.Fprintln(w, "  (cancelled)")
			markSeriesDone(nb, id)
			return
		}
	}
	markSeriesDone(nb, id)
}

func markSeriesDone(nb *notebook.Notebook, id notebook.CellID) {
	nb.Update(id, func(c notebook.Cell) notebook.Cell {
		if oc, ok := c.(*cells.OutputCell); ok {
			oc.MarkDone()
		}
		return c
	})
}

// defaultSeriesStep picks a step that gives a comfortable point
// count (~40) when the user doesn't specify one. Keeps the
// streaming visible — too few points and there's nothing to
// stream, too many and it drags.
func defaultSeriesStep(a, b float64) float64 {
	const target = 40.0
	span := b - a
	if span == 0 {
		return 1
	}
	return span / target
}

// stepCount returns the number of points produced by stepping
// from a to b at the given step (inclusive of a and b). Robust
// to floating-point drift — uses integer math on the rounded
// span/step ratio.
//
// Edge cases: a == b is a single point; mismatched directions
// (positive span / negative step or vice versa) return 0.
func stepCount(a, b, step float64) int {
	if a == b {
		return 1
	}
	span := b - a
	if (span > 0) != (step > 0) {
		return 0
	}
	n := int(math.Round(span/step)) + 1
	if n < 0 {
		return 0
	}
	return n
}

// parseSeriesStep is exported for tests in the same package.
// (Public only by lowercase-package convention in `main`.)
func parseSeriesStep(s string) (float64, error) {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, err
	}
	return f, nil
}
