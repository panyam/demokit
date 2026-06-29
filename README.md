# demokit

A small Go library for building runnable, branching demos that you can step through, record, replay, and turn into documentation.

It started as a linear "press Enter, watch the next slide" tool. It's now a state machine: each step can decide what runs next, can prompt for input, and the whole run can be recorded to disk and replayed verbatim.

## Why

If you've ever written an example program that's part walkthrough, part script, part talk-track, you know the trade-off. Slides go stale. Scripts don't explain themselves. Live coding is a lottery. demokit is the middle ground: you write a real Go program, but the framework lets it pause, show what's happening, branch on user choice, and emit a markdown or HTML transcript afterwards.

Best fit for tutorial demos, protocol walkthroughs (auth flows, RFC examples), bug postmortems where the path matters, conference talks that need to survive a flaky network.

## A taste

```go
demo := demokit.New("Token Refresh").
    Description("What happens when an access token expires")

demo.Step("Pick a symptom").ID("triage").
    Input(demokit.Choice("expired", "ratelimit").
        Named("kind", "Symptom").WithDefault("expired")).
    Run(func(ctx demokit.StepContext) *demokit.StepResult {
        switch ctx.Inputs["kind"] {
        case "expired":   return &demokit.StepResult{Next: "expired"}
        case "ratelimit": return &demokit.StepResult{Next: "ratelimit"}
        }
        return nil
    })

demo.Step("Token expired").ID("expired").
    Run(func(ctx demokit.StepContext) *demokit.StepResult {
        fmt.Println("API returned 401 token_expired")
        return demokit.Errf("expired")
    })

demo.Step("Rate limited").ID("ratelimit").
    Run(func(ctx demokit.StepContext) *demokit.StepResult {
        fmt.Println("Got 429; backing off")
        return demokit.Warn("retry after delay")
    })

demo.Execute()
```

A working version is in [`examples/graph/`](examples/graph/). Run it:

```bash
go run ./examples/graph/                                  # interactive
go run ./examples/graph/ --tui                            # styled boxes
go run ./examples/graph/ --record /tmp/run.json           # save a trace
go run ./examples/graph/ --replay /tmp/run.json           # replay it
go run ./examples/graph/ --doc md                         # static markdown
go run ./examples/graph/ --doc md --from /tmp/run.json    # markdown of that path
go run ./examples/graph/ --doc html --from /tmp/run.json  # standalone HTML
go run ./examples/graph/ --doc json --from /tmp/run.json  # JSON for embed hosts
```

## What's in the box

**Steps and routing.** Each step has an ID. A step's `Run` returns a `*StepResult`; setting `Result.Next = "some-id"` jumps there next. Empty `Next` falls through to the next item declared. That's the whole control flow — there is no separate "branch" node, "decision" type, or DSL.

**Inputs.** A step can declare inputs declaratively (`Input(demokit.Int().Named("port", "Port").WithDefault(8080))`). The renderer prompts, parses, and re-prompts on bad input — preserving valid values across retries so the user only retypes the broken field. Parsed values land in `ctx.Inputs` (typed `any`); if you want a struct, attach a `Coalesce` to assemble one and read `ctx.Input.(MyConfig)`.

**Recording and replay.** `--record path.json` writes the path the user took, including inputs and step output. `--replay path.json` reruns the demo over that trace — same inputs, same path, same output. Steps that diverge (e.g., refactored to take a different branch) get their `Next` overridden so the replay is deterministic.

**Trace-driven docs.** `--doc md --from trace.json` renders the *actual visited path* as markdown — useful when one demo has many paths and you want to document each. `--doc html --from` does the same for HTML. `--doc json --from` produces structured data for embed hosts that want to render their own DOM. Without `--from`, `--doc md` falls back to a rich static walkthrough of the demo definition (mermaid diagram, notes summary, run-it commands).

