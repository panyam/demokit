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

### Doc-format registry (`Demo.RegisterDocFormat`) and `Demo.RegisterServeHandler`

Built-in formats (`md`/`html`/`json`) are hard-coded in core. Additional formats — and the `--serve` handler — are **per-instance** registries: methods on `*Demo`, not package globals. Demos opt in by calling a setup function from the helper package:

```go
// in demokit/web/web.go
func RegisterWith(d *demokit.Demo) {
    d.RegisterDocFormat("bundle", func(d *demokit.Demo, entries []demokit.TraceEntry, out string) error {
        return WriteBundle(d, entries, out)
    })
    d.RegisterServeHandler(func(d *demokit.Demo, addr string) error {
        return ServeHTTP(d, addr)
    })
}

// in your demo's main.go
import "github.com/panyam/demokit/web"

demo := demokit.New("...")
// ... define steps ...
web.RegisterWith(demo)
demo.Execute()
```

Demos that don't call `web.RegisterWith` see clear stderr errors if they invoke `--doc bundle` or `--serve`: `"demokit: --doc bundle is not enabled. Call web.RegisterWith(demo) before Execute (import github.com/panyam/demokit/web)."`. Names `md`, `html`, `json` are reserved; reusing them panics.

Reasoning for instance-scoped: multiple demos in one process don't collide; tests don't share state across runs; the explicit `web.RegisterWith(demo)` line is grep-able evidence of opt-in (vs. a silent `init()`). The `tui/` subpackage uses the same shape (separate package; demos opt in via explicit `tui.New()`).

## Embed surface — `<demokit-demo>` web player

The player is a vanilla-JS Custom Element shipped under `web/player/` and embedded into the demokit binary via `//go:embed`. It consumes the same JSON shape `--doc json` produces — there's one trace contract for terminal renders, doc gen, and embeds.

Three (soon four) source modes, in priority order:

| Mode | Host invokes via | Use case |
|---|---|---|
| Programmatic | `el.trace = traceObject` (JS property) | Dynamic insertion, framework integrations |
| URL static | `<demokit-demo data-src="trace.json">` | Published demos, demokit.com/traces/xyz, slide CDNs |
| Inline blob | `<demokit-demo>{...JSON...}</demokit-demo>` | Self-contained HTML, file://, copy-paste embeds |
| URL live | `<demokit-demo data-src="/events" data-mode="live">` | Live presentations served by `demokit --serve` |

Four Go entry points in the `web` subpackage:

```go
import "github.com/panyam/demokit/web"

web.WriteBundle(d, entries, "/path/out.html")  // self-contained HTML
web.TraceFragment(d, entries)                  // <demokit-demo>{...}</demokit-demo>
web.PlayerJS()                                 // raw bundled player JS for hosting at your own URL
web.PlayerCSS()
```

`web.RegisterWith(demo)` (called before `Execute`) installs the per-instance handlers for `--doc bundle` and `--serve`. Registries are scoped to the `*Demo` (not package globals) so multiple demos in one process can be configured independently and tests don't leak state.

The player events / public methods are documented at the top of `web/player/demokit-player.js`.

### Live mode (`--serve <addr>`)

`web.ServeHTTP` runs the demo behind a small HTTP+WebSocket server:

- `GET /` — `live.html` template, instantiates `<demokit-demo data-src="/events" data-mode="live">` and the embedded player + ansi_up assets.
- `GET /demokit-player.{js,css}`, `/ansi_up.js` — sibling assets (linked, not inlined; `bundle.html` mode is the inlined sibling).
- `GET /trace.json` — current run's history as a JSON document for debugging or post-hoc replay.
- `WS /events` — bidirectional channel. Server pushes structured events (`header` / `section` / `step-start` / `chunk` / `step-end` / `input-needed` / `input-timeout` / `done` / `reset`); the client posts `{kind:"input", values:{...}}` and `{kind:"reset"}`.

A few invariants worth knowing:

