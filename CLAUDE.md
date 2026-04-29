# CLAUDE.md — demokit

See [README.md](README.md) for what demokit is and how to use it.
See [examples/graph/main.go](examples/graph/main.go) for a working branching demo.

## Quick ref

```bash
go test ./...
go run ./examples/graph/                                  # branching demo
go run ./examples/graph/ --record /tmp/run.json           # save a trace
go run ./examples/graph/ --replay /tmp/run.json           # replay it
go run ./examples/graph/ --readme-from /tmp/run.json      # markdown of path

# Per-example README regeneration (checked into repo)
cd examples/basic && make gen-readme
cd examples/graph && make gen-readme
```

## Files

- `demokit.go` — Demo, StepDef, StepResult, StepContext, Execute loop, PlainRenderer
- `inputs.go` — InputDef + helpers (String/Int/Choice/WithDefault)
- `recorder.go` — TraceEntry, Recorder, JSONFileRecorder, LoadTrace
- `render_trace.go` — MarkdownFromTrace, HTMLFromTrace
- `tui/` — Lipgloss renderer + FormPrompter interface

## Gotchas

- `Note()`/`Ref()` are setters — readers are `NoteText()`/`Refs()`
- `ID(s)` is a setter; reader is `StepID()` (no Go overloading)
- `WithDefault` not `Default` on `InputDef` (Default is the public field)
- `print()` writes to stderr — use `fmt.Print()` for capturable output
- `term.GetSize(os.Stdout.Fd())` fails when piped — `TermWidth()` falls back to stderr
- JSON round-trip widens numeric inputs to float64 — tests handle both
- It's a **directed graph**, not a DAG — cycles allowed (hence MaxVisits)
- Replay forces deterministic Next: user's Run can return anything, recorded Next wins
- Cancellable stdin reads use `muesli/cancelreader` (not bare goroutines) — Go has no portable stdin deadline; `os.Stdin.SetReadDeadline` is unreliable on terminals across macOS/Windows. See `WaitForEnterOrTimeout` in demokit.go.
- Regenerate example READMEs with `make gen-readme` in `examples/basic/` (static linear via `--readme`) and `examples/graph/` (trace-driven via `--readme-from`)

## Open polish

CLI arg parsing in `Demo.Execute` is ad-hoc `os.Args` walking. Refactor to stdlib `flag` with a `RegisterFlags(*flag.FlagSet)` hook so demos with their own flags compose cleanly. Tracked as a separate task in this PR.
