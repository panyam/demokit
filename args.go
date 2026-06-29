package demokit

import (
	"os"
	"slices"
	"strings"
)

// FilterArgs strips demokit's dispatcher flags from args so a caller's
// own flag.Parse only sees the flags it cares about. Useful in examples
// that layer their own flags (`-addr`, `--url`, `--file`, ...) on top
// of the demokit-recognized ones.
//
// Built-in strip set — the flags demokit's dispatcher consumes:
//
//   - --mode <plain|tui|notebook>  (value; selects the renderer)
//   - --tui                   (bare; alias for --mode=tui; deprecated)
//   - --note                  (bare; alias for --mode=notebook)
//   - --non-interactive       (bare; skips between-step pauses)
//   - --doc <format>          (value; routes to doc emission)
//   - --from <trace-path>     (value; trace input for --doc)
//   - --out <path>            (value; --doc bundle output)
//   - --variant <name>        (value; filters verbatim variant output)
//   - --record <path>         (value; trace record target)
//   - --replay <path>         (value; trace replay source)
//   - --serve <addr>          (value; live server address)
//   - --input-timeout <dur>   (value; input prompt deadline)
//
// This mirrors [Demo.RegisterFlags]; the two are kept in sync by a test.
// Both `--flag value` and `--flag=value` forms are stripped for the
// value flags. Anything else passes through untouched.
//
// To strip caller-declared flags too (e.g. an example's own --url),
// pass them as extras built with [BoolFlag] or [ValueFlag]:
//
//	flag.CommandLine.Parse(demokit.FilterArgs(os.Args[1:],
//	    demokit.ValueFlag("--url"),
//	))
func FilterArgs(args []string, extra ...ExtraFlag) []string {
	bare := map[string]bool{
		"--tui":             true,
		"--note":            true,
		"--non-interactive": true,
	}
	value := map[string]bool{
		"--doc":           true,
		"--from":          true,
		"--variant":       true,
		"--mode":          true,
		"--record":        true,
		"--replay":        true,
		"--out":           true,
		"--serve":         true,
		"--input-timeout": true,
	}
	for _, e := range extra {
		if e.TakesValue {
			value[e.Name] = true
		} else {
			bare[e.Name] = true
		}
	}

	out := make([]string, 0, len(args))
	skip := false
	for _, a := range args {
		if skip {
			skip = false
			continue
		}
		if bare[a] {
			continue
		}
		if value[a] {
			skip = true
			continue
		}
		if eq := strings.IndexByte(a, '='); eq > 0 && value[a[:eq]] {
			continue
		}
		out = append(out, a)
	}
	return out
}

// ExtraFlag declares a caller-side flag that [FilterArgs] should strip
// alongside demokit's built-in set. Construct with [BoolFlag] (bare) or
// [ValueFlag] (consumes the next arg, or `--flag=value`).
type ExtraFlag struct {
	Name       string
	TakesValue bool
}

// BoolFlag declares a bare extra flag (no value) for [FilterArgs] to
// strip. Pass the flag's leading dashes (e.g. "--serve").
func BoolFlag(name string) ExtraFlag {
	return ExtraFlag{Name: name, TakesValue: false}
}

// ValueFlag declares an extra flag that consumes a value for
// [FilterArgs] to strip. Both `--flag value` and `--flag=value` forms
// are handled. Pass the flag's leading dashes (e.g. "--url").
func ValueFlag(name string) ExtraFlag {
	return ExtraFlag{Name: name, TakesValue: true}
}

// IsTUI reports whether the user asked for the legacy TUI renderer —
// either by the bare --tui flag (deprecated) or by --mode=tui /
// --mode tui. Examples use this to gate the TUI renderer
// (`demo.WithRenderer(tui.New())`) without having to scan os.Args
// inline.
//
// Prefer [Mode] for new code: it returns the explicit mode string
// and handles "plain" / "notebook" too. IsTUI is kept as a
// convenience for examples that pre-date --mode.
func IsTUI() bool {
	if slices.Contains(os.Args[1:], "--tui") {
		return true
	}
	return Mode() == "tui"
}

// IsNotebook reports whether the user asked for the notebook
// renderer — either by the bare --note flag (shorthand) or by
// --mode=notebook / --mode notebook. Mirrors [IsTUI].
//
// Examples use this to gate `demo.WithRenderer(notebook.NewRenderer())`
// without scanning os.Args inline; [Mode] is the underlying source
// of truth.
func IsNotebook() bool {
	if slices.Contains(os.Args[1:], "--note") {
		return true
	}
	return Mode() == "notebook"
}

// Mode returns the renderer mode the user asked for via --mode. The
// recognized values are:
//
//   - "plain"    (the default — PlainRenderer)
//   - "tui"      (today's lipgloss-styled tui.Renderer)
//   - "notebook" (Bubble Tea notebook UI)
//
// Returns "" when --mode is absent — examples typically branch on
// "" the same way they branch on "plain". The bare --tui and --note
// flags are honored as aliases for "tui" and "notebook"
// respectively so callers can write either form.
//
// Unrecognized mode strings are returned as-is so the caller can
// surface a helpful error rather than silently falling back.
func Mode() string {
	args := os.Args[1:]
	for i, a := range args {
		if a == "--mode" && i+1 < len(args) {
			return args[i+1]
		}
		if strings.HasPrefix(a, "--mode=") {
			return strings.TrimPrefix(a, "--mode=")
		}
	}
	if slices.Contains(args, "--note") {
		return "notebook"
	}
	if slices.Contains(args, "--tui") {
		return "tui"
	}
	return ""
}

// IsNonInteractive reports whether --non-interactive appears in
// os.Args. Examples use this to skip live-interaction steps in CI runs
// or replay-only paths. demokit's own Execute also honors the flag via
// scanOwnArgs (or RegisterFlags); this helper is for callers that need
// to branch on the same signal before calling Execute.
func IsNonInteractive() bool {
	return slices.Contains(os.Args[1:], "--non-interactive")
}