- **The live demo run has its own renderer** (`webRenderer`) which **tees onto whatever renderer the caller configured** before `--serve` (default `PlainRenderer`, or `tui.Renderer` if `--tui` was also passed). Framing methods delegate; `Prompt` and `WaitForStep` stay WS-only because the inner versions would read the operator's stdin.
- **No stdin in serve mode.** Demokit's `RunLoop` keys off `flagServe != ""` to suppress both the between-step Enter-pause and the `Cancellable` Enter-watcher. The browser drives advancement; stray operator keystrokes don't cancel steps.
- **`captureOutput` runs as in CLI mode** — chunks reach `webRenderer.StreamOutput`, which writes to the snapshotted-pre-capture stdout (operator terminal) AND broadcasts a `chunk` event over WS.
- **Aborts surface in the player.** `MaxSteps` / unknown-`Next` paths in `RunLoop` call `RenderResult` without a preceding `RenderStep`. `webRenderer` tracks `stepOpen` and synthesizes a "Aborted" `step-start` so the error is visible in the live UI rather than silently truncating.
- **Shutdown.** `gohttp.ListenAndServeGraceful` traps SIGINT/SIGTERM; the `WithOnShutdown` callback cancels the demo's run context (which `Prompt` selects on, unblocking it) and force-closes WS connections by calling `liveConn.forceClose()` (closes the underlying `*websocket.Conn` so gorilla's `ReadMessage` returns and the handler unwinds — `BaseConn.OnClose` alone only stops the writer goroutine).
- **`RunLoop` vs `Execute`.** `runDemo` calls `Demo.RunLoop()` rather than `Demo.Execute()` so the `--serve` flag dispatch isn't re-entered (which would otherwise recurse infinitely).
- **Reset.** Clients post `{kind:"reset"}`; `liveServer.reset()` cancels the current `runCtx` (unblocking any pending `Prompt` via its select), waits on `runDone` for the goroutine to exit, drains `srv.inputs`, broadcasts a `reset` event, clears `srv.history`, and re-launches `runDemo`. Concurrent resets are serialized via `runMu`. Steps that ignore `ctx.Ctx.Done()` block reset until they return naturally — same contract as `Cancellable`.
- **Input timeouts.** `Demo.InputTimeout(d)` (or `--input-timeout d`) sets a default deadline; `StepDef.InputTimeout(d)` overrides per-step. `webRenderer.Prompt` adds a `time.After` case to its select; on timeout, declared defaults from each `InputDef` fill the inputs map and an `input-timeout` event broadcasts so the player can dismiss its form. `Demo.EffectiveInputTimeout(stepID)` resolves the effective value (per-step beats demo).

### Why no shadow DOM

Hosts (notably slide tools that retheme) rely on CSS custom-property inheritance. Shadow DOM would block that. The player uses BEM-ish scoped class names plus `--demokit-*` CSS variables; revisit if real style-bleed reports appear.


## Notebook — standalone cell-based TUI component

`notebook/` is a **separate Go module** (`github.com/panyam/demokit/notebook`) that has no dependency on demokit. It's a reusable widget toolkit for cell-based terminal UIs — REPLs, wizards, log viewers, anything that wants a navigable list of typed cells. The demokit bridge (planned phase 4) will sit on top of it; today the only consumer is `notebook/examples/mathrepl/` (the math REPL with braille plots).

### Multi-module workspace

```
demokit/                              module github.com/panyam/demokit
├── go.work                           ., ./notebook, ./notebook/examples/mathrepl
├── Makefile                          MODULES := . notebook notebook/examples/mathrepl
├── notebook/
│   ├── go.mod                        module github.com/panyam/demokit/notebook
│   │                                 deps: charm libs only (no demokit, no events)
│   ├── cells/                        built-in widget library
│   └── examples/mathrepl/
│       ├── go.mod                    deps: notebook + expr-lang
│       └── Makefile                  run/build/test/race/tidy
```

