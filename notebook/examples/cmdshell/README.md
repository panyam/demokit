# cmdshell

A tiny shell wrapper built on the `notebook` component. Each
command you type runs through `sh -c <cmd>` with combined
stdout/stderr streaming into a fresh `OutputCell` in real time.

```bash
cd notebook/examples/cmdshell
make run
```

## Why it exists

`mathrepl` exercises the notebook with fixed-shape result cells
(short results, fixed-height plots). `cmdshell` lets you throw
arbitrary output at it — `ls`, `ps aux`, `find /`, `tail -f`,
`seq 1 100000`. Useful for testing:

- **Wheel scrolling** inside an `OutputCell` whose buffer overflows
  `maxBody`.
- **Drag-select + copy** from a streaming body. `OutputCell`
  defaults to `HorizontalEdges` (top/bottom borders only) so
  selections don't pick up vertical bar characters.
- **Viewport resize** — resize the terminal mid-command and watch
  cells re-flow to the new width.
- **Long-running streams** that don't fit in one screen.

## Commands

- Type any shell command. Output streams into a new
  `OutputCell` named `cmd-N`. The cell shows "·live" until the
  command exits, then "·end".
- `clear` removes every cmd cell (start fresh).
- `q` or `quit` exits.

## Cells used

| Cell | Role |
|---|---|
| `cells.NewHeader` (via `SetHeader`) | banner |
| `cells.NewNote` | one intro cell with usage hints |
| `cells.NewPrompt` (via `AwaitInput`) | the `$` prompt |
| `cells.NewOutput` (via `Stream`) | per-command output, one per cmd |

Plus the `OSC52Clipboard` for `c`-to-copy when a cell is focused.

All wiring lives in a single ~100-line `main.go`.
