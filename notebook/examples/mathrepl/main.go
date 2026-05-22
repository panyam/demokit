// mathrepl is a standalone math REPL built on the notebook
// component — no demokit anywhere. Type expressions to see
// results; type `plot <expr> from <a> to <b>` for a braille graph;
// `series <expr> from <a> to <b>` to stream a table of values
// line-by-line (exercises the OutputCell streaming path); `q`
// quits.
//
// The point of the demo is to prove that notebook is genuinely
// reusable: a third-party consumer with its own custom Cell
// types (ResultCell, PlotCell) drives the same Notebook API the
// demokit bridge will use.
package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/panyam/demokit/notebook"
	"github.com/panyam/demokit/notebook/cells"
)

func main() {
	env := NewEnv()
	seriesCtl := newSeriesController()

	// Override Ctrl+C: when any series is streaming, cancel
	// them all and stay running (so the prompt remains usable).
	// With no series in flight, fall through to Quit — preserves
	// the default UX for non-series workflows.
	km := notebook.DefaultKeyMap()
	km.Global["ctrl+c"] = func(nb *notebook.Notebook) tea.Cmd {
		if seriesCtl.cancelAll() {
			return nil
		}
		return notebook.Quit(nb)
	}

	nb := notebook.New(
		notebook.WithPromptFactory(cells.PromptFactory()),
		notebook.WithKeyMap(km),
	)

	// The REPL drives the notebook from a goroutine; the BT
	// program runs in main and blocks until Stop. Standard
	// bubbletea-driven-from-outside pattern.
	go runREPL(nb, env, seriesCtl)
	if err := nb.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runREPL(nb *notebook.Notebook, env *Env, seriesCtl *seriesController) {
	nb.SetHeader("Math Notebook", "expressions · plot · series · q quits")
	nb.Append(cells.NewNote("intro", "How to use", introBody()))

	n := 0
	for {
		n++
		resp := nb.AwaitInput([]notebook.Input{
			notebook.NewStringInput("expr", "λ", nil),
		})
		if resp.Source == "cancelled" {
			return
		}
		src, _ := resp.Answers["expr"].(string)
		src = strings.TrimSpace(src)

		switch {
		case src == "":
			// empty submit — ignore, the next iteration prompts again
		case src == "q" || src == "quit":
			nb.SetDone()
			nb.Stop()
			return
		case strings.HasPrefix(src, "plot "):
			cell, err := NewPlotCell(fmt.Sprintf("plot-%d", n), src, env)
			if err != nil {
				appendResultCell(nb, n, src, "", err)
				continue
			}
			nb.Append(cell)
		case strings.HasPrefix(src, "series "):
			// `series <e> from <a> to <b>` — streams f(x) values
			// into an OutputCell one row at a time. The eval loop
			// runs in its own goroutine so the notebook stays
			// interactive while the series fills in.
			if err := runSeries(nb, seriesCtl, n, src, env); err != nil {
				appendResultCell(nb, n, src, "", err)
			}
		case strings.HasPrefix(src, "lines "):
			// `lines N` — appends an OutputCell with N generated
			// lines. Use it to exercise scroll + drag-select
			// copy from a body that overflows maxBody.
			countStr := strings.TrimSpace(strings.TrimPrefix(src, "lines"))
			count, err := strconv.Atoi(countStr)
			if err != nil || count <= 0 {
				appendResultCell(nb, n, src, "", fmt.Errorf("usage: lines <positive integer>"))
				continue
			}
			appendStressCell(nb, n, count)
		default:
			val, err := env.Eval(src)
			appendResultCell(nb, n, src, formatValue(val), err)
		}
	}
}

func appendResultCell(nb *notebook.Notebook, n int, src, value string, err error) {
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	nb.Append(NewResult(fmt.Sprintf("res-%d", n), src, value, msg))
}

// appendStressCell creates an OutputCell with N generated lines.
// Use it to exercise mouse wheel scrolling + drag-select copy on
// a body that overflows the cell's maxBody. The cell defaults to
// HorizontalEdges (no left/right borders) so drag-selection
// across the body doesn't pick up vertical bar characters.
func appendStressCell(nb *notebook.Notebook, n, count int) {
	oc := cells.NewOutput(fmt.Sprintf("lines-%d", n), 12)
	oc.SetFallbackClipboard(notebook.FileClipboard(""))
	id, err := nb.Append(oc)
	if err != nil {
		return
	}
	w := nb.Stream(id)
	for i := 1; i <= count; i++ {
		fmt.Fprintf(w, "line %4d  : the quick brown fox jumps over the lazy dog\n", i)
	}
}

func formatValue(v any) string {
	switch x := v.(type) {
	case nil:
		return "<nil>"
	case string:
		return x
	default:
		return fmt.Sprintf("%v", x)
	}
}

func introBody() string {
	return strings.Join([]string{
		"Try:",
		"  2 + 2 * sin(pi/2)",
		"  x = 5",
		"  x * x",
		"  plot sin(x) from 0 to pi*2",
		"  series x*x from 1 to 20                 # streams 20 lines (60ms per row) — watch them arrive live",
		"  series sin(x) from 0 to pi*2 step pi/24",
		"  series x from 1 to 200 rate 25          # 25ms cadence; Ctrl+C cancels a running series",
		"  lines 50          # streaming-cell stress test (mouse wheel scrolls, drag-select copies)",
		"",
		"Type q to quit. Available: sin cos tan sqrt log exp abs floor ceil pow · pi e",
	}, "\n")
}
