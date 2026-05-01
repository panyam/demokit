# Architecture

This document describes how demokit is wired together internally — the files, the contracts between them, and the pitfalls that matter when extending it. For *what* demokit does and how to use it, see [README.md](README.md).

## Module map

| File | Responsibility |
|---|---|
| `demokit.go` | `Demo` builder, CLI flag parsing, the `Execute` loop, doc-emit dispatch |
| `step.go` | `StepDef`, `SectionDef`, `ActorDef`, `Ref`, `ArrowView` — the static definition types |
| `inputs.go` | `InputDef` + `String/Int/Choice/WithDefault` helpers |
| `result.go` | `StepResult`, `StepContext`, `ResultStatus`, convenience constructors (`Errf`, `Warn`, `Info`) |
| `recorder.go` | `TraceEntry`, `Recorder` interface, `MemoryRecorder`, `JSONFileRecorder`, `LoadTrace` |
| `renderer.go` | `Renderer` interface + `PlainRenderer` (default zero-dep stdout renderer) |
| `markdown.go` | `Demo.Markdown()` — the static-visitor markdown emitter (used by `--doc md` without `--from`) |
| `render.go` | `RenderContext{Demo, Trace, State}` + `EntryOpts{StepNumber}` — the contract every doc renderer consumes |
| `render_trace.go` | `RenderEntryMD/HTML`, `RenderDocumentMD/HTML` |
| `render_json.go` | `RenderDocumentJSON`, `JSONFromTrace`, `Demo.JSON()` + projection view structs |
| `markdown_load.go` | `Demo.FromMarkdown(path)` + `Demo.FromMarkdownBytes` + `Demo.Bind(id)` — sidecar markdown loader |
| `web/` | Subpackage `package web`. Go-side embed surface (`TraceFragment`, `WriteBundle`, `PlayerJS`, `PlayerCSS`); registers `--doc bundle` with core via `init()`. Imported via `_ "github.com/panyam/demokit/web"` to enable bundle output. |
| `web/player/` | Vanilla-JS Custom Element (`<demokit-demo>`) + scoped CSS, embedded into the `web` package binary via `//go:embed`. |
| `term.go` | Terminal width detection with stdout/stderr fallback |
| `logger.go` | Internal `print()` helper (writes to stderr — see gotchas) |
| `tui/` | Lipgloss-based renderer + `FormPrompter` interface |

## Render contract

Every documentation renderer (markdown, HTML, JSON, future templ/React plugins) is a pure function of `RenderContext{Demo, Trace, State}`:

- **Static** (`Trace == nil`): walks `Demo` definition only. Used by `Demo.Markdown()` and `Demo.JSON()`.
- **Trace** (`Trace == loaded entries`): post-hoc record from `--record`. Used by `RenderDocument*(ctx)`.
- **Live** (future): partial trace + state snapshot; reserved for WebSocket embeds.

Renderers **never invoke `Run()`**. Computed values are produced by `Execute`, persisted to the trace, and consumed downstream. This guarantees:

- Renderers are deterministic and side-effect-free.
- Multiple format renderers consume the same trace without re-executing the demo.
- A renderer can run in a different process (web server, embed host).

The trace renderers (md, html) are layered into a per-entry primitive and a whole-document shell — `RenderEntryMD/HTML` for self-contained fragments (no preamble, no aggregations), `RenderDocumentMD/HTML` for full documents with title preamble, `## Walkthrough` header, and deduplicated references footer. JSON is a single document function (per-entry JSON would be a trivial `json.Marshal(entry)` and not worth wrapping).

## Sidecar markdown

A demo's *content* (titles, notes, mermaid arrows, refs, declared inputs, sections) can live in a companion markdown file loaded via `Demo.FromMarkdown(path)`. *Behavior* — `Run`, `Coalesce`, custom `Parse` closures — stays in Go and attaches to loaded steps via `Demo.Bind(id)`. The sidecar is **optional and additive**: every demo can be expressed inline via `Step()`/`Note()`/`Run()` without a markdown file. Sidecar mode is for demos where prose dominates and the content is easier to edit in markdown than in Go.

### Single source of truth: `StepDef`

Both paths converge on the same struct. Markdown populates `StepDef` fields at `FromMarkdown` time; Go setters mutate the same fields at `Bind(id)` chain time; `Run` reads them at execution. There is no separate "sidecar StepDef" type.

```
Md (frontmatter / fenced blocks) ─┐
                                  ├─→ StepDef ──→ Execute / Renderer reads
Go setter (Bind(id).XYZ(v))     ──┘             ↗
                                                 
Run(ctx) closure                ───────────────→ reads via ctx.Inputs
```

### Override semantics, by field shape

