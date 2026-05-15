# PLAN: TUI keystroke dispatch (v1.1)

> Short-term planning artifact for this branch only. Deleted at merge.
> Long-term references live in GitHub issues / PR descriptions.

## Goal

Replace the line-based pause dispatcher (`c<Enter>`, `c <label><Enter>`,
`<label><Enter>`) with a proper TUI keypress UI built on Bubble Tea. The
data model and clipboard primitive landed in PR 10 are unchanged — this
PR is purely the interactive surface.

Concretely, deliver these from the v1.1 list in PR 10's description:

- Raw-mode for the entire pause prompt (not just countdown)
- `Tab` / `Shift+Tab` to cycle variants of the focused block
- `1`–`9` to jump variant of the focused block
- `[` / `]` to cycle focused block (multi-block steps)
- Single-keystroke `c` (no Enter)
- In-place redraw — no scrollback accumulation
- Visual copy indicator (status flash ~1s)
- Countdown integration with the new key handling
- Reserved `↑` / `↓` — still no-op, reserved for cross-step history

Out of scope (still): sidecar markdown variant syntax, web player tabbed
UI, cross-step history navigation.

## Approach

Two parallel UIs over the same data model + clipboard primitive — both
in this PR for parity:

- **TUI (`tui.Renderer`)** uses **Bubble Tea** (`github.com/charmbracelet/bubbletea`)
  for a proper keypress-driven overlay only during the pause prompt.
  Tab cycles, single-keystroke `c`, in-place redraw, toast confirmation.
- **Plain (`PlainRenderer`)** adds a line-mode copy dispatcher to its
  pause prompt. Keystrokes commit on Enter; each reprint pushes the
  prompt down a line (no cursor positioning). Acceptable per user
  feedback — Plain explicitly doesn't need a proper TUI, just feature
  parity. Same commands (`c` / `c <label>` / `<label>` / `<n>`),
  same countdown integration (any input cancels via
  `WaitForLineOrTimeout`).

The rest of the renderer stays procedural — `RenderHeader`,
`RenderStep`, `RenderResult`, `RenderSection`, `RenderDone`,
`StreamOutput`, `Prompt` all continue with `fmt.Println`-style output
as today. **Only `WaitForStep` swaps out** to the new dispatchers.

This keeps:

- Streaming step output (`captureOutput` → `StreamOutput`) compatible —
  Bubble Tea owns the terminal only during the TUI pause overlay; Plain
  is always cooked-mode.
- `web.webRenderer` untouched — `--serve` mode doesn't pause for stdin.
- Renderer interface unchanged — `WaitForStep(WaitOpts)` keeps its
  signature; both implementations swap internally.

### Why not extend further (e.g. a full Bubble Tea TUI from header → done)?

A full-screen Bubble Tea program conflicts with demokit's streaming
output contract — a step's `Run` can `fmt.Println` while executing,
and the renderer streams via `StreamOutput`. Bubble Tea captures the
viewport; mixing streaming `Println` with a tea.Program is messy.
Per-pause overlay sidesteps that: tea owns the screen only while the
pause is active, and the program exits before the next `Run`.

## Pause-overlay model

### State

```go
// pauseModel is the Bubble Tea model for the line-up-to-pause-end
// overlay on a step. It owns:
//
//   - the rendered step (lastStep + active variant per block)
//   - the auto-accept deadline (or zero for no countdown)
//   - transient toast state (copy confirmation, error messages)
type pauseModel struct {
    step          *demokit.StepDef
    blocks        []demokit.VerbatimView // copyable subset
    activeBlock   int                    // index into blocks
    activeVariant []int                  // per-block active variant index
    deadline      time.Time              // zero = no countdown
    toast         string                 // transient status line ("(copied …)")
    toastUntil    time.Time              // when to clear the toast
    palette       Palette
    width         int
}
```

### View

```
[ Block label, bold + Title ]

<curl (default)>   python    go    ← tab strip (lipgloss)
╭──────────────────────────────╮
│ <active variant content>     │
╰──────────────────────────────╯

  Tab/Shift+Tab cycle · 1-3 jump · c copy · Enter run · 4.2s
       ↑ toast appears here for ~1s after copy
```

The renderer prints up to the box, then `tea.Program` takes over and
draws the tab strip + box + status line. On every `Update`, the View is
re-rendered (Bubble Tea handles the cursor positioning and clears).

### Updates

| Key | Effect |
|---|---|
| `Enter` | program returns; main loop continues to next step |
| `Tab` | active variant of focused block → next (wraps) |
| `Shift+Tab` | active variant of focused block → previous (wraps) |
| `1`–`9` | active variant of focused block → index (1-based) |
| `[` | focused block → previous (wraps) |
| `]` | focused block → next (wraps) |
| `c` | copy active variant of focused block; toast "(copied curl via osc52)" for ~1s |
| `↑` / `↓` | no-op; reserved |
| any printable char that doesn't match above | no-op (or visual nudge?) |
| countdown tick | redraws status line with remaining time; on expiry, returns (auto-advance) |

