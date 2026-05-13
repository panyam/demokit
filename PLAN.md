# PLAN: Verbatim variants + copy

## Goal

Two related additions to demokit's verbatim block:

1. **Variants** — a single labeled snippet ("Fetch a user") can carry N alternative
   forms (curl / gcloud CLI / Python). Renderers pick what to show; in TUI, the user
   tabs between them.
2. **Copy** — a clipboard primitive (`demokit.Copy(s)`) that combines OSC 52 with
   OS shell fallbacks (`pbcopy` / `wl-copy` / `xclip` / `xsel` / `clip.exe`). When a
   step's verbatim block is rendered "boxed" in TUI, the user can copy the focused
   variant via the `c` key.

Both are opt-in at the **demo level**, not per-block. Today's single-variant unboxed
verbatim — the existing surface — keeps working unchanged. A new demo flag
(`Demo.BoxedVerbatim()`) flips all verbatim into the boxed + interactive form in
TUI. Multi-variant blocks auto-box regardless of the flag (tabs need a frame).

## Surface

### Variant data model

```go
// Variant is one labeled form of a verbatim snippet.
type Variant struct {
    Label   string  // "curl", "gcloud", "Python" — empty on single-variant blocks
    Lang    string  // fenced-code hint
    Content string
    Default bool    // marks the preferred form for non-interactive / doc default
}

// Constructor + fluent default marker.
func Variant(label, lang, content string) Variant
func (v Variant) Default() Variant
```

### Multi-variant constructor on StepDef

```go
// VerbatimVariants attaches a multi-variant snippet to the step. The first
// variant is the implicit default if no .Default() marker is set.
func (s *StepDef) VerbatimVariants(label string, variants ...Variant) *StepDef
```

Existing single-variant setters (`Verbatim`, `VerbatimLang`, `Shell`) keep their
current signatures and return type (`*StepDef`) — no API change, no new public
type, chained step setters continue to work identically.

Usage:

```go
demo.Step("Fetch the user").
    Note("Same API, three idioms.").
    VerbatimVariants("Fetch user 123",
        demokit.Variant("curl",   "bash",   `curl -X GET https://api/users/123`).Default(),
        demokit.Variant("gcloud", "bash",   `gcloud users describe 123`),
        demokit.Variant("Python", "python", `import requests; r = requests.get(...)`),
    ).
    Run(...)