**Auto-advance with countdown.** `Demo.AutoAcceptAfter(5 * time.Second).ShowCountdown(true)` makes `WaitForStep` advance after a timer with a visible burn-down bar, while still letting Enter accept early. Useful for kiosks and timed demos.

**Sidecar markdown (optional).** Demos with a lot of prose can move content into a companion `demo.md` file. See [`examples/dungeon/`](examples/dungeon/) for a CYOA showcase that exercises cycles, the `MaxVisits` guard, an `int` input, and Go-side state in 50 lines of markdown + 80 lines of Go. Frontmatter holds the title/description/actors; `## Heading {#id}` blocks become steps or sections; blockquotes become step notes; reserved fenced blocks (`mermaid`, `inputs`, `refs`) declare arrows, typed inputs, and references. Verbatim snippets (including the curl/go/python tabbed variants) are authored with a `verbatim="<title>"` attribute on a fenced code block, so the copy-paste content lives in markdown too. Behavior (Run, Coalesce, custom parsers) stays in Go and attaches via `Demo.Bind(id)`:

```go
demo := demokit.New("Auth Failure Triage").FromMarkdown("demo.md")

demo.Bind("triage").Run(func(ctx demokit.StepContext) *demokit.StepResult {
    // ctx.Inputs["symptom"] is the value the user picked, validated by
    // the choice parser declared in demo.md.
    return &demokit.StepResult{Next: ctx.Inputs["symptom"].(string)}
})
```

