package notebook

import (
	"fmt"
	"strconv"
	"strings"
)

// Input describes one field of a PromptCell. Concrete types own
// their Parse + Hint logic; cells don't type-switch on them for
// behavior. notebook.Notebook.AwaitInput accepts []Input and
// returns the parsed answer map.
//
// The Input* method names avoid colliding with the matching
// struct fields on the concrete types (Name/Prompt/Default).
type Input interface {
	InputName() string
	InputPrompt() string
	InputDefault() any // nil if no default
	// Parse converts the user's typed string into the input's
	// concrete type. PromptCell calls this on submit; an error
	// surfaces in the cell's per-field error row.
	Parse(raw string) (any, error)
	// Hint returns the per-field hint row PromptCell renders under
	// the prompt (e.g. "options: a · b · c"). Empty string means
	// "no hint row" — the default-display row is added separately
	// by PromptCell.
	Hint() string
}

// StringInput collects a free-text string. Default is shown as
// placeholder; an empty submit substitutes Default if non-nil.
type StringInput struct {
	Name    string
	Prompt  string
	Default any // string or nil
}

// NewStringInput is a convenience constructor.
func NewStringInput(name, prompt string, def any) StringInput {
	return StringInput{Name: name, Prompt: prompt, Default: def}
}

// InputName implements Input.
func (s StringInput) InputName() string { return s.Name }

// InputPrompt implements Input.
func (s StringInput) InputPrompt() string { return s.Prompt }

// InputDefault implements Input.
func (s StringInput) InputDefault() any { return s.Default }

// Parse implements Input.
func (s StringInput) Parse(raw string) (any, error) { return raw, nil }

// Hint implements Input — StringInput has no type-specific hint.
func (s StringInput) Hint() string { return "" }

// IntInput collects an integer. Parse returns int(0) on success.
type IntInput struct {
	Name    string
	Prompt  string
	Default any // int or nil
}

// NewIntInput is a convenience constructor.
func NewIntInput(name, prompt string, def any) IntInput {
	return IntInput{Name: name, Prompt: prompt, Default: def}
}

func (i IntInput) InputName() string   { return i.Name }
func (i IntInput) InputPrompt() string { return i.Prompt }
func (i IntInput) InputDefault() any   { return i.Default }
func (i IntInput) Hint() string        { return "" }
func (i IntInput) Parse(raw string) (any, error) {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("not an integer: %q", raw)
	}
	return n, nil
}

// ChoiceInput collects one of a fixed set of options. Match is
// case-insensitive; the returned answer is the canonical (declared)
// casing.
type ChoiceInput struct {
	Name    string
	Prompt  string
	Default any // string (one of Options) or nil
	Options []string
}

// NewChoiceInput is a convenience constructor.
func NewChoiceInput(name, prompt string, def any, options []string) ChoiceInput {
	return ChoiceInput{Name: name, Prompt: prompt, Default: def, Options: options}
}

func (c ChoiceInput) InputName() string   { return c.Name }
func (c ChoiceInput) InputPrompt() string { return c.Prompt }
func (c ChoiceInput) InputDefault() any   { return c.Default }
func (c ChoiceInput) Hint() string {
	if len(c.Options) == 0 {
		return ""
	}
	return "options: " + strings.Join(c.Options, " · ")
}
func (c ChoiceInput) Parse(raw string) (any, error) {
	got := strings.TrimSpace(raw)
	for _, opt := range c.Options {
		if strings.EqualFold(got, opt) {
			return opt, nil
		}
	}
	return nil, fmt.Errorf("must be one of: %s", strings.Join(c.Options, ", "))
}