```

### Demo-level boxing — `Demo.BoxedVerbatim()`

```go
// BoxedVerbatim enables the boxed + interactive rendering of verbatim
// blocks in TUI mode. When unset (default), single-variant blocks render
// outside the TUI box (mouse-select-friendly) as today. Multi-variant
// blocks always render boxed regardless of this flag — tabs require a
// frame.
//
// Plain renderer, markdown, HTML, and JSON output are unaffected by
// this flag (no "box" concept outside the TUI).
func (d *Demo) BoxedVerbatim() *Demo
```

Effective per-block boxing at render time:

| Block type | `BoxedVerbatim()` unset | `BoxedVerbatim()` set |
|---|---|---|
| Single variant | unboxed (today's behavior) | boxed + `c`-to-copy |
| Multi variant  | boxed + tabs + `c`-to-copy | boxed + tabs + `c`-to-copy |

The rationale for auto-boxing multi-variant regardless of the flag: calling
`VerbatimVariants(...)` is itself an opt-in to interactivity (tabs require a
frame). Forcing authors to also set `BoxedVerbatim()` would be a second knob for
no extra signal.

### `verbatimBlock` updated

```go
type verbatimBlock struct {
    Label    string
    Variants []Variant
    // No explicit `Boxed` field — derived at render time from
    // (demo.BoxedVerbatim flag) OR (len(Variants) > 1).
}
```

Single-variant constructors fill `Variants` with a one-element slice keyed by the
existing Lang/Content. The legacy `Lang`/`Content` fields on `verbatimBlock` are
removed — `Variants[0]` carries them.

`VerbatimView` (the read-only projection) grows a `Variants []VariantView` field;
existing `Lang`/`Content` accessors are sugar over `Variants[0]` for callers that
ignore variants.

### Demo flag — `--variant`

Added to `RegisterFlags`, `scanOwnArgs`, and `FilterArgs`'s value-flag strip set.

| `--variant` value | Effect on plain / `--non-interactive` / `--doc md|html|json` |
|---|---|
| _omitted_ | If a variant is `.Default()`-marked, render only that one. Else render all. |
| `--variant=all` | Render all variants. |
| `--variant=default` | Render the default-marked variant; error if none marked. |
| `--variant=<label>` | Render the named variant; error (stderr, exit) if no block has it. |

**TUI ignores `--variant`** — the user picks interactively. Documenting this in
the flag help text avoids surprise.

## TUI interaction

A verbatim block is "boxed" if `Demo.BoxedVerbatim()` is set OR the block has
multiple variants (auto-box).

### v1: line-based copy

The interactive key dispatch (Tab cycling, single-keystroke `c`, numeric jump,
`[`/`]` block focus) requires raw terminal mode + cursor-positioned redraws.
Those land in a follow-up PR. v1 ships a line-based copy UX that delivers the
core value (variants visible + keyboard copy via OSC 52 / pbcopy) without
adding a raw-mode dependency:

- **Single-variant boxed** (e.g. `BoxedVerbatim()` set on a step with one
  snippet): block renders in a styled box. Pause prompt:
  ```
  Press Enter to continue · type `c` to copy
  ```
  Reading: `bufio.NewReader(os.Stdin).ReadString('\n')`. On `c`, the clipboard
  primitive copies the block's only variant; status line shows the strategy
  (`(copied via osc52)`); prompt re-displays. On empty Enter, continue.

- **Multi-variant** (always boxed): block renders inside a single box with
  every variant printed stacked, each prefixed by its `**label**`. Pause
  prompt:
  ```
  Press Enter to continue · type `c` to copy default · `c <label>` for a named variant
  ```
  `c` alone copies the default-marked variant (or the first if none marked).
  `c curl` copies the variant labeled `curl`; case-insensitive match. Unknown
  label re-prompts with an error line.

- **Unboxed verbatim** (single-variant, `BoxedVerbatim()` unset): unchanged
  from today — rendered outside the box, mouse-select copies. No `c` key in
  the prompt.

### v1.1 (follow-up): raw-mode interactivity

Will replace the line-based UX with the bindings already designed:

| Key | Scope | Effect |
|---|---|---|
| `Tab` / `Shift+Tab` | block (focused) | cycle variants forward / back |
| `1`–`9` | block (focused) | jump to variant N of focused block |
| `[` / `]` | step | cycle which block is focused |
| `c` | block (focused) | copy focused variant |
| `↑` / `↓` | **reserved** | future history mode |
| `Enter` | step | continue (unchanged) |

Reserved tab strip render (`[1] curl  <2> gcloud  [3] Python`) lands then.

### Block ID scheme

Independent of v1 vs v1.1: `<step-id>:verbatim:<index-within-step>`. Reserved
in v1 for future focus addressing.

## Clipboard primitive — new `clipboard.go`

```go
// Copy writes s to the system clipboard. Strategy order:
//   1. OSC 52 escape sequence (terminal-side; works over SSH if the
//      terminal allows it; tmux 3.3+ honors it with set-clipboard on).
//   2. pbcopy   (darwin)
//   3. wl-copy  (Wayland)
//   4. xclip    (X11)
//   5. xsel     (X11 fallback)
//   6. clip.exe (Windows / WSL)
//
// Returns the strategy that worked and ok=true, or ("", false) if all
// failed. Missing tools are silent skips, not errors.
func Copy(s string) (strategy string, ok bool)
```

OSC 52 implementation writes `\x1b]52;c;<base64(s)>\x07` to a writer chosen at
package init (`os.Stderr` default — stdout is captured during step Run). We expose
a small `clipboardWriter` seam for tests to assert the sequence without actually
shelling out.

Shell fallbacks use `exec.LookPath` first; only execute if the binary exists.
Each invocation pipes `s` to stdin with a short context timeout (2s) so a hung
clipboard daemon never blocks the TUI.

No third-party dependency.

## Markdown rendering

Sub-headings per variant (your preference):

```markdown
#### Fetch user 123

**curl**

​```bash
curl -X GET https://api/users/123
​```

**gcloud**

​```bash
gcloud users describe 123
​```

**Python**

​```python
import requests; r = requests.get(...)
​```
```

Same shape in both `markdown.go` (the rich static visitor) and `render_trace.go`
(per-entry trace renderer). `writeVerbatimMD` is updated to emit either:

- One variant — current single block format (no `**label**` line); or
- N variants — `**label**` line above each fenced block, in declaration order.

`--variant=<label>` filters to that one in markdown output; default behavior is
"default-marked if set, else all."

## HTML rendering

The minimal standalone HTML (`render_trace.go::RenderDocumentHTML`) uses
`<h4>` for the outer label and bolded `<strong>` lines for each variant label
plus `<pre><code class="language-X">` per snippet. No JS, no tabs — same model
as the markdown render.

**Web player tabbed UI (the `<demokit-demo>` custom element + bundle) is
deferred to a follow-up PR.** The JSON projection lands now so embed hosts have
the data; the player's vanilla-JS UI for tabs is its own work.

## JSON projection

```go
type VariantView struct {
    Label   string `json:"label,omitempty"`
    Lang    string `json:"lang,omitempty"`
    Content string `json:"content"`
    Default bool   `json:"default,omitempty"`
}

