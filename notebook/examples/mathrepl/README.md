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
- Series (streamed line-by-line): `series x*x from 1 to 20`,
  `series sin(x) from 0 to pi*2 step pi/24`,
  `series x from 1 to 200 rate 25` (faster cadence — 25ms per row)
- Cancel a running series: **Ctrl+C** while it streams. With no
  series in flight, Ctrl+C quits (default behavior).
- Stress test: `lines 200`
- Quit: `q` or `quit`

## What `series` proves

`series <expr> from <a> to <b> [step <s>] [rate <ms>]` exists
specifically to validate the notebook's streaming-output path
end-to-end. Each evaluation prints one line into a fresh
`OutputCell` with a `rate`-ms pause between rows (default 60ms);
the cell is sized to the full point count so every row stays
visible (no in-cell scroll), and the notebook viewport handles
overflow between cells. Watch the table fill in row-by-row to
confirm `OutputBuffer` → repaint tick → `RenderRows` streams
correctly under your terminal.

The eval loop runs in its own goroutine, so the prompt stays
interactive while the series fills in — you can queue another
command on top of a running series. Press **Ctrl+C** to cancel
every running series (the in-cell tail line shows `(cancelled)`);
with no series running, Ctrl+C exits the notebook as usual. The
override lives in `main.go` — a one-line `KeyMap.Global`
replacement that consults the series controller before falling
back to `notebook.Quit`.

## What it shows off

| Notebook feature | Used by |
|---|---|
| `notebook.New` + `Run` / `Stop` | `main.go` |
| `notebook.AwaitInput` (via `cells.PromptFactory`) | the REPL read loop |
| `notebook.Append` | results, plots, the intro note |
| `notebook.SetHeader` / `SetDone` | banner + done indicator |
| **Custom `notebook.Cell` implementations** | `ResultCell`, `PlotCell` |
| Built-in `cells.NoteCell` | the intro |
| **Streaming via `notebook.Stream(id)`** | `lines N`, `series ...` |
| `cells.OutputCell` (with maxBody sized to the full count) | `lines`, `series` |

The braille plot lives in `plot.go` + `braille.go` — about 200
lines total. `plot.go`'s `PlotCell` implements `notebook.Cell`
directly, with no notebook-package changes needed.

## Module structure

`mathrepl` is its own Go module so its dependencies (notably
[`expr-lang/expr`](https://github.com/expr-lang/expr)) stay
isolated from the `notebook` module.
