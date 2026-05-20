# mathrepl

A standalone math REPL built on the `notebook` component — no
`demokit` anywhere. Demonstrates that the notebook is reusable
for third-party Go TUI apps and that custom `Cell` types
(`ResultCell`, `PlotCell`) just work.

```bash
cd notebook/examples/mathrepl
go run .
```

## Commands

- Expressions: `2 + 2 * sin(pi/2)`, `sqrt(2)`, `pow(2, 10)`
- Variables: `x = 5` then `x * x`
- Plots: `plot sin(x) from 0 to pi*2`
- Quit: `q` or `quit`

## What it shows off

| Notebook feature | Used by |
|---|---|
| `notebook.New` + `Run` / `Stop` | `main.go` |
| `notebook.AwaitInput` (via `cells.PromptFactory`) | the REPL read loop |
| `notebook.Append` | results, plots, the intro note |
| `notebook.SetHeader` / `SetDone` | banner + done indicator |
| **Custom `notebook.Cell` implementations** | `ResultCell`, `PlotCell` |
| Built-in `cells.NoteCell` | the intro |

The braille plot lives in `plot.go` + `braille.go` — about 200
lines total. `plot.go`'s `PlotCell` implements `notebook.Cell`
directly, with no notebook-package changes needed.

## Module structure

`mathrepl` is its own Go module so its dependencies (notably
[`expr-lang/expr`](https://github.com/expr-lang/expr)) stay
isolated from the `notebook` module.