### Toast

A `tea.Cmd` schedules a `clearToastMsg` after ~1s; on receipt, clears
`toast` / `toastUntil` and triggers a re-render. Standard Bubble Tea
pattern.

## Single-block / single-variant degradation

- **No copyable blocks on step:** the overlay shows just a status line
  (`Press Enter to run...`) and reads for Enter. No tab strip, no box.
- **Single block, single variant + `BoxedVerbatim()` set:** the overlay
  shows the box without a tab strip; `c` copies; no `Tab` / `1` / `[`.
- **Single block, multi-variant:** tab strip + box; no `[`/`]`.
- **Multiple blocks, multi-variant:** full UI.

The overlay reads the step's blocks at start; the prompt hint adapts.

## Countdown integration

Bubble Tea's `tea.Tick` runs a 100ms ticker. The model's `Update`
checks `deadline` each tick:

- If `deadline.Sub(time.Now()) <= 0` → return `tea.Quit` (auto-advance)
- Else → re-render status line with remaining time

On any keypress during the countdown, the model **clears `deadline`**
(setting to zero) — the user signaled intent to interact, the countdown
is paid. The keypress itself dispatches normally (Tab cycles, c copies,
etc.). Enter still returns (advance). This matches PR 10's "any key
interrupts" semantic but without the line-mode awkwardness.

## Plain-mode dispatcher (simpler than TUI — all visible, pick to copy)

Plain mode shows every variant up front (no tab cycling needed — the
content is already on screen), so the only interaction is "which one do
you want copied?". No `c`, no `<label>`, no switching state — just a
single number maps to a single variant.

### Plain-mode rendering of verbatim blocks

**Plain mode currently renders nothing for verbatim blocks at all** —
that's a pre-existing gap (PR 10 added variant data + non-TUI markdown
rendering but didn't touch `PlainRenderer.RenderStep`). v1.1 fixes
that. The plain render numbers variants globally across all copyable
blocks on the step so the copy prompt can refer to them uniformly:

```
Step 4: Refresh succeeds
----------------------------------------------------------------------
  App ->> AS: POST /token (refresh)
  AS -->> App: {access_token, expires_in: 3600}

Refresh the token
  [1] curl (default)
      curl -s -X POST https://auth.example/oauth2/token \
        -H 'Content-Type: application/x-www-form-urlencoded' \
        -d 'grant_type=refresh_token&refresh_token=eyJhbGci...'
  [2] python
      import requests
      r = requests.post(...)
  [3] go
      resp, _ := http.PostForm(...)
```

For non-interactive runs (`--non-interactive`, `--replay`, `--doc`),
the `--variant` selection filters which variants render — same as
markdown today. `[N]` numbering still aligns with whatever's shown.

### Plain-mode pause prompt

```
[Enter to accept, [1-3] to copy]:
```

`[1-N]` adapts to the actual count of copyable variants on the step:

- N copyables → `[Enter to accept, [1-N] to copy]:`
- 1 copyable → `[Enter to accept, [1] to copy]:`
- 0 copyables → today's `Press Enter to run this step...`

Behavior:

- Read a line. Empty → return (advance). Single digit in range →
  copy that variant via `demokit.Copy(...)`, print `(copied python via
  osc52)`, reprint the prompt. Anything else → reprint the prompt (no
  error message; the prompt itself shows the valid form).
- Countdown: `WaitForLineOrTimeout` races a 100ms ticker against
  stdin. Any non-empty input cancels (clears the deadline); subsequent
  reads are pure line mode (no countdown).

No active-variant state to track. No switching. Just print the numbered
variants once, accept a digit to copy, advance on Enter.

## File-level changes

| File | Change |
|---|---|
| `go.mod` | Add `github.com/charmbracelet/bubbletea` |
| `tui/pause.go` (new) | `pauseModel` + Init/Update/View + key bindings + toast Cmd |
| `tui/tui.go` | `WaitForStep` becomes a thin wrapper that constructs `pauseModel` and runs a `tea.Program`; removes line-mode `copyPromptLoop`, `waitWithCountdown`, `echoActiveVariant`. The `lastStep` / `activeVariant` fields move to the model. |
| `tui/copy.go` | `handleCopyCommand` is replaced by the model's `Update` — file becomes much smaller (just the OSC52 invocation helper). |
| `tui/copy_test.go` | Rewritten against the model: send `tea.KeyMsg`s, assert state changes (active variant, toast text) and side effects (OSC52 byte write). |
| `renderer.go` | `PlainRenderer.RenderStep` grows verbatim rendering: numbered variants, one block per outer label. `PlainRenderer.WaitForStep` adopts the digit-to-copy dispatcher, integrating with `WaitForLineOrTimeout` for the countdown race. `WaitForKeyOrTimeout` removed (unused after the TUI moves to Bubble Tea). `WaitForLineOrTimeout` + `WaitForEnterOrTimeout` stay — Plain uses them. |
| `examples/graph/main.go` | No changes needed — same API. |

