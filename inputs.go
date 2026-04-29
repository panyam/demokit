package demokit

import (
	"fmt"
	"strconv"
	"strings"
)

// InputDef declares a single input the renderer should collect before a
// step runs. Parse converts the raw string the user types into the typed
// value placed into StepContext.Inputs[Name]. A non-nil error from Parse
// causes the renderer to re-prompt.
//
// Build InputDefs via the helpers (String, Int, Choice) and chain Named,
// Default, WithParse, etc. to customize.
type InputDef struct {
	Name    string                    // map key in StepContext.Inputs
	Prompt  string                    // user-facing label; defaults to Name
	Default any                       // shown in brackets; Enter accepts it
	Parse   func(string) (any, error) // returns the typed value or a retry error
}

// Named sets the input's identifier and user-facing prompt label. The
// name becomes the key in StepContext.Inputs.
func (d InputDef) Named(name, prompt string) InputDef {
	d.Name = name
	d.Prompt = prompt
	return d
}

// WithDefault sets a value used when the user submits an empty line.
// The default must be the same Go type as Parse returns — it is placed
// into the inputs map verbatim, not re-parsed.
func (d InputDef) WithDefault(v any) InputDef {
	d.Default = v
	return d
}

// WithParse swaps in a custom parser. Use to layer extra validation on
// top of a base helper, e.g. demokit.Int().WithParse(rangeCheck).
func (d InputDef) WithParse(p func(string) (any, error)) InputDef {
	d.Parse = p
	return d
}

// String returns an InputDef whose Parse returns the raw line unchanged.
// Useful for free-form text.
func String() InputDef {
	return InputDef{Parse: func(s string) (any, error) { return s, nil }}
}

// Int returns an InputDef that parses the line as a base-10 integer.
func Int() InputDef {
	return InputDef{Parse: func(s string) (any, error) {
		n, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil {
			return nil, fmt.Errorf("not a valid integer: %q", s)
		}
		return n, nil
	}}
}

// Choice returns an InputDef that accepts only one of the given values.
// Matching is case-insensitive against the trimmed input. The returned
// value is the canonical form from opts.
func Choice(opts ...string) InputDef {
	return InputDef{Parse: func(s string) (any, error) {
		got := strings.TrimSpace(s)
		for _, opt := range opts {
			if strings.EqualFold(got, opt) {
				return opt, nil
			}
		}
		return nil, fmt.Errorf("must be one of: %s", strings.Join(opts, ", "))
	}}
}
