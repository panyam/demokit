# Considered & rejected: Scenario-filtering primitives

**Status:** Considered 2026-05, rejected in favor of consumer-side composition
(shared helper package + N small `main.go` binaries per variant). See the closing
section at the bottom of this file for the rationale.

The plan below is preserved as a record of what was designed and why, in case the
trade-off changes (e.g. a demokit consumer hits a case that composition cannot
express cleanly).

---

## Goal

Let a single `Demo` be sliced into named **scenarios** — overlapping subsets of steps that
can be run, recorded, and rendered independently — without breaking demos that don't opt in.

Driven by mcpkit's events demos (14 linear steps each, 5–7 minutes live). Same shape will
hit any other demokit consumer that grows past comfortable single-narrative size.

## Surface (additive, no breaking changes)

### 1. Tag steps and sections — `.Scenario(name, more ...string)`

```go
demo.Step("Subscribe to webhook").ID("subscribe").
    Scenario("webhook").
    Run(...)

demo.Step("Connect").ID("connect").
    Scenario("basics", "webhook", "validation").    // belongs to all three
    Run(...)

demo.Section("Webhook overview").Scenario("webhook")
```

Variadic so authors can group a step into multiple scenarios in one call.
Multiple `.Scenario()` calls on the same step are **additive** (set-union) — your design
intuition is right; the alternative (last-call-wins) makes it impossible to grow tags
across composition layers.

A step / section with **zero `.Scenario()` calls is universal** — runs and renders in
every scenario filter. This is what preserves existing-demo behavior for consumers that
never opt in.

### 2. Declare scenario metadata — `demo.Scenario(name)`

```go
demo.Scenario("webhook").
    Description("Subscribe and receive deliveries via HTTP").
    Next("validation", "Run make demo-validation to see spec validation in action.")
```

A `*ScenarioDef` chain. Properties:

- `Description(string)` — used in the index doc.
- `Next(toScenario, blurb string)` — pointer rendered after the last step of this
  scenario (in TUI, in the per-scenario Markdown, and in the index).

**Why on the scenario, not on the last step (push-back on item 3 of the proposal):**

The "last step of scenario X" is something demokit can compute from the tagged step list
in declaration order. If the pointer lives on a step, the author has to remember to put
it on the right one — which moves any time another scenario is interleaved. And if the
same step happens to be the last step of *two* scenarios (entirely possible with overlap),
a step-bound pointer can't say what the next-of-X is vs. next-of-Y.

A scenario-declared `Next("validation", ...)` is unambiguous: "when scenario X's render
ends, point readers at scenario Y." The renderer does the bookkeeping.

If a scenario name appears in `step.Scenario(...)` but is never declared via
`demo.Scenario(name)`, demokit auto-declares it with no description (mirrors how step
IDs auto-fill). This keeps the simplest case (`step.Scenario("foo")` only) one-liner.

### 3. Runtime filter — `--scenario=<name>`

Added to demokit's standard flag set in `RegisterFlags` and `scanOwnArgs` (and to
`FilterArgs`'s strip set).

| Flag value         | Effect                                                           |
|--------------------|------------------------------------------------------------------|
| _omitted_          | Run / render every item — current behavior.                      |
| `--scenario=all`   | Same as omitted. Explicit form for Make targets.                 |
| `--scenario=foo`   | Filter to items that match scenario `foo` (or are universal).    |
| `--scenario=index` | Reserved sentinel — see (4).                                     |

Unknown scenario name: stderr error, exit. (Same shape as unknown `--doc` format.)

`index` is a reserved name; `demo.Scenario("index")` panics at registration time, so we
never have to disambiguate the sentinel from a real scenario.

The filter applies uniformly:

- **Run mode:** the loop walks the filtered items list; jumps via `StepResult.Next` to
  IDs outside the filter are an error (logged + recorded, run aborts cleanly). This is
  the right loud failure — silent skip would mask broken demos.
