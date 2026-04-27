# demokit

Interactive step-through framework for runnable Go examples. Define steps with mermaid sequence diagram arrows, explanatory sections, and RFC references — then run interactively in the CLI or generate README.md from the same source.

## Features

- **Interactive CLI** — pause between steps, show diagram arrows and references
- **Non-interactive mode** — `--non-interactive` for CI / full output
- **README generation** — `--readme` outputs markdown with mermaid diagrams, step descriptions, and deduped reference links
- **Single source of truth** — steps, arrows, notes, and references defined once in Go code
- **Sections** — arbitrary markdown blocks between steps (explanations, tables, code snippets)
- **References** — `Ref` type for linking to RFCs, CVEs, specs, blog posts per step

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
        Run(func() {
            fmt.Println("    Making request...")
        })

    demo.Section("Why this matters",
        "This demonstrates the basic request/response pattern.",
        "",
        "In production, you'd add authentication headers.",
    )

    demo.Execute()
}
```

## Run modes

```bash
go run ./examples/01-my-example/                    # interactive (pauses between steps)
go run ./examples/01-my-example/ --non-interactive   # full output, no pauses
go run ./examples/01-my-example/ --readme            # generate README.md
```

## Install

```bash
go get github.com/panyam/demokit
```
