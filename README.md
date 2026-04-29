# demokit

Interactive step-through framework for runnable Go examples. Define steps with mermaid sequence diagram arrows, explanatory sections, and RFC references — then run interactively in the CLI or generate README.md from the same source.

## Features

- **Interactive CLI** — pause between steps, show diagram arrows and references
- **TUI mode** — styled terminal boxes via Lipgloss (`tui/` subpackage) with distinct colors for step numbers, titles, arrows, notes, refs, and results
- **Pluggable renderers** — `Renderer` interface lets you swap presentation without touching demo logic
- **Non-interactive mode** — `--non-interactive` for CI / full output
- **README generation** — `--readme` outputs markdown with mermaid diagrams, step descriptions, and deduped reference links
- **Single source of truth** — steps, arrows, notes, and references defined once in Go code
- **Sections** — arbitrary markdown blocks between steps (explanations, tables, code snippets)
- **References** — `Ref` type for linking to RFCs, CVEs, specs, blog posts per step
- **Output capture** — step output is captured and rendered inside styled result boxes
- **Status-aware results** — `StepResult` with Success/Error/Warning/Info status, custom labels, and per-status styling
- **Dynamic width** — both renderers adapt to terminal width (configurable fraction + max cap)
- **Smooth scroll** — new content scrolls in line-by-line for a polished demo feel (TUI default, plain opt-in)

## Usage

```go
package main

import "github.com/panyam/demokit"

func main() {
    demo := demokit.New("My Example").
        Dir("01-my-example").
        Description("What this demonstrates").
        Actors(
            demokit.Actor("Client", "Client App"),
            demokit.Actor("Server", "API Server"),
        )

    demo.Step("Send a request").
        Ref(demokit.Ref{Name: "RFC 7231", URL: "https://www.rfc-editor.org/rfc/rfc7231"}).
        Arrow("Client", "Server", "GET /api/data").
        DashedArrow("Server", "Client", "200 {data}").
        Note("The client sends a request and gets a response.").
        Run(func() (result *demokit.StepResult) {
            fmt.Println("    Making request...")
            return // nil = success
        })

    demo.Section("Why this matters",
        "This demonstrates the basic request/response pattern.",
        "",
        "In production, you'd add authentication headers.",
    )

    demo.Execute()
}
```

### TUI mode

Use the `tui` subpackage for styled terminal output with Lipgloss:

```go
import "github.com/panyam/demokit/tui"

// Add --tui flag support, or set it directly:
demo.WithRenderer(tui.New())
```

The TUI renderer shows each step, section, and result in distinct colored boxes with differentiated styling for step numbers, titles, arrows, notes, and references.

## Run modes

```bash
go run ./examples/basic/                    # interactive (pauses between steps)
go run ./examples/basic/ --tui              # interactive with styled TUI boxes + smooth scroll
go run ./examples/basic/ --smooth           # plain text with smooth scroll
go run ./examples/basic/ --non-interactive  # full output, no pauses
go run ./examples/basic/ --readme           # generate README.md
```

## Install

```bash
go get github.com/panyam/demokit
```
