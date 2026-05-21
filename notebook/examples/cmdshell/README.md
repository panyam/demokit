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

## Vim-style `:` command bar

Press `:` from navigation mode (Esc out of the prompt first if
the cursor is sitting in it) to open a `CommandCell` docked at
the viewport bottom. Type a shell command and Enter — same
plumbing as the prompt path, but the input lives outside the
main list. Esc cancels.

The command bar **grows as you type**: short commands stay one
row, long pipelines wrap and push the bar up. When the bar
would oversubscribe the screen it's clamped and its tail (where
the cursor is) keeps rendering.

`":"` is wired in `main.go` via:

```go
km := notebook.DefaultKeyMap()
km.Modes[notebook.NavigationMode][":"] = func(nb *notebook.Notebook) tea.Cmd {
    cells.OpenCommandBar(nb, ":", repl.runFromCommandBar)
    return nil
}
```

`cells.OpenCommandBar` installs the command cell at
`notebook.Bottom`, focuses it, restores the prior dock (the
default `StatusCell`) on Enter / Esc. Apps with their own status
bar get that bar back automatically — `OpenCommandBar` snapshots
whatever was there at open time, not "the default."

## Cells used

| Cell | Role |
|---|---|
| `cells.NewHeader` (via `SetHeader`) | banner |
| `cells.NewNote` | one intro cell with usage hints |
| `cells.NewPrompt` (via `AwaitInput`) | the `$` prompt |
| `cells.NewOutput` (via `Stream`) | per-command output, one per cmd |
| `cells.CommandCell` (via `OpenCommandBar`) | `:` vim-style command bar at the Bottom dock |

Plus the `OSC52Clipboard` for `c`-to-copy when a cell is focused
and the built-in `StatusCell` (auto-installed at `notebook.Bottom`
on `New`).

All wiring lives in a single ~140-line `main.go`.
