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
| `harness/` | Subpackage `package harness`. `harness.Run(demo)` / `harness.SetupRenderer(demo)` — one-call renderer/mode/web wiring (the `--mode` switch + `web.RegisterWith`). Batteries-included: imports `tui` + `notebookbridge` + `web`, so it pulls their charm/websocket deps; lean consumers skip it and wire renderers directly. |

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
| **Plain list** (refs, verbatim blocks, future `tags`) | initial list | append |
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

~~~bash {verbatim="Reproduce on the wire" label=curl default=true}
curl -s https://api.example/...
~~~
~~~go {verbatim="Reproduce on the wire" label=go}
http.Get("https://api.example/...")
~~~
```

### Conventions

- **Heading anchor `{#id}` is the join key.** CommonMark extension; renders cleanly on GitHub. Without an explicit anchor, the title is slugified.
- **Step vs section is decided by content shape.** A heading with any of [blockquote note, mermaid arrows, refs, inputs, verbatim blocks] is a step; prose-only headings are sections. Bind only steps.
- **Three reserved fenced info-strings:** `mermaid`, `inputs`, `refs`. Other fenced blocks pass through as section body for future renderers.
- **Verbatim blocks are attribute-driven, not a reserved info-string.** Any fence with a `verbatim="<title>"` attribute becomes a verbatim block (`<title>` is the block label); `label="<name>"` names the variant and `default=true` marks the preferred one. Consecutive fences sharing a title merge into one multi-variant block (the curl/go/python tabs), mapping to `VerbatimVariants`; a lone titled fence maps to `VerbatimLang`. Attributes are parsed by goldmark's own `parser.ParseAttributes`, so `default=true` is required (a bare `default` makes goldmark reject the whole attribute group). Use `~~~` fences so a ```` ``` ```` block can appear inside the verbatim body.
- **Mermaid features beyond `->>` / `-->>`** (`participant`, `Note over`, `alt`, `loop`, `autonumber`) are silently dropped with a load warning. demokit's model is arrow-only.

### Examples

| Directory | Mode | Stresses |
|---|---|---|
| `examples/basic/` | inline | inline-only API; regression check that the Go-only path keeps working |
| `examples/graph/` | inline | branching state machine, `Coalesce`, `AutoAcceptAfter` countdown |
| `examples/dungeon/` | sidecar | `FromMarkdownBytes` + `go:embed`, `Bind`, multiple cycles, `MaxVisits` guard, `int` input, Go-side state (the magic ring), cancellable streaming step (`listen`), live ANSI dragon scene |

## CLI: scaffolding and extraction (`cmd/demokit`)

`cmd/demokit` is a `package main` tool (stdlib only — `flag`, `go/parser`, `text/template`, `embed`) installed via `go install github.com/panyam/demokit/cmd/demokit@latest`. Three subcommands:

| Command | Does |
|---|---|
| `demokit init [dir]` | writes a base `walkthrough.mk` and one sample example (`--kind`, default `live`) so `make demo` runs immediately |
| `demokit new <name> --kind=narrated\|live\|branching` | renders one example dir from an embedded starter (`templates/<kind>/*.tmpl`, substituting the titleized name) plus a per-example `Makefile` that `include`s `walkthrough.mk` |
| `demokit extract <file.go> [--out dir]` | converts a Go walkthrough to sidecar form: `demo.md` (content) + `bindings.go` (behavior skeleton) |

**Starters are a per-example gradient, not project modes:** `narrated` (sidecar only) ⊂ `live` (sidecar + `Bind`) ⊂ `branching` (Go routing/state). A project mixes them freely; `--kind` is a one-time generator choice, and the scaffold is dep-light (generated `main.go` imports only `demokit` + `harness`). The genuinely-common renderer wiring lives in `harness`, not in generated project code.

**`extract` is a `go/ast` transform**, deliberately a first cut. It handles the linear builder pattern — `demokit.New(...)`/`Description`/`Actors` → frontmatter; `Step`/`Section` chains with `ID`/`Note`/`Arrow`/`DashedArrow`/`Verbatim*`/`Shell` → markdown; `Run`/`Coalesce`/`Parse`/`Timeout`/`Cancellable` carried verbatim into the `Bind` skeleton by source-slicing. It **guarantees unique `{#id}`s** (explicit `ID`, else slugified title, else `-2`/`-3` dedup). What it can't statically resolve — non-literal content, `Input(...)` declarations, project-specific content helpers like a `WireRecipe` wrapper — becomes a `TODO(extract)` marker with a stderr warning, never a silent drop. The emitted `demo.md` is verified by loading it back through demokit's own loader in tests.

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

The `harness/` subpackage bundles this opt-in: `harness.SetupRenderer(demo)` (and `harness.Run`, which adds `Execute`) selects the renderer for `demokit.Mode()` and calls `web.RegisterWith` for you, so a walkthrough's `main` is one line. Because harness owns the `web.RegisterWith` call, a demo using harness must **not** also call it — the `bundle` format would register twice and panic. Demos that need bespoke renderer wiring (e.g. `examples/basic`'s `--smooth` plain renderer, `examples/verbatims`'s border-style flag) skip harness and wire renderers directly; `examples/{graph,dungeon}` use `harness.Run`.

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
| `notebook.go` | `Notebook` struct, `New`, `Run`, `Stop`, options (`WithKeyMap`, `WithMouseConfig`, `WithClipboard`, `WithPromptFactory`, `WithHeadless`, `WithSize`), `Snapshot`, dock CRUD (`SetDockedCell` / `ClearDocked` / `DockedCell` / `UpdateDocked`), `FocusDock` / `ReleaseDockFocus` |
| `mouse.go` | `MouseConfig`, `MouseContext`, `MouseHandler`, `ClickActivate` / `ClickCursorOnly` / `WheelNavCursor` / `DefaultOnClick`, `DefaultMouseConfig` |
| `model.go` | `tea.Model` — viewport, mode, key & mouse dispatch, render, dock-aware layout (`edgeAllotments`, `bodyHeight`, `anchoredRowSpan`) |
| `store.go` | `store` — shared RWMutex-guarded cells + cursor + header + docked-cell registry |
| `crud.go` | `Append` / `Insert` / `Update` / `Remove` / `Get` / `IndexOf` / `Len` / `SetCursor` / `FocusCell` / `SetHeader` / `SetDone` |
| `dock.go` | `Position` interface, `Top` / `Bottom` edges, `After(id)` / `Before(id)` cell-anchored positions, internal `positionKey` |
| `status_cell.go` | Built-in `StatusCell` — the default Bottom dock, reproduces the legacy "MODE  cell N/M" line |
| `rendezvous.go` | `AwaitInput` / `AwaitInputBy` (blocking primitives) + `InputResponse` |
| `stream.go` | `Stream(id) io.Writer` over a cell's `OutputBuffer` |
| `keymap.go` | `KeyMap`, `Action`, built-in actions (`Quit` / `NavUp` / `NavDown` / `EnterFocus` / `ExitFocus` / `SetMode` / `FocusDock` / `ToggleBottomDockFocus`), `DefaultKeyMap` |
| `keys.go` | `KeyEnter` / `KeyEsc` / `KeyCtrlC` / ... — canonical key-string constants |
| `clipboard.go` | `Clipboard` type, `NoClipboard`, `OSC52Clipboard` (terminal-escape-based), `FileClipboard(dir)` (tmp-file fallback) |
| `messages.go` | `ClearCopyMsg`, `CellAdvanceMsg`, `PromptSubmittedMsg`, `ReleaseFocusMsg`, `setModeMsg` + helpers |
| `cell.go` | `Cell` interface, `Mode` interface, `NewMode`, `NavigationMode` / `CellActiveMode` |
| `input.go` | `Input` interface + `StringInput` / `IntInput` / `ChoiceInput` |
| `output_buffer.go` | `OutputBuffer` (line-indexed, RWMutex-guarded, `io.Writer`) |
| `cells/` | `HeaderCell`, `NoteCell`, `VerbatimCell`, `OutputCell`, `PromptCell`, `CommandCell` (+ `OpenCommandBar` helper) + per-cell `*Style` types + `Theme` aggregator |

### Cell-first key dispatch

Every keystroke routes to the cursor cell first. `Cell.Update` returns `(Cell, tea.Cmd, bool)` — the third value is `handled`. If true, the notebook stops; if false, the notebook checks `KeyMap.Global` then `KeyMap.Modes[currentMode]` and dispatches the matching `Action`. The "rare case" of `handled=false + cmd!=nil` is honored: cell had a side effect AND wants notebook to also try its bindings.

The principle: **cells own how they react to keys when focused, including how they signal release.** The notebook claims no key unconditionally. `Ctrl+C` lives in `DefaultKeyMap.Global` — cells that don't intercept it (every built-in cell, by convention) passthrough and the global binding catches it. A cell that wants to consume `Ctrl+C` (a confirmation cell, an undo-stack cell, …) returns `handled=true` and the global never fires.

`Esc → ReleaseFocus` is a convention, not a notebook rule. Built-in cells handle Esc → `notebook.ReleaseFocus` cmd; the model exits CellActiveMode back to NavigationMode. Cells with internal sub-modes can consume multiple Escs before returning ReleaseFocus.

### Two built-in modes; apps add more

The framework promotes two modes as canonical:

- **`NavigationMode`** — cell-to-cell cursor nav. Notebook owns nav keys (j/k/↑/↓), clicks land on cells geometrically.
- **`CellActiveMode`** — cursor cell is "activated" and owns nearly every keystroke (text input, scroll, …).

Apps extend via `NewMode("CUSTOM")` and register per-mode bindings in `KeyMap.Modes[mode]`; `KeyMap.Global` applies in every mode. A mode with no `Modes` entry means "no notebook bindings here" — every key passes through to the cell (the natural setup for CellActiveMode).

### Mouse customization via `MouseConfig`

Mouse routing follows the same shape as keys: cells see wheel events first (cell-first dispatch), and the notebook falls back to `MouseConfig` handlers when cells don't claim. Clicks are geometric — they go straight to `MouseConfig.OnClick`. `DefaultMouseConfig` wires left-click → `ClickActivate` (sets cursor + CellActiveMode) and wheel-fallback → `WheelNavCursor` (cell-to-cell nav). Apps swap handlers via `WithMouseConfig` — see `notebook/mouse.go`.

### Focus presentation belongs to the cell

`RenderRows(width, startRow, endRow, focused bool, mode Mode)` — the `focused` argument is the only signal the notebook gives. Each cell decides what focused looks like (border color, internal cursor, status hint, etc.). PromptCell calls `syncFieldFocus(focused, mode)` at the top of RenderRows to drive its bubbles/textinput Focus state — without this the textinput cursor would blink even on cells that aren't focused.

### Concurrency model

- Mutations from any goroutine go through `store.*` methods (RWMutex-guarded). **Mutations never `Send`** — `tea.Program.Send` is unbuffered and blocks until `Run` drains it (BT v1.3.10 source confirmed). A 16ms repaint tick is the universal trigger that picks up store changes within one frame.
- Reads (`Get` / `Len` / `IndexOf`) lock-read; cheap.
- Model state (`mode`, `width`, `height`, `viewportOffset`) is BT-goroutine-only; no lock.
- **Send-from-BT-goroutine deadlocks.** The Send-using public methods — `Notebook.SetMode`, `Notebook.FocusCell` (calls SetMode), `Notebook.Stop` (calls `program.Quit`, internally Send-based) — are safe from a *non-BT* goroutine (e.g. a driver that just `Append`ed a prompt cell) but **deadlock the UI when called from inside a KeyMap action handler** (which runs on the BT goroutine itself: the Send blocks waiting for the same goroutine that has to drain it). From a KeyMap action, compose the equivalent `tea.Cmd` instead: `ModeCmd(m)` for mode, `tea.Quit` for shutdown; for focus, do `SetCursor(id)` (safe store mutation) and return `ModeCmd(CellActiveMode)`. The DockedCells helpers (`FocusDock` / `ReleaseDockFocus` / `ClearDocked`) follow this rule and are Send-free.
- Streaming writes go to `OutputBuffer` (its own mutex); the cell's render-time `clampScroll` makes follow-mode "tail -f" behavior surface within one tick without a separate notification hook.
- Rendezvous (`AwaitInput*`) registers a buffered-cap-1 chan; the model resolves it on `PromptSubmittedMsg`; `Stop` drains pending awaits with `Source: "cancelled"`.

### Mouse

`tea.WithMouseCellMotion` is enabled in non-headless mode. The model handles `tea.MouseMsg`:

- Wheel up/down → cursor up/down (same as `↑`/`↓`)
- Left click → cursor to the cell at the clicked Y (`cellAtRow` maps absolute Y → header offset → viewportOffset → cell-span lookup)
- Release events are ignored

Mouse is intentionally **NOT** routed cell-first: clicks are geometric (the Y identifies the target, which may not be the focused cell), so the notebook owns dispatch. Future per-cell mouse handling (textinput cursor positioning, clicking verbatim-cell variant tabs) can hook in at `cellAtRow` before the navigation fallback.

### Docked cells

The notebook ships positioned cell slots for chrome that lives outside the cursor-navigable main list: vim-style command bars, status rows, breadcrumbs, per-cell annotations. Same `Cell` interface, separate registry.

**Positions (v1):**

| Family | Examples | Layout |
|---|---|---|
| Edges | `Top`, `Bottom` | Viewport-pinned chrome above / below the body. Claim layout space; body shrinks by that many rows. |
| Cell-anchored | `After(id)`, `Before(id)` | Annotations that travel with a main cell. Render adjacent to the anchor in the body flow; scroll with it. Auto-unregister when `Remove(id)` fires on the anchor. |

`Left` / `Right` and a floating overlay layer are deferred to a v2 PR — column-split rendering and a Z-order engine are out of scope for the issue 36 ship.

**One Cell per Position.** `SetDockedCell` replaces any prior occupant at the same position. Apps composing richer chrome put multiple lines into a single Cell that knows how to render multiple regions.

**Architectural commitment:** docks live in their own registry, NOT in `cells[]`. The invariant `cells[N] is the N-th cursor-navigable cell` holds; main-list iteration is dock-free. The model walks dock keys explicitly in `View`, `cellRowSpan`, `cellAtRow`, and `edgeAllotments`.

**Default Bottom dock = `StatusCell`.** `New()` auto-installs a built-in `StatusCell` at `Bottom` that renders the legacy "MODE  cell N/M" line. Apps replace it with `SetDockedCell(Bottom, custom)` and can restore it via `SetDockedCell(Bottom, NewStatusCell(nb))`. `ClearDocked(Bottom)` truly empties — no auto-restore.

**Focus model.** Docked cells receive keystrokes when they're the focus target. Tracking is one `atomic.Pointer[positionKey]` on the notebook: nil = main list focused, set = that dock is focused. `FocusDock(pos)` / `ReleaseDockFocus()` are the goroutine-safe entry points; `ToggleBottomDockFocus` is the default `Ctrl+W` binding (vim/tmux-style window-switch shortcut). When a docked cell emits `ReleaseFocusMsg` (the standard Esc convention), the model clears the dock-focus pointer AND drops to `NavigationMode` — symmetric to how main cells release.

The `safeSend` helper guards mode-change Sends on dock APIs: when the BT program isn't running yet (tests, setup-before-Run paths), Sends are dropped instead of deadlocking on the unbuffered `tea.Program.Send`. Store mutations (the dock registry itself) remain Send-free and pick up via the repaint tick.

**Auto-grow + clamp + tail rendering.** Edge docks report a desired height via `HeightHint(width)` that can grow with content (a `CommandCell` wraps as you type). `edgeAllotments` enforces "body always has at least 1 row" by yielding rows from Bottom first, then Top. When a dock is clamped below its desired height, `View` renders the **tail** for `Bottom` (where the command cursor lives) and the **head** for `Top` (logo / breadcrumb reads left-to-right, top-first).

**Cell-anchored interleaving.** `Before(id)` and `After(id)` participate in body row math: `cellRowSpan` includes the `Before` height in the cursor's row span (so scrolling brings both into view) and skips `After` rows from the cursor span (they trail). Clicks on anchored-dock rows resolve to the anchor cell — they're not cursor-targetable, matching the "one cursor" principle.

**`OpenCommandBar` convenience (`cells` package).** Apps that want a vim-style `:command` bar wire it in one line: bind their trigger key to `cells.OpenCommandBar(nb, ":", onSubmit)`. The helper snapshots the current Bottom occupant, installs a `CommandCell`, focuses the dock, and on Enter / Esc restores the **same instance** that was there before — so a stateful status bar keeps its state across the show/hide cycle. The trigger key is entirely app policy; the framework ships no opinion about which key opens which dock.

### Snapshot for headless tests

`Snapshot()` does a synchronous `Send(snapshotMsg{reply})`; the model recomputes `ensureCursorVisible` then writes `View()` to reply. Waits on a `ready` channel (closed by Init's first cmd) before issuing the Send to avoid racing program startup. The headless input is a custom `blockingReader` so BT's input goroutine parks instead of spinning.

### `notebookbridge/` — demokit → notebook adapter

`notebookbridge/` is demokit's adapter onto the standalone notebook package. It implements `demokit.EventAwareRenderer`; demokit's `Execute` attaches the event queue to it, and a background goroutine drains the queue and translates each event into the equivalent `notebook.*` call:

| Event | Notebook call |
|---|---|
| `Header` | `nb.SetHeader(title, desc)` |
| `Section` | `nb.Append(cells.NewNote(...))` |
| `StepStart` | `nb.Append(cells.NewHeader(...))` + `nb.Append(cells.NewVerbatim(...))` for each verbatim block |
| `StepReadyToRun` | `nb.Append(cells.NewOutput(...))`; bridge tracks `visit → CellID` |
| `OutputChunk` | `io.WriteString(nb.Stream(visitID), chunk)` |
| `StepEnd` | `nb.Update(visitID, MarkDone)` + error line on `status == "error"` |
| `WaitForAdvance` | `nb.AwaitInput(nil)` → empty-input prompt; resolved on Enter |
| `PromptOpen` | `nb.AwaitInput(convertInputs(...))`; result mirrored back via `queue.Resolve` |
| `Done` | `nb.SetDone()` |

The bridge owns no rendering state — all UI state lives in the notebook. When the notebook program exits (user quits), the bridge `os.Exit(0)`s to avoid demokit's `RunLoop` continuing against a dead renderer.

`convertInputs` is the only place the bridge type-switches on the closed-set `events.Input` shapes (`StringInput` / `IntInput` / `ChoiceInput`) — the notebook only sees its own `notebook.Input` interface.

Examples wire it explicitly when `--mode=notebook` is selected:

```go
case "notebook":
    demo.WithRenderer(notebookbridge.New())
```

### Status

`notebook/` + `notebookbridge/` are the production path; the legacy `tui/notebook/` was deleted in phase 4. Two live consumers: `notebook/examples/mathrepl/` (standalone, no demokit) and `examples/{basic,dungeon,graph}` via the bridge.

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