- **`--record` / `--replay`:** trace records only the filtered visited path. Replay is
  agnostic of the filter — if the trace was recorded under a scenario, replaying without
  `--scenario=…` just walks that recorded path; replaying with a *different* `--scenario`
  is also fine (the trace drives, scenario filter doesn't reapply during replay).
- **`--doc md|html|json`:** the renderer walks the filtered items list. The static
  `Demo.Markdown()` path filters mermaid actors to those referenced by the filtered steps
  (your second design call — agreed, only-actors-used).
- **`--doc bundle` / `--serve`:** same filter in the items the bundle / live stream sees.

### 4. Index document — `--scenario=index`

`--doc=md --scenario=index` (and `html`/`json` analogues) produces a top-level index of
all declared scenarios:

- For each scenario in declaration order: name, description, step count, and a
  `→ run with: --scenario=<name>` hint.
- Optionally a one-line summary derived from the first step's note's first sentence
  when the scenario has no `Description`.

Filename / output path stays consumer-owned: demokit emits to stdout (or `--out` for
formats that support it — bundle), consumer Makefiles route to `WALKTHROUGH-index.md` or
wherever they want.

`--scenario=index` without `--doc` (i.e. just running the demo) prints the index to
stdout and exits — useful as `go run ./mydemo --scenario=index` for "what scenarios does
this demo support?". This costs us little and avoids surprising "why does my CLI hang?"
when someone forgets `--doc`.

## Decisions on the open questions

| Question                                          | Decision                                                                                                                |
|---------------------------------------------------|-------------------------------------------------------------------------------------------------------------------------|
| `.Scenario()` additive vs. exclusive on multi-call| **Additive** (set-union).                                                                                               |
| Filtered mermaid: full actors or only-used        | **Only-used.** Walk filtered steps, collect referenced `from`/`to` IDs, emit only those `participant` lines.            |
| `make demo` (no flag) behavior                    | **Consumer-owned.** demokit doesn't define this. README documents the canonical pattern — author chooses (a) sequential with banners, (b) print-index. |
| Makefile generated vs. consumer-owned             | **Consumer-owned.** demokit only ships the runtime flag.                                                                |

## File-level changes

| File                  | Change                                                                                       |
|-----------------------|----------------------------------------------------------------------------------------------|
| `step.go`             | `StepDef.scenarios []string`, `StepDef.Scenario(name, more...) *StepDef`, `StepDef.Scenarios() []string`. Same on `SectionDef`. |
| `demokit.go`          | `Demo.scenarios map[string]*ScenarioDef`, `Demo.scenarioOrder []string`, `Demo.Scenario(name) *ScenarioDef`, `Demo.flagScenario` (+ register/scan/Filter), `Demo.filteredItems(scenario) []item`, runtime filter wiring in `RunLoop`. `index` reserved-name panic. |
| `scenario.go` (new)   | `ScenarioDef` type and its setter chain (`Description`, `Next`). Helper `Demo.MatchesScenario(it item, name string) bool` (universal + tagged check). |
| `args.go`             | Add `--scenario` to `FilterArgs`'s built-in value-flag set.                                  |
| `markdown.go`         | `Demo.Markdown()` accepts the active scenario filter; filters items + actors; appends `Next` pointer if scenario has one. |
| `render_trace.go`     | `RenderDocumentMD/HTML` filter trace entries by scenario; `Next` pointer at end.             |
| `render_json.go`      | Trace + items projected post-filter; `demoView.Scenarios` field for embed hosts.             |
| `render_index.go` (new)| `RenderIndexMD/HTML/JSON(ctx)` for the index document.                                       |
| `examples/graph/`     | Add a couple of `.Scenario(...)` tags + one `demo.Scenario(...).Description().Next(...)` so the test consumer exercises the path end-to-end. |

## Tests (to land with the implementation)

- `TestScenarioTagAdditive` — multiple `.Scenario("a")`, `.Scenario("b","c")` calls on
  one step produces the union `{a,b,c}`.
- `TestUntaggedStepsAreUniversal` — a step with no `.Scenario(...)` calls runs / renders
  in every scenario filter.
- `TestScenarioFilterRunLoop` — `flagScenario="foo"` runs only foo-tagged + universal
  items; `--scenario=all` and "" both run everything.
- `TestUnknownScenarioNamePanicsOrErrors` — `--scenario=does-not-exist` exits with the
  expected stderr message; `demo.Scenario("index")` panics at registration.
- `TestUndeclaredScenarioAutoCreated` — tagging steps with `Scenario("foo")` without
  `demo.Scenario("foo")` works; the scenario shows up in the index with empty description.
- `TestStaticMarkdownActorFiltering` — only actors referenced by the filtered scenario's
  steps appear as `participant` lines.
- `TestNextScenarioRenderedOnce` — when `Next("validation", blurb)` is set on scenario
  "webhook", rendering scenario webhook ends with the blurb; rendering scenario foo
  doesn't include it.
- `TestRenderIndexMD` — index doc lists declared scenarios, descriptions, step counts,
  and stable order.
- `TestFilterArgsStripsScenario` — `--scenario foo`, `--scenario=foo`, and missing-value
  forms all behave like other value flags.
- `TestJumpAcrossScenarioBoundaryErrors` — a `StepResult{Next: "step-not-in-filter"}`
  produces a clean error and trace abort entry.

## Out of scope (for this PR)

- **Sidecar markdown integration.** Authors that load content via `FromMarkdown` get no
  scenario syntax in this PR. Once the Go-side primitives stabilize, frontmatter (or a
  per-heading `[scenarios: a,b]` annotation) is a small follow-up.
- **mcpkit consumer-side changes.** This branch only ships the framework primitives.
  The events-demo split happens in a separate worktree against mcpkit, exercising what
  this PR builds.
- **Per-scenario `MaxSteps` / `MaxVisits`.** The existing demo-level guardrails apply
  to the filtered run as-is. If real demos hit a need for per-scenario caps later, add
  it on `ScenarioDef`.

## Build sequence

1. PLAN.md (this file) — review.
2. `scenario.go` + `step.go` + `demokit.go` (data model + filtering helper, no rendering).
3. `args.go` + flag wiring + run-loop filter; first set of tests pass.
4. `markdown.go` (static md filter + actors + Next pointer); render tests pass.
5. `render_trace.go` (trace md/html filter + Next pointer); render tests pass.
6. `render_index.go` + index dispatch in `emitDoc`; index test passes.
7. `render_json.go` projection update; JSON test passes.
8. `examples/graph` tagging update + manual smoke (run, record/replay, all four
   `--doc` formats).
9. `ARCHITECTURE.md` and `README.md` short additions describing the surface.

---

## Why rejected (2026-05)

The same problem — "this big demo has N narrative threads, run one at a time" — is
solved cleanly by **consumer-side composition** without any demokit change:

```
examples/events/discord/
  common/steps.go      # AddConnect(d), AddEventsList(d), AddPush(d), ...
  basics/main.go       # composes the basics subset
  webhook/main.go      # composes the webhook subset
  full/main.go         # composes all helpers, in canonical order
  README.md            # the "index" — plain prose
  Makefile             # demo-basics → go run ./basics, etc.
```

Trade-offs that drove the decision:

- **Wide blast radius.** Scenarios touch every renderer (`markdown.go`,
  `render_trace.go`, `render_json.go`, plus index renderer), the run loop, the
  flag parser, and `FilterArgs`. Cross-cutting filter axis vs. focused framework.
- **Composition already idiomatic in mcpkit.** The pattern (helper package +
  N `main.go`s) is how mcpkit organizes other examples — adding scenarios would
  be a fifth pattern, not a unification.
- **Overlap reality.** The driving demo (mcpkit events, 14 steps) has heavy
  overlap (`connect` is in 4 of 5 scenarios). The cleanest tagging API for that
  is "list step IDs in a scenario block" — but that's nearly the same source
  shape as "call helpers in a main.go." Composition wins on cost.
- **Per-variant flexibility.** Each binary can have its own description,
  actors, renderer, `--serve` port, etc. without framework support for "these
  fields are per-scenario."
- **Reversibility.** If a real demokit consumer later hits a case composition
  can't express, this plan can be picked up; no code commitment was made.

What the consumer ends up writing instead:

- Shared `common.NewDemo(title, description)` factory that pre-applies repeated
  knobs (MaxSteps, MaxVisits, AutoAcceptAfter, Actors).
- One `common.Add<StepName>(d)` per canonical step.
- A `common.NextDemoPointer(d, "webhook", blurb)` that's just
  `demo.Section("Next steps", "Run `make demo-webhook` to ...")`.

No demokit-side primitive needed.