type VerbatimView struct {
    Label    string        `json:"label,omitempty"`
    Variants []VariantView `json:"variants"`
}
```

A single-variant block emits a one-element `variants` array — embed hosts read
the same shape unconditionally. No `lang`/`content` on the outer view; everything
lives in variants for symmetry. No `Boxed` field — boxing is a TUI rendering
concern, derived at render time from the demo flag + variant count, not a per-
block property worth wire-projecting.

## Block identity for future history nav

Each rendered verbatim block gets an addressable ID:

```
<step-id>:verbatim:<index-within-step>
```

E.g. `triage:verbatim:0`, `triage:verbatim:1`. The ID is computed in the renderer
and stored on the in-memory block state. v1 doesn't read these IDs (no history
nav yet), but reserving the scheme avoids retrofitting when history mode lands.

## What stays out of the trace

Variants are static (author-time). The trace does NOT record which variant the
user copied (or "viewed") — variants don't affect demo flow. `TraceEntry` is
unchanged. Replays don't need to know about variants.

## File-level changes

| File | Change |
|---|---|
| `step.go` | `Variant` type + `.Default()`; `verbatimBlock` rewritten with `Variants []Variant` (no Boxed field); `VerbatimVariants` constructor returning `*StepDef`; existing single-variant constructors unchanged in signature; `VerbatimView` updated. |
| `clipboard.go` (new) | `Copy(s string) (strategy string, ok bool)`; OSC 52 emitter; shell-fallback dispatch. |
| `clipboard_test.go` (new) | Asserts OSC 52 byte sequence via a fake writer; asserts LookPath-miss falls through; asserts no exec when all tools missing. |
| `args.go` | Add `--variant` to FilterArgs value-flag strip set. |
| `demokit.go` | `flagVariant string` + `boxedVerbatim bool` fields; `Demo.BoxedVerbatim() *Demo` setter; `RegisterFlags` / `scanOwnArgs` updates for `--variant`; expose `Demo.VariantSelection() VariantSelection` helper used by renderers to pick variants for non-interactive output; expose `Demo.IsBoxedVerbatim() bool` for TUI to consult at render time. |
| `markdown.go` | `writeVerbatimMD` rewritten to emit one-or-many variants with sub-headings; applies `VariantSelection` filter. |
| `render_trace.go` | `writeVerbatimMD/HTML` reuse; same selection filter; HTML output uses `<strong>` per variant. |
| `render_json.go` | `VariantView`; `VerbatimView` reshaped; `inputView` untouched. |
| `tui/` | Render single-variant inside the box when `Demo.IsBoxedVerbatim()` is true. Multi-variant always boxed with stacked-with-labels rendering. After step render, the TUI's pause loop reads a line: empty = continue, `c` = copy default/single variant, `c <label>` = copy named variant. `clipboard.Copy()` integration, status-line "(copied via X)" feedback. **Raw-mode key dispatch (Tab, single-key c, 1-9, [/], focus model) deferred to follow-up PR.** |
| `renderer.go` | `PlainRenderer.RenderStep` applies the `--variant` selection (no interactivity). |
| `examples/graph/` | Demo calls `.BoxedVerbatim()`; the "Refresh succeeds" step's `.Shell(...)` is converted to `.VerbatimVariants("Refresh the token", curl.Default(), python, go)` — three client forms; curl is the default. The "Retry succeeds" step keeps its single-variant `.VerbatimLang(...)` so it exercises the demo-flag-driven boxing path. |
| `examples/dungeon/` | Spot-check that sidecar-md path still loads (no markdown variant syntax in v1). |

## Sidecar markdown

Out of scope for v1. Sidecar-loaded verbatim blocks stay single-variant; the
goldmark fenced-code parser doesn't get a `variants` block type in this PR.
Authors who need variants in a sidecar demo declare them via `Demo.Bind(id)` and
call `VerbatimVariants(...)` from Go (mirrors how `Run` / `Coalesce` are
Go-only). Follow-up can add a `variants` reserved fence info-string.

## Tests

- `TestVariantConstruction` — `Variant("curl", "bash", "...").Default()` sets Default; `Variant(...)` without it leaves Default false.
- `TestSingleVariantBackcompat` — `Verbatim("l", "c")`, `VerbatimLang("l", "py", "c")`, `Shell("c")` each produce one-element `Variants` slices preserving label / lang / content; rendered markdown / JSON / HTML byte-equal pre-change reference output. Existing chain shape (`step.Verbatim(...).Shell(...).Run(...)`) compiles unchanged.
- `TestBoxedVerbatimFlag` — `Demo.BoxedVerbatim()` toggles the flag; `Demo.IsBoxedVerbatim()` reads it; default is false.
- `TestMultiVariantAutoBoxed` — TUI render of a step with a multi-variant block produces the boxed/tabbed layout even when `BoxedVerbatim()` is unset.
- `TestSingleVariantUnboxedByDefault` — TUI render of a step with a single-variant block produces today's unboxed output when `BoxedVerbatim()` is unset; the same step produces boxed output when `BoxedVerbatim()` is set.
- `TestRenderMarkdownVariants` — multi-variant block emits `#### <label>` + `**variant-label**` + fenced block per variant; single-variant block emits the legacy shape unchanged.
- `TestRenderJSONVariants` — multi-variant block produces a `variants` array; single-variant block produces a one-element array.
- `TestRenderHTMLVariants` — same shape via `<strong>` labels.
- `TestVariantFlagDefault` — `--variant` unset + one variant marked Default → render only Default. Unset + no Default → render all.
- `TestVariantFlagExplicit` — `--variant=all` renders all; `--variant=curl` renders only curl; `--variant=does-not-exist` exits with stderr error.
- `TestFilterArgsStripsVariant` — both `--variant foo` and `--variant=foo` are removed from caller-side args.
- `TestClipboardOSC52` — `Copy("hello")` with a fake writer emits `\x1b]52;c;aGVsbG8=\x07`.
- `TestClipboardShellFallback` — when `exec.LookPath("pbcopy")` is stubbed to fail, the next strategy is tried; when all fail, `Copy` returns `("", false)`.
- `TestClipboardTimeout` — a hanging stub binary is killed after the 2s context deadline; `Copy` returns `("", false)`.

