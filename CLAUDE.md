# CLAUDE.md — demokit

## Quick ref

```bash
go test ./...                                    # run all tests
go run ./examples/basic/                         # interactive plain demo
go run ./examples/basic/ --tui                   # interactive TUI demo (smooth scroll)
go run ./examples/basic/ --smooth                # plain text with smooth scroll
go run ./examples/basic/ --non-interactive       # no pauses
go run ./examples/basic/ --readme                # generate README markdown
```

## Architecture

- **`demokit.go`** — core: Demo, StepDef, StepResult, Renderer, PlainRenderer, TermWidth, captureOutput
- **`tui/`** — Lipgloss TUI renderer (styled boxes, palette, width)
- **`examples/basic/`** — showcase example with Makefile

See README.md for usage API and install instructions.

## Conventions

- `Renderer` interface decouples presentation from execution
- `Run(fn func() *StepResult)` — named returns for ergonomics: bare `return` = success
- `StepResult` carries `Status/Label/Message/Err`; renderers style by status
- Builder pattern: all setters return receiver for chaining
- StepDef fields unexported; read accessors for external renderers
- Data between steps: shared closure variables (no framework plumbing)
- Both renderers are terminal-width-aware (`Fraction`/`MaxWidth` fields)

## Gotchas

- `Note()`/`Ref()` are setters — read accessors are `NoteText()`/`Refs()`
- `lipgloss.Color()` in v2 is a func returning `color.Color`, not a type — use `image/color.Color`
- `print()` writes to stderr — use `fmt.Print()` for capturable output
- `.gitignore` pattern `basic` matches dirs — use `/basic` for root binary only
- `term.GetSize(os.Stdout.Fd())` fails when piped (e.g. `make`) — `TermWidth()` falls back to stderr
- Smooth scroll: `Delay < 0` disables in tests; TUI default 18ms, PlainRenderer opt-in
