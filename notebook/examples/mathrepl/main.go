// mathrepl is a standalone math REPL built on the notebook
// component — no demokit anywhere. Type expressions to see
// results; type `plot <expr> from <a> to <b>` for a braille graph;
// `q` quits.
//
// The point of the demo is to prove that notebook is genuinely
// reusable: a third-party consumer with its own custom Cell
// types (ResultCell, PlotCell) drives the same Notebook API the
// demokit bridge will use.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/panyam/demokit/notebook"
	"github.com/panyam/demokit/notebook/cells"
)

func main() {
	env := NewEnv()
	nb := notebook.New(
		notebook.WithPromptFactory(cells.PromptFactory()),
	)

	// The REPL drives the notebook from a goroutine; the BT
	// program runs in main and blocks until Stop. Standard
	// bubbletea-driven-from-outside pattern.
	go runREPL(nb, env)
	if err := nb.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runREPL(nb *notebook.Notebook, env *Env) {
	nb.SetHeader("Math Notebook", "expressions · plot <e> from <a> to <b> · q quits")
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
		"",
		"Type q to quit. Available: sin cos tan sqrt log exp abs floor ceil pow · pi e",
	}, "\n")
}