## Tests

### TUI (Bubble Tea pauseModel)

Bubble Tea models are unit-testable by sending `tea.Msg`s directly to
`Update` and checking returned state / `tea.Cmd`s. No raw TTY required.

- `TestPauseModelTabCycles` — `Tab` rotates `activeVariant`, wraps at end.
- `TestPauseModelShiftTabCycles` — `Shift+Tab` rotates backward.
- `TestPauseModelNumericJump` — `1`/`2`/`3` jump to index N-1; out-of-range no-ops.
- `TestPauseModelBlockFocus` — `[`/`]` rotates `activeBlock` for multi-block steps; no-op for single-block.
- `TestPauseModelCopy` — `c` writes OSC 52 of active variant content (byte-exact base64); toast set to "(copied … via osc52)".
- `TestPauseModelToastClears` — `clearToastMsg` clears the toast.
- `TestPauseModelEnterQuits` — `Enter` returns `tea.Quit`.
- `TestPauseModelCountdown` — `tea.Tick` decrements; expiry returns `tea.Quit`; any non-Enter key cancels the countdown without quitting.
- `TestPauseModelReservedArrows` — `↑` / `↓` are no-ops.

### Plain renderer

- `TestPlainRenderStepEmitsNumberedVariants` — multi-variant block renders with `[1]` / `[2]` / `[3]` prefixes; outer block label + variant labels appear; default-marked variant gets `(default)` tag.
- `TestPlainRenderStepRespectsVariantFlag` — `--variant=python` filters the numbered list to only the python variant (renumbered `[1]`); `--variant=all` renders all.
- `TestPlainWaitForStepCopiesByDigit` — fed `2\n\n` to the pause prompt, asserts OSC 52 contains the python variant's content (byte-exact base64) and pause returns on the trailing blank line.
- `TestPlainWaitForStepInvalidReprompts` — fed `xyz\n\n` (invalid then blank Enter), asserts no copy happens and pause returns after the blank.
- `TestPlainCountdownCancelOnInput` — fed `2\n\n` during a 5s countdown, asserts countdown stops on the first input, copy fires, then a blank Enter advances.

## Build sequence

1. PLAN.md (this file) — review.
2. `renderer.go` — `PlainRenderer.RenderStep` numbers + emits verbatim variants; respects `--variant` filter. Tests pass with the existing pause behavior (no copy interaction yet).
3. `renderer.go` — `PlainRenderer.WaitForStep` digit-to-copy prompt, countdown integration. Plain-mode interactive tests pass.
4. `go get bubbletea` + go.mod / go.sum.
5. `tui/pause.go` — new Bubble Tea model + Init/Update/View + tests.
6. `tui/tui.go` — replace `WaitForStep` internals with the `tea.Program`; remove line-mode `copyPromptLoop` / `waitWithCountdown` / `echoActiveVariant`.
7. `tui/copy.go` — trim to clipboard invocation helper; tests rewritten against the Bubble Tea model.
8. `renderer.go` — remove unused `WaitForKeyOrTimeout`.
9. Smoke: `make demo` (TUI) and `make run` (Plain) in `examples/graph/`. Verify Tab cycling + toast (TUI), digit-to-copy (Plain), countdown integration end-to-end in both modes.
10. Delete this `PLAN.md` in the merge commit.

## Risk / blast radius

- **Bubble Tea adoption:** new dep on a well-maintained, popular package; no incompatibilities expected with our existing lipgloss usage.
- **Terminal state:** Bubble Tea handles raw-mode setup + restore + signal handling internally. Smaller surface for "left the terminal in raw mode after panic" than hand-rolling.
- **Streaming compatibility:** Bubble Tea owns the terminal only during the pause overlay; `Run` execution + `StreamOutput` continue outside. Worth a manual test with a step that writes during execution to confirm no interaction.
- **Behavioral break:** the line-mode commands (`c<Enter>`, `c <label><Enter>`, `<label><Enter>`, `<n><Enter>`) go away entirely. Any user who scripted demokit by piping text into stdin during a TUI pause stops working — but TUI mode was always intended for human interaction, not scripting (scripts use `--non-interactive` + `--replay`). Acceptable.

## Cleanup at merge

- Delete `PLAN.md` (this file).
- Keep `considered-rejected.md` (rejected scenario-filtering design — enduring record).