| Field shape | Md sets | Go setter does |
|---|---|---|
| **Scalar** (note, title, future per-step `autoAcceptAfter`) | initial value | replace (latest wins) |
| **Keyed list** (inputs, keyed by `Name`) | initial list | replace-by-key, else append |
| **Plain list** (refs, future `tags`) | initial list | append |
| **Closure** (Run, Coalesce, Parse) | (md has no closures) | set; only Go can |

### File format

```markdown
---
title: ...
description: ...
actors:
  - { id: A, label: Alpha }
---

## Section title {#section-id}

Prose-only section body.

## Step title {#step-id}

> Blockquote becomes the step's note (multi-paragraph supported).

​```mermaid
A ->> B: solid arrow
B -->> A: dashed arrow
​```

​```inputs
- name: x
  type: string|int|choice
  options: [a, b, c]   # only for choice
  default: ...
​```

​```refs
- name: RFC ...
  url: https://...
​```
```

### Conventions

- **Heading anchor `{#id}` is the join key.** CommonMark extension; renders cleanly on GitHub. Without an explicit anchor, the title is slugified.
- **Step vs section is decided by content shape.** A heading with any of [blockquote note, mermaid arrows, refs, inputs] is a step; prose-only headings are sections. Bind only steps.
- **Three reserved fenced info-strings:** `mermaid`, `inputs`, `refs`. Other fenced blocks pass through as section body for future renderers.
- **Mermaid features beyond `->>` / `-->>`** (`participant`, `Note over`, `alt`, `loop`, `autonumber`) are silently dropped with a load warning. demokit's model is arrow-only.

### Examples

| Directory | Mode | Stresses |
|---|---|---|
| `examples/basic/` | inline | inline-only API; regression check that the Go-only path keeps working |
| `examples/graph/` | inline | branching state machine, `Coalesce`, `AutoAcceptAfter` countdown |
| `examples/dungeon/` | sidecar | `FromMarkdownBytes` + `go:embed`, `Bind`, multiple cycles, `MaxVisits` guard, `int` input, Go-side state (the magic ring), cancellable streaming step (`listen`), live ANSI dragon scene |

## Long-running steps: timeout + cancellation

Steps that consume infinite or near-infinite streams (event listeners, polling loops, "wait for press-Enter") need a way to end. `StepDef.Timeout(d)` and `StepDef.Cancellable(b)` are two orthogonal knobs:

| Setter | Effect |
|---|---|
| `Timeout(d)` | After `d` elapses, `ctx.Ctx` fires `Done()`. Run notices and returns. |
| `Cancellable(true)` | In interactive mode, an Enter keypress cancels `ctx.Ctx`. Has no effect in `--non-interactive` or replay mode. |
| both | Whichever fires first cancels — Enter wins if pressed early; timeout wins otherwise. |
| neither (default) | `ctx.Ctx` is a never-cancelled background context. Run executes uninterrupted. |

**Run is responsible for honoring cancellation.** demokit does NOT abandon a Run that ignores the context — the demo blocks until Run returns. The standard pattern:

```go
demo.Step("Watch events").
    Timeout(5 * time.Minute).
    Cancellable(true).
    Run(func(ctx demokit.StepContext) *demokit.StepResult {
        for {
            select {
            case ev := <-events:
                fmt.Printf("[event] %v\n", ev)  // streams live
            case <-ctx.Ctx.Done():
                fmt.Println("flushing...")     // cleanup also streams
                saveSummary()
                return demokit.Info("watched")
            }
        }
    })
```

Practical implication: the user sees the demo paused for however long Run's cleanup phase takes after `<-ctx.Done()` fires. Keep cleanup fast or print progress chunks during it.

If Run returns with no result and the context fired during execution, demokit synthesizes an `Info` with `"step timed out"` or `"step cancelled"` so the trace records why the step ended.

## Sidecar load errors

`FromMarkdown` never panics or returns an error directly — it stores any failure on the `Demo` and the chained-call surface stays clean. `Demo.Execute()` checks for stored errors at startup and aborts with a clear stderr message before any step runs:

- File not found / permission denied
- Malformed frontmatter YAML or unterminated frontmatter
- Unknown input `type`
- `Bind(id)` to an id that doesn't appear in the markdown

Load warnings (unsupported mermaid syntax, content before the first heading) print to stderr at Execute start but don't abort.

## Doc-emit dispatch (`--doc <format>` + `--from <path>`)

| Flag | `--from`? | Renderer |
|---|---|---|
| `--doc md` | no | `Demo.Markdown()` (rich static visitor) |
| `--doc md` | yes | `RenderDocumentMD(ctx)` (per-entry layered) |
| `--doc html` | no | `RenderDocumentHTML(ctx)` (minimal: title + description) |
| `--doc html` | yes | `RenderDocumentHTML(ctx)` |
| `--doc json` | no | `RenderDocumentJSON(ctx)` (definition only) |
| `--doc json` | yes | `RenderDocumentJSON(ctx)` (definition + trace) |
| `--doc bundle [--out path]` | either | `web.WriteBundle(d, entries, path)` — self-contained HTML with player + CSS + trace inlined. Requires `_ "github.com/panyam/demokit/web"` import (registers via `RegisterDocFormat`). |

