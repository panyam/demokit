# CLAUDE.md — demokit

## Quick ref

```bash
go test ./...                                    # run all tests
go run ./examples/basic/                         # interactive plain demo
go run ./examples/basic/ --tui                   # interactive TUI demo
go run ./examples/basic/ --non-interactive       # no pauses
go run ./examples/basic/ --readme                # generate README markdown
```

## Architecture

- **`demokit.go`** — core library: Demo, StepDef, SectionDef, Renderer interface, PlainRenderer, captureOutput
- **`tui/`** — Lipgloss-based TUI renderer (styled boxes, configurable palette/width)
- **`examples/basic/`** — showcase example with Makefile

See README.md for usage API and install instructions.

## Conventions

- `Renderer` interface decouples presentation from execution — add new renderers without touching core
- Core package (`demokit`) has zero external dependencies; Lipgloss is isolated in `tui/` subpackage
- StepDef fields are unexported; read accessors (`Title()`, `NoteText()`, `Refs()`, `Arrows()`) exist for external renderers
- `captureOutput()` redirects stdout during `runFn()` so renderers can display results in styled boxes
- Builder pattern: all setters return receiver for chaining

## Gotchas

- `Note()` and `Ref()` are setters on StepDef — read accessors are `NoteText()` and `Refs()` to avoid conflict
- `lipgloss.Color()` in v2 is a function returning `color.Color`, not a type — use `image/color.Color` for struct fields
- Go's builtin `print()` writes to stderr, not stdout — use `fmt.Print()` for output that needs capturing
