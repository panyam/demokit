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

The legacy flags `--readme`, `--readme-from`, `--readme-html-from` still work but print a deprecation warning; new code should use `--doc <format>`.

**Auto-advance with countdown.** `Demo.AutoAcceptAfter(5 * time.Second).ShowCountdown(true)` makes `WaitForStep` advance after a timer with a visible burn-down bar, while still letting Enter accept early. Useful for kiosks and timed demos.

**Two renderers.** `PlainRenderer` (zero deps) for plain stdout. `tui.Renderer` (via the `tui/` subpackage, Lipgloss-backed) for styled boxes. Both implement the same `Renderer` interface; you can swap your own in if you want HTML, JSON, or a TUI Bubble app.

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

## A note on the API surface

The builder pattern is deliberately verbose: `Step("name").ID("foo").Input(…).Coalesce(…).Run(…)`. There is no implicit magic — IDs default to `step-N` if you skip them, but anything you'll route to should have an explicit `ID`.

The `Run` function takes a `StepContext` and returns `*StepResult`. Returning `nil` is success-with-no-message and falls through to the next step. Returning `&StepResult{Next: "x"}` jumps. Returning `demokit.Errf("...")` is a typed error result that the renderer styles distinctly.

If you're coming from a linear-only `func() *StepResult` API: that signature was retired — `Run(func(ctx StepContext) *StepResult)` is the only signature now. Existing demos need a one-line update.
