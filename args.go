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
// Built-in strip set — the four flags demokit's dispatcher consumes:
//
//   - --tui                   (bare; selects the TUI renderer)
//   - --non-interactive       (bare; skips between-step pauses)
//   - --doc <format>          (value; routes to doc emission)
//   - --from <trace-path>     (value; trace input for --doc)
//
// Both `--flag value` and `--flag=value` forms are stripped for the
// value flags. Anything else passes through untouched.
//
// To strip caller-declared flags too (e.g. an example's own --serve or
// --url), pass them as extras built with [BoolFlag] or [ValueFlag]:
//
//	flag.CommandLine.Parse(demokit.FilterArgs(os.Args[1:],
//	    demokit.BoolFlag("--serve"),
//	    demokit.ValueFlag("--url"),
//	))
func FilterArgs(args []string, extra ...ExtraFlag) []string {
	bare := map[string]bool{
		"--tui":             true,
		"--non-interactive": true,
	}
	value := map[string]bool{
		"--doc":  true,
		"--from": true,
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

// IsTUI reports whether --tui appears in os.Args. Examples use this to
// gate the TUI renderer (`demo.WithRenderer(tui.New())`) without having
// to scan os.Args inline. Mirrors [FilterArgs]'s --tui recognition so
// the convention is honored consistently across the dispatcher and
// caller-side renderer selection.
func IsTUI() bool {
	return slices.Contains(os.Args[1:], "--tui")
}

// IsNonInteractive reports whether --non-interactive appears in
// os.Args. Examples use this to skip live-interaction steps in CI runs
// or replay-only paths. demokit's own Execute also honors the flag via
// scanOwnArgs (or RegisterFlags); this helper is for callers that need
// to branch on the same signal before calling Execute.
func IsNonInteractive() bool {
	return slices.Contains(os.Args[1:], "--non-interactive")
}
