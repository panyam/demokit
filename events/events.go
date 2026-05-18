// Package events defines the public event vocabulary demokit emits
// during Execute and the EventQueue that consumers drain. It's the
// substrate for the event-sourced architecture described in issue
// 18: demokit.Execute appends events at each lifecycle point;
// renderers (notebook, future TUI v2, web) consume them at their
// own pace.
//
// Events are pure data — no channels, no presentation types — so
// they serialize naturally for record/replay and stream cleanly
// across N consumers. Sync events (WaitForAdvance, PromptOpen)
// carry a Resolution field that's filled in when the user acts;
// the producer (Execute) blocks via the queue's AwaitResolution,
// not via a channel embedded in the event.
//
// The package deliberately doesn't import demokit. demokit imports
// events (one-way) and converts its internal step/input/arrow
// types into events projection types when emitting. That keeps
// the events package importable from any consumer (web, replay,
// doc emitters) without dragging in demokit's runtime.
package events

import "time"

// Event is the unit of demokit→consumer communication. Concrete
// types live in this package and implement isEvent() as a
// closed-set marker.
type Event interface {
	isEvent()
}

// --- Projection types (data-only, serializable) ---

// Arrow is the events-package projection of a sequence-diagram
// arrow on a step. Mirrors demokit.ArrowView; duplicated here so
// the events package stays demokit-independent.
type Arrow struct {
	From   string
	To     string
	Label  string
	Dashed bool
}

// Ref is a named reference (RFC, CVE, blog post, …).
type Ref struct {
	Name string
	URL  string
}

// Variant is one labeled form of a verbatim snippet.
type Variant struct {
	Label     string
	Lang      string
	Content   string
	IsDefault bool
}

// Verbatim is a copyable code/text block on a step, possibly
// with multiple variants.
type Verbatim struct {
	Label    string
	Variants []Variant
}

// Input is the closed-set marker interface for declared inputs.
// Concrete types — StringInput, IntInput, ChoiceInput — carry
// their own validation shape. Consumers type-switch on the
// concrete type to render + parse appropriately.
//
// The accessor methods (InputName, InputPrompt, InputDefault)
// let consumers iterate inputs without unwrapping for common
// chrome (labels, default hints) — the per-type validation
// still requires a type switch.
type Input interface {
	isInput()
	InputName() string
	InputPrompt() string
	InputDefault() any // nil if no default declared
}

// inputHeader is the common payload (name, prompt, default)
// embedded in every concrete Input type. Embedding inherits the
// accessor methods + the isInput marker without per-type
// boilerplate.
type inputHeader struct {
	Name    string
	Prompt  string
	Default any
}

func (h inputHeader) isInput()             {}
func (h inputHeader) InputName() string    { return h.Name }
func (h inputHeader) InputPrompt() string  { return h.Prompt }
func (h inputHeader) InputDefault() any    { return h.Default }

// StringInput declares a free-form text input.
type StringInput struct {
	inputHeader
}

// NewStringInput is a convenience constructor.
func NewStringInput(name, prompt string, def any) StringInput {
	return StringInput{inputHeader{Name: name, Prompt: prompt, Default: def}}
}

// IntInput declares an integer-valued input. Consumers parse
// the user's typing via strconv.Atoi (no min/max in Phase 2;
// add fields here when richer validation is needed).
type IntInput struct {
	inputHeader
}

// NewIntInput is a convenience constructor.
func NewIntInput(name, prompt string, def any) IntInput {
	return IntInput{inputHeader{Name: name, Prompt: prompt, Default: def}}
}

// ChoiceInput declares a one-of-N choice input. Options is the
// allowed set; match is case-insensitive on the trimmed user
// input.
type ChoiceInput struct {
	inputHeader
	Options []string
}

// NewChoiceInput is a convenience constructor.
func NewChoiceInput(name, prompt string, def any, options []string) ChoiceInput {
	return ChoiceInput{inputHeader{Name: name, Prompt: prompt, Default: def}, options}
}

// --- Events ---

// Header is emitted once at the start of Execute.
type Header struct {
	Title       string
	Description string
	StepCount   int
}

// Section is emitted for each non-executable explanatory block.
type Section struct {
	Title string
	Body  string
}

// StepStart is emitted at the start of each visited step. Carries
// the step's static metadata; consumers project this into their
// own cell/widget representation. The Run output and per-visit
// state arrive in subsequent events.
type StepStart struct {
	Visit     int
	StepID    string
	Title     string
	Note      string
	Arrows    []Arrow
	Refs      []Ref
	Verbatims []Verbatim
}

// StepReadyToRun marks the point where the user has signalled
// "advance" past any pause or prompt for this visit. Run is about
// to execute. Renderers that defer creating an output widget
// until run-time use this event as the trigger.
type StepReadyToRun struct {
	Visit int
}

// OutputChunk carries a chunk of captured stdout from a step's
// Run. Chunks for a Visit can keep arriving after StepEnd — a
// step's Run may spawn a background goroutine that emits
// indefinitely. Renderers route by Visit and apply chunks even
// after StepEnd.
type OutputChunk struct {
	Visit int
	Chunk []byte
}

// StepEnd is emitted when a step's Run function returns. Carries
// the result so renderers can display errors. Does NOT seal the
// step's output — late OutputChunk events for the same Visit
// continue to apply.
type StepEnd struct {
	Visit     int
	Status    string // "ok", "error", "warning", "info"
	Message   string
	ErrorText string // populated when Status == "error"
}

// Done is emitted once at demo end.
type Done struct{}

// --- Sync events ---

// WaitForAdvance is a sync event: demokit.Execute calls
// q.AppendBarrier(WaitForAdvance{...}); a consumer (notebook,
// web, etc.) resolves the queue offset with *AdvanceResolution
// when the user advances. AppendBarrier unblocks with that
// resolution value.
//
// Resolution data is NOT stored on the event — it lives in the
// queue's side map (q.Resolution(offset) to peek; the resolution
// returned by AppendBarrier or AwaitResolution is the canonical
// access). Keeping events as pure data preserves the immutable-
// log invariant.
type WaitForAdvance struct {
	Visit int
}

// AdvanceResolution carries the data resolving a WaitForAdvance.
// Source describes how the wait completed; useful for trace /
// replay annotations.
type AdvanceResolution struct {
	Source    string    // "user-enter", "user-space", "timeout", "auto-advance", "legacy-renderer"
	Timestamp time.Time
}

// PromptOpen is a sync event: demokit.Execute calls
// q.AppendBarrier(PromptOpen{...}); a consumer renders a prompt
// UI and on submit resolves the queue offset with
// *PromptResolution. Same first-writer-wins semantics as
// WaitForAdvance.
//
// Resolution data is NOT stored on the event (see
// WaitForAdvance docs).
type PromptOpen struct {
	Visit  int
	Inputs []Input
}

// PromptResolution carries the user's typed answers + provenance.
type PromptResolution struct {
	Answers   map[string]any
	Source    string // "user-submitted", "default", "replay", "legacy-renderer"
	Timestamp time.Time
}

func (Header) isEvent()         {}
func (Section) isEvent()        {}
func (StepStart) isEvent()      {}
func (StepReadyToRun) isEvent() {}
func (OutputChunk) isEvent()    {}
func (StepEnd) isEvent()        {}
func (Done) isEvent()           {}
func (WaitForAdvance) isEvent() {}
func (PromptOpen) isEvent()     {}