TUI interaction (line-based `c` / `c <label>` copy command parsing) is
exercised by feeding lines into the renderer's pause loop through a stdin pipe
— same pattern as the existing `cancel_test.go` cancellable-stdin tests. The
raw-mode key dispatch tests land in the v1.1 follow-up PR.

## Build sequence

1. PLAN.md (this file) — review.
2. `step.go` — data model + `Variant` type + `VerbatimVariants` constructor; `verbatimBlock` rewritten around `Variants []Variant`; existing single-variant constructors keep signature/return type. Existing tests adjusted for internal shape change only.
3. `clipboard.go` + `clipboard_test.go` — standalone primitive, no other code depends on it yet.
4. `markdown.go`, `render_trace.go`, `render_json.go` — variant-aware renderers; `--variant` flag wired in.
5. `args.go` + `demokit.go` — `--variant` flag registration + strip set; `BoxedVerbatim()` / `IsBoxedVerbatim()`; `VariantSelection` helper.
6. `tui/` — interactive variant tabs, focus model, clipboard integration.
7. `examples/graph/` smoke; manual TUI exercise of Tab/`c`/`[`/`]`.
8. `ARCHITECTURE.md` + `README.md` short additions describing the surface.

## Open questions resolved

- **Box opt-in**: **demo-level only** via `Demo.BoxedVerbatim()`. Multi-variant blocks auto-box regardless of the flag (tabs need a frame); single-variant blocks honor the flag (default unboxed = today's behavior). No per-block `.Boxed()` API, no `VerbatimRef` type — keeps the chain shape and API surface unchanged.
- **`.Default()` placement**: marker on `Variant` (local; one source of truth per option).
- **Numeric quick-jump**: `1-9` acts on focused block's variants. `[` / `]` switches block focus. Preserves `↑` / `↓` for future history mode.
- **`--variant` scope**: applies to plain / non-interactive / `--doc`. TUI ignores it (interactive selection).
- **Block ID**: `<step-id>:verbatim:<index>` scheme reserved now; unused in v1.
- **Trace impact**: none. Variants are static.

## Out of scope (explicit)

- Sidecar-markdown `variants` fenced-block syntax.
- Web player (`<demokit-demo>` custom element) tabbed UI — JSON projection lands; player JS is a follow-up.
- Cross-step history navigation (`↑` / `↓` mode). Reserved bindings only.
- Per-block boxing override (e.g. "this demo is mostly boxed but THIS block should be mouse-select"). YAGNI until a real demo asks for it.
- Per-variant copy in non-TUI contexts (e.g. a "copy" button in the static HTML render). HTML stays static; web player gets the dynamic UX.
