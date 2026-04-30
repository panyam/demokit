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
| `render_trace.go` | `RenderEntryMD/HTML`, `RenderDocumentMD/HTML`, legacy `MarkdownFromTrace`/`HTMLFromTrace` wrappers |
| `render_json.go` | `RenderDocumentJSON`, `JSONFromTrace`, `Demo.JSON()` + projection view structs |
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

## Doc-emit dispatch (`--doc <format>` + `--from <path>`)

| Flag | `--from`? | Renderer |
|---|---|---|
| `--doc md` | no | `Demo.Markdown()` (rich static visitor) |
| `--doc md` | yes | `RenderDocumentMD(ctx)` (per-entry layered) |
| `--doc html` | no | `RenderDocumentHTML(ctx)` (minimal: title + description) |
| `--doc html` | yes | `RenderDocumentHTML(ctx)` |
| `--doc json` | no | `RenderDocumentJSON(ctx)` (definition only) |
| `--doc json` | yes | `RenderDocumentJSON(ctx)` (definition + trace) |

Static-md and trace-md route to **different renderers** because they walk fundamentally different sources (declarations vs. recorded entries) and produce intentionally different shapes. The static visitor includes a "What you'll learn" notes summary, a consolidated mermaid sequence diagram, and a Run-it footer; the trace renderer produces a per-step walkthrough with captured outputs and inputs.

Legacy flags `--readme`, `--readme-from`, `--readme-html-from` still work but print a one-line deprecation warning to stderr.

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