`make testall` / `make race` / `make tidy` loop the modules; `go test ./...` from any root is module-scoped (won't descend across `go.mod`).

### Notebook package layout

| File | Responsibility |
|---|---|
| `notebook.go` | `Notebook` struct, `New`, `Run`, `Stop`, options (`WithKeyMap`, `WithClipboard`, `WithPromptFactory`, `WithHeadless`, `WithSize`), `Snapshot` |
| `model.go` | `tea.Model` — viewport, mode, key & mouse dispatch, render |
| `store.go` | `store` — shared RWMutex-guarded cells + cursor + header |
| `crud.go` | `Append` / `Insert` / `Update` / `Remove` / `Get` / `IndexOf` / `Len` / `SetCursor` / `FocusCell` / `SetHeader` / `SetDone` |
| `rendezvous.go` | `AwaitInput` / `AwaitInputBy` (blocking primitives) + `InputResponse` |
| `stream.go` | `Stream(id) io.Writer` over a cell's `OutputBuffer` |
| `keymap.go` | `KeyMap`, `Action`, built-in actions (`Quit` / `NavUp` / `NavDown` / `EnterFocus` / `ExitFocus` / `SetMode`), `DefaultKeyMap` |
| `keys.go` | `KeyEnter` / `KeyEsc` / `KeyCtrlC` / ... — canonical key-string constants |
| `clipboard.go` | `Clipboard` type, `NoClipboard`, `OSC52Clipboard` (terminal-escape-based) |
| `messages.go` | `ClearCopyMsg`, `CellAdvanceMsg`, `PromptSubmittedMsg`, `ReleaseFocusMsg`, `setModeMsg` + helpers |
| `cell.go` | `Cell` interface, `Mode` interface, `NewMode`, `SelectMode` / `ViewMode` |
| `input.go` | `Input` interface + `StringInput` / `IntInput` / `ChoiceInput` |
| `output_buffer.go` | `OutputBuffer` (line-indexed, RWMutex-guarded, `io.Writer`) |
| `cells/` | `HeaderCell`, `NoteCell`, `VerbatimCell`, `OutputCell`, `PromptCell` + per-cell `*Style` types + `Theme` aggregator |

### Cell-first key dispatch

Every keystroke routes to the cursor cell first. `Cell.Update` returns `(Cell, tea.Cmd, bool)` — the third value is `handled`. If true, the notebook stops; if false, the notebook checks `KeyMap.Global` then `KeyMap.Modes[currentMode]` and dispatches the matching `Action`. The "rare case" of `handled=false + cmd!=nil` is honored: cell had a side effect AND wants notebook to also try its bindings.

The principle: **cells own how they react to keys when focused, including how they signal release.** The notebook claims no key unconditionally. `Ctrl+C` lives in `DefaultKeyMap.Global` — cells that don't intercept it (every built-in cell, by convention) passthrough and the global binding catches it. A cell that wants to consume `Ctrl+C` (a confirmation cell, an undo-stack cell, …) returns `handled=true` and the global never fires.

`Esc → ReleaseFocus` is a convention, not a notebook rule. Built-in cells handle Esc → `notebook.ReleaseFocus` cmd; the model exits ViewMode. Cells with internal sub-modes can consume multiple Escs before returning ReleaseFocus.

### Modes are app-defined

`Mode` is opaque (`NewMode("CUSTOM")`); `SelectMode` and `ViewMode` are shipped defaults but apps can use any number of modes. `KeyMap.Modes[mode]` holds per-mode bindings; `KeyMap.Global` applies in every mode. A mode with no `Modes` entry means "no notebook bindings here" — every key passes through to the cell (the natural setup for ViewMode).

### Focus presentation belongs to the cell

`RenderRows(width, startRow, endRow, focused bool, mode Mode)` — the `focused` argument is the only signal the notebook gives. Each cell decides what focused looks like (border color, internal cursor, status hint, etc.). PromptCell calls `syncFieldFocus(focused, mode)` at the top of RenderRows to drive its bubbles/textinput Focus state — without this the textinput cursor would blink even on cells that aren't focused.

### Concurrency model

- Mutations from any goroutine go through `store.*` methods (RWMutex-guarded). **Mutations never `Send`** — `tea.Program.Send` is unbuffered and blocks until `Run` drains it (BT v1.3.10 source confirmed). A 16ms repaint tick is the universal trigger that picks up store changes within one frame.
- Reads (`Get` / `Len` / `IndexOf`) lock-read; cheap.
- Model state (`mode`, `width`, `height`, `viewportOffset`) is BT-goroutine-only; no lock.
- Streaming writes go to `OutputBuffer` (its own mutex); the cell's render-time `clampScroll` makes follow-mode "tail -f" behavior surface within one tick without a separate notification hook.
- Rendezvous (`AwaitInput*`) registers a buffered-cap-1 chan; the model resolves it on `PromptSubmittedMsg`; `Stop` drains pending awaits with `Source: "cancelled"`.

### Mouse

`tea.WithMouseCellMotion` is enabled in non-headless mode. The model handles `tea.MouseMsg`:

- Wheel up/down → cursor up/down (same as `↑`/`↓`)
- Left click → cursor to the cell at the clicked Y (`cellAtRow` maps absolute Y → header offset → viewportOffset → cell-span lookup)
- Release events are ignored

Mouse is intentionally **NOT** routed cell-first: clicks are geometric (the Y identifies the target, which may not be the focused cell), so the notebook owns dispatch. Future per-cell mouse handling (textinput cursor positioning, clicking verbatim-cell variant tabs) can hook in at `cellAtRow` before the navigation fallback.

### Snapshot for headless tests

`Snapshot()` does a synchronous `Send(snapshotMsg{reply})`; the model recomputes `ensureCursorVisible` then writes `View()` to reply. Waits on a `ready` channel (closed by Init's first cmd) before issuing the Send to avoid racing program startup. The headless input is a custom `blockingReader` so BT's input goroutine parks instead of spinning.

### What's not (yet) wired

- **No demokit bridge.** The standalone notebook + the existing `tui/notebook/` (the legacy event-queue-based renderer used by demokit today) coexist. Phase 4 will replace `tui/notebook/` with a `notebookbridge/` package that drains `events.EventQueue` and calls notebook methods.
- **Single live consumer**: `notebook/examples/mathrepl/`. Test surface in `notebook/notebook_test.go`, `notebook/concurrent_test.go`, `notebook/keymap_test.go`, `notebook/cells/*_test.go`.

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