Static-md and trace-md route to **different renderers** because they walk fundamentally different sources (declarations vs. recorded entries) and produce intentionally different shapes. The static visitor includes a "What you'll learn" notes summary, a consolidated mermaid sequence diagram, and a Run-it footer; the trace renderer produces a per-step walkthrough with captured outputs and inputs.

### Doc-format registry (`RegisterDocFormat`)

Built-in formats (`md`/`html`/`json`) are hard-coded in core. Additional formats register themselves at `init()` time so demos opt in via blank import:

```go
// in demokit/web/web.go
func init() {
    demokit.RegisterDocFormat("bundle", func(d *demokit.Demo, entries []demokit.TraceEntry, out string) error {
        return WriteBundle(d, entries, out)
    })
}

// in your demo's main.go
import _ "github.com/panyam/demokit/web"
```

Demos that don't import the package see a clear stderr error if they invoke `--doc bundle`: `"demokit: --doc bundle is not enabled. Add `_ \"github.com/panyam/demokit/web\"` to your imports."`. Names `md`, `html`, `json` are reserved; reusing them panics.

Reasoning for the indirection: keeping the Go-side embed code (TraceFragment, WriteBundle, PlayerJS) in its own subpackage matches the layout of the JS/CSS assets under `web/player/`. The `tui/` subpackage uses the same shape (separate package; demos opt in via explicit `tui.New()`). Without the registry, `--doc bundle` would have to leave core CLI; with it, the unified `--doc <format>` surface stays intact.

## Embed surface — `<demokit-demo>` web player

The player is a vanilla-JS Custom Element shipped under `web/player/` and embedded into the demokit binary via `//go:embed`. It consumes the same JSON shape `--doc json` produces — there's one trace contract for terminal renders, doc gen, and embeds.

Three (soon four) source modes, in priority order:

| Mode | Host invokes via | Use case |
|---|---|---|
| Programmatic | `el.trace = traceObject` (JS property) | Dynamic insertion, framework integrations |
| URL static | `<demokit-demo data-src="trace.json">` | Published demos, demokit.com/traces/xyz, slide CDNs |
| Inline blob | `<demokit-demo>{...JSON...}</demokit-demo>` | Self-contained HTML, file://, copy-paste embeds |
| URL live *(reserved; PR for issue #3)* | `<demokit-demo data-src="..." data-mode="live">` | Live presentations served by `demokit --serve` |

Four Go entry points in the `web` subpackage:

```go
import "github.com/panyam/demokit/web"

web.WriteBundle(d, entries, "/path/out.html")  // self-contained HTML
web.TraceFragment(d, entries)                  // <demokit-demo>{...}</demokit-demo>
web.PlayerJS()                                 // raw bundled player JS for hosting at your own URL
web.PlayerCSS()
```

A blank import `import _ "github.com/panyam/demokit/web"` is enough to enable `--doc bundle` on the CLI side without exposing the package's symbols.

The player events / public methods are documented at the top of `web/player/demokit-player.js`.

### Why no shadow DOM

Hosts (notably slide tools that retheme) rely on CSS custom-property inheritance. Shadow DOM would block that. The player uses BEM-ish scoped class names plus `--demokit-*` CSS variables; revisit if real style-bleed reports appear.


## Gotchas

### API shape

- `Note()`/`Ref()` are setters — readers are `NoteText()`/`Refs()`.
- `ID(s)` is a setter; reader is `StepID()` (Go has no overloading).
- `WithDefault(v)` is the chainable setter; the underlying field is `Default`.

### Output capture

- `print()` (in `logger.go`) writes to **stderr** — use `fmt.Print()` for capturable stdout.
- `term.GetSize(os.Stdout.Fd())` fails when stdout is piped — `TermWidth()` falls back to stderr.

### Trace round-trip

- JSON unmarshal widens numeric inputs to `float64`. Tests handle both int and float forms.
- Replay forces deterministic `Next`: the user's `Run` can return anything, but the recorded `Next` wins so refactored demos still replay the original path.

### Control flow

- It's a **directed graph**, not a DAG — cycles are allowed (hence `MaxVisits`). Two guardrails: `Demo.MaxSteps(n)` (default 200) caps total visits; `Demo.MaxVisits(n)` caps per-step revisits.

### Stdin handling

- Cancellable stdin reads use `muesli/cancelreader`, not bare goroutines. Go has no portable stdin deadline; `os.Stdin.SetReadDeadline` is unreliable on terminals across macOS/Windows. See `WaitForEnterOrTimeout` in `renderer.go`.

### Example READMEs

- Regenerate via `make gen-readme` in `examples/basic/` (static linear via `--doc md`) and `examples/graph/` (trace-driven via `--doc md --from`).