Sidecar is **optional**. Every feature works inline via `Step()`/`Note()`/`Run()` — sidecar is just a content layer for demos where prose dominates. See [ARCHITECTURE.md](ARCHITECTURE.md#sidecar-markdown) for the file format and override semantics.

**Embed in any HTML host.** demokit ships a `<demokit-demo>` web component (vanilla JS, no framework). Three ways to embed:

```html
<!-- 1. Self-contained HTML bundle (offline-safe, file:// works) -->
<iframe src="my-demo.html" width="800" height="600"></iframe>

<!-- 2. Inline custom element (host theme flows through via CSS variables) -->
<script src="demokit-player.js"></script>
<demokit-demo data-src="trace.json"></demokit-demo>

<!-- 3. Programmatic — Go-emitted fragment with inline JSON -->
<demokit-demo>{"demo": {...}, "trace": [...]}</demokit-demo>
```

Generate a self-contained bundle with `go run ./mydemo --doc bundle --from /tmp/run.json --out /tmp/demo.html`. Bundle support and `--serve` are opt-in — wire them on the demo before `Execute()`:

```go
import "github.com/panyam/demokit/web"

func main() {
    demo := demokit.New("...")
    // ... define steps ...
    web.RegisterWith(demo)  // enables --doc bundle and --serve
    demo.Execute()
}
```

Or skip the boilerplate with the `harness` package, which selects the renderer for `--mode` (plain/tui/notebook), calls `web.RegisterWith`, and runs the demo in one call:

```go
import "github.com/panyam/demokit/harness"

func main() {
    demo := demokit.New("...")
    // ... define steps ...
    harness.Run(demo)  // --mode wiring + --doc bundle / --serve + Execute
}
```

`harness` is batteries-included: it imports the `tui`, `notebookbridge`, and `web` subpackages (and their deps). Demos that want a leaner binary or custom renderer wiring skip it and wire renderers directly (see `examples/basic`). Don't call `web.RegisterWith` yourself when using harness — it registers the bundle format for you.

For programmatic embedding, the same package exposes `web.TraceFragment(d, entries) string` and `web.WriteBundle(d, entries, outPath) error`. See [ARCHITECTURE.md](ARCHITECTURE.md#embed-surface--demokit-demo-web-player) for the full data-source model (URL static, inline blob, programmatic, and the live URL mode driven by `--serve`).

**Live mode (`--serve`).** `go run ./mydemo --serve :8765` runs the demo as an HTTP+WebSocket server. Open `http://localhost:8765/` and the embed page connects via WS, renders structured events as they arrive, and submits input forms back. The server terminal mirrors the same demo in your chosen renderer's style (`PlainRenderer` by default, `tui.Renderer` if you also pass `--tui`). Browser-side reset clears the feed and replays from the top; clients posting `{kind:"reset"}` over WS trigger the same restart. Useful for live presentations and interactive walkthroughs in slide decks.

`--input-timeout 60s` (or `Demo.InputTimeout(60 * time.Second)`) bounds how long a prompt waits for input. After the deadline, declared defaults are filled and the demo continues; the live page shows a "(no input — continuing with defaults)" notice. Per-step overrides via `step.InputTimeout(d)` for steps that need a different deadline (e.g. a long-pause "press Enter to advance" step in a kiosk).

**Two renderers.** `PlainRenderer` (zero deps) for plain stdout. `tui.Renderer` (via the `tui/` subpackage, Lipgloss-backed) for styled boxes. Both implement the same `Renderer` interface; you can swap your own in if you want HTML, JSON, or a TUI Bubble app.

**Standalone notebook component.** `notebook/` is a separate Go module (`github.com/panyam/demokit/notebook`) with no demokit dependency — a reusable cell-based TUI toolkit for any Go program that wants a navigable list of typed cells (REPLs, wizards, log viewers). Built-in widgets live in `notebook/cells/` (header / note / verbatim / output / prompt); apps wire bindings via `KeyMap`, define their own modes, implement their own `Cell` types. A live example is `notebook/examples/mathrepl/` — a math REPL with braille plots that uses none of demokit. The demokit→notebook bridge is on the roadmap; see [ARCHITECTURE.md § Notebook](ARCHITECTURE.md#notebook--standalone-cell-based-tui-component).

**Pluggable form prompts.** `tui.Renderer.WithPrompter(myPrompter)` swaps the input collection step. The default is a sequential readline; richer impls (e.g. one backed by `huh.Select` for arrow-key choices) are a small interface away.

## Two safety guardrails

A graph (cycles allowed) needs termination. Two knobs:

- `Demo.MaxSteps(n)` — hard ceiling on total step visits per run. Default 200.
- `Demo.MaxVisits(n)` — per-step revisit cap. Useful when a step routes back to itself for retry.

Hit either and the demo bails with a clear error and renders Done.

## Installing

```bash
go get github.com/panyam/demokit
```

Go 1.22+. The TUI renderer also pulls in `charm.land/lipgloss/v2`.

## Scaffolding a project (`demokit` CLI)

```bash
go install github.com/panyam/demokit/cmd/demokit@latest

demokit init myproject          # base Makefile + a runnable sample example
demokit new login --kind=live   # add an example from a starter
```

`--kind` picks a starter on a gradient: **narrated** (sidecar markdown only, no Go), **live** (markdown content + Go behavior bound by step id), **branching** (Go-driven routing and state). They mix freely within a project — pick per example. Generated code imports only `demokit` and `harness`.

Already have a Go-defined walkthrough and want to move its content into markdown? `demokit extract mydemo.go --out .` emits a `demo.md` (content) and a `bindings.go` skeleton (your `Run` closures, keyed by step id), assigning unique heading ids and flagging anything it can't statically convert with `TODO(extract)`.

## A note on the API surface

The builder pattern is deliberately verbose: `Step("name").ID("foo").Input(…).Coalesce(…).Run(…)`. There is no implicit magic — IDs default to `step-N` if you skip them, but anything you'll route to should have an explicit `ID`.

The `Run` function takes a `StepContext` and returns `*StepResult`. Returning `nil` is success-with-no-message and falls through to the next step. Returning `&StepResult{Next: "x"}` jumps. Returning `demokit.Errf("...")` is a typed error result that the renderer styles distinctly.

If you're coming from a linear-only `func() *StepResult` API: that signature was retired — `Run(func(ctx StepContext) *StepResult)` is the only signature now. Existing demos need a one-line update.
