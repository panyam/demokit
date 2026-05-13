package demokit

import (
	"strings"
	"time"
)

// ActorDef defines a participant in the sequence diagram.
type ActorDef struct {
	ID    string `json:"id"`    // Short identifier used in arrows (e.g., "AS")
	Label string `json:"label"` // Display label (e.g., "Auth Server")
}

// Actor creates an ActorDef.
func Actor(id, label string) ActorDef {
	return ActorDef{ID: id, Label: label}
}

// Ref is a named reference (RFC, CVE, blog post, spec section, etc.).
type Ref struct {
	Name string `json:"name"` // e.g., "RFC 7519 (JWT)" or "CVE-2015-9235"
	URL  string `json:"url"`  // e.g., "https://www.rfc-editor.org/rfc/rfc7519"
}

// item is a union type for the ordered sequence of steps and sections.
type item interface {
	isItem()
}

// StepDef defines one executable step in the demo.
type StepDef struct {
	demo        *Demo // back-pointer set by Demo.Step (and Demo.Bind) so renderers can query demo-wide flags (BoxedVerbatim, ...). Nil only for steps constructed standalone in tests.
	id          string
	title       string
	arrows      []arrowDef
	refs        []Ref
	note        string
	inputs      []InputDef
	verbatim    []verbatimBlock
	coalesce    func(map[string]any) any
	runFn       func(StepContext) *StepResult
	timeout      time.Duration // 0 = no timeout
	cancellable  bool          // press-Enter cancels in interactive mode
	inputTimeout time.Duration // per-step prompt deadline; 0 = inherit Demo.InputTimeout
}

// verbatimBlock holds character-exact content for copy-paste-friendly
// display. Stored author-time on StepDef; renderers look it up via
// VerbatimBlocks(). Same lifecycle as note/refs — not part of TraceEntry.
//
// A block may carry multiple Variants when the author wants to show the
// same task expressed several ways (curl / gcloud CLI / Python). Single-
// snippet blocks are stored as a one-element Variants slice — renderers
// don't branch on the count, they iterate uniformly.
type verbatimBlock struct {
	Label    string
	Variants []Variant
}

// Variant is one labeled form of a verbatim snippet. Multi-variant blocks
// declare N variants and renderers pick which to show (or, in markdown,
// emit all of them with sub-labels). IsDefault marks the form preferred
// by non-interactive contexts and the demo-defined "copy this one"
// target; it has no effect when only one variant is declared.
//
// Construct via MakeVariant("curl", "bash", "...").Default() for the
// fluent form, or as a struct literal — both are supported.
type Variant struct {
	Label     string // "curl", "gcloud", "Python" — empty for single-variant blocks
	Lang      string // fenced-code language hint
	Content   string
	IsDefault bool
}

// MakeVariant constructs a Variant. Use .Default() to mark the preferred
// form for non-interactive output and the keyboard-copy target.
//
// Named MakeVariant (not Variant) because the type is already named
// Variant — Go doesn't allow a function and a type to share a name.
func MakeVariant(label, lang, content string) Variant {
	return Variant{Label: label, Lang: lang, Content: content}
}

// Default marks this variant as the preferred form. At most one variant
// per block is expected to carry IsDefault=true; if multiple are marked,
// the first wins. If none is marked, "default" behaves as "first".
func (v Variant) Default() Variant {
	v.IsDefault = true
	return v
}

// VariantView is the read-only projection of a Variant exposed to
// renderers and JSON consumers. JSON field is named "default" for wire
// stability — embed hosts read it as the marker for the preferred form.
type VariantView struct {
	Label     string `json:"label,omitempty"`
	Lang      string `json:"lang,omitempty"`
	Content   string `json:"content"`
	IsDefault bool   `json:"default,omitempty"`
}

// VerbatimView is the read-only projection of a verbatim block exposed
// to renderers and JSON consumers. Single-variant blocks produce a
// one-element Variants slice; renderers treat all blocks uniformly.
type VerbatimView struct {
	Label    string        `json:"label,omitempty"`
	Variants []VariantView `json:"variants"`
}

// VariantSelection encodes a renderer's filter over a block's variants.
// Renderers consult it to decide which variants to emit when the user
// has set --variant on the CLI.
//
// Use the constructors (VariantSelectionAll, VariantSelectionDefault,
// VariantSelectionNamed) rather than the struct literal — internal
// fields may shift.
type VariantSelection struct {
	all      bool
	defOnly  bool   // strict: error if no Default-marked variant exists
	name     string // empty = no name filter
}

// VariantSelectionAll renders every variant of every block.
func VariantSelectionAll() VariantSelection {
	return VariantSelection{all: true}
}

// VariantSelectionDefault renders only Default-marked variants. Blocks
// with no Default-marked variant fall back to all variants. Use this
// when "--variant" is unset and the demo wants the implicit-default
// behavior.
func VariantSelectionDefault() VariantSelection {
	return VariantSelection{}
}

// VariantSelectionNamed renders only variants whose Label matches name
// (case-insensitive). Blocks with no matching variant are dropped from
// output. Used when the user passes --variant=<label>.
func VariantSelectionNamed(name string) VariantSelection {
	return VariantSelection{name: name}
}

// Apply filters block.Variants according to the selection rules. The
// returned slice is always a fresh allocation so callers may mutate.
func (s VariantSelection) Apply(in []Variant) []Variant {
	if s.all || len(in) == 0 {
		out := make([]Variant, len(in))
		copy(out, in)
		return out
	}
	if s.name != "" {
		var out []Variant
		for _, v := range in {
			if strings.EqualFold(v.Label, s.name) {
				out = append(out, v)
			}
		}
		return out
	}
	// Default behavior.
	for _, v := range in {
		if v.IsDefault {
			return []Variant{v}
		}
	}
	// No Default marked — render everything so non-interactive output
	// stays lossless when the author didn't specify a preference.
	out := make([]Variant, len(in))
	copy(out, in)
	return out
}

type arrowDef struct {
	from, to, label string
	dashed          bool // -->> vs ->>
}

// ArrowView is a read-only view of an arrow for use by renderers.
type ArrowView struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Label  string `json:"label,omitempty"`
	Dashed bool   `json:"dashed,omitempty"`
}

func (s *StepDef) isItem() {}

// ID assigns a stable identifier to this step. IDs are used as jump
// targets by StepResult.Next and by recordings. If unset, the demo
// auto-assigns "step-N" based on declaration order at Execute time.
func (s *StepDef) ID(id string) *StepDef {
	s.id = id
	return s
}

// Arrow adds a solid arrow (request) to the step's sequence diagram.
func (s *StepDef) Arrow(from, to, label string) *StepDef {
	s.arrows = append(s.arrows, arrowDef{from: from, to: to, label: label})
	return s
}

// DashedArrow adds a dashed arrow (response) to the step's sequence diagram.
func (s *StepDef) DashedArrow(from, to, label string) *StepDef {
	s.arrows = append(s.arrows, arrowDef{from: from, to: to, label: label, dashed: true})
	return s
}

// Ref adds a reference (RFC, CVE, spec section, blog post, etc.) to this step.
func (s *StepDef) Ref(ref Ref) *StepDef {
	s.refs = append(s.refs, ref)
	return s
}

// Note adds explanatory text shown in both CLI and README. Accepts
// either a single string or multiple line/paragraph fragments which
// are joined with "\n" — handy for assembling bullet lists without
// hand-rolled string concatenation:
//
//	step.Note("One-liner.")
//	step.Note(
//	    "Lead sentence.",
//	    "",
//	    "- bullet 1",
//	    "- bullet 2",
//	)
//
// Calling Note() with no args leaves the note empty (so spreading an
// empty slice via Note(parts...) is a no-op rather than an error).
func (s *StepDef) Note(text ...string) *StepDef {
	s.note = strings.Join(text, "\n")
	return s
}

// Input declares an input the renderer should collect before this step's
// Run executes. Inputs prompt in declaration order and the parsed values
// are placed into StepContext.Inputs keyed by InputDef.Name.
//
// If a previously-declared input has the same Name (typically populated
// by FromMarkdown), this call replaces it in place — preserving order.
// Otherwise the new input is appended. This lets sidecar-md authors
// declare inputs by name and Go callers selectively override one
// input's parser via .Input(demokit.Choice(...).Named("x", ...).WithParse(custom)).
func (s *StepDef) Input(d InputDef) *StepDef {
	for i := range s.inputs {
		if s.inputs[i].Name == d.Name && d.Name != "" {
			s.inputs[i] = d
			return s
		}
	}
	s.inputs = append(s.inputs, d)
	return s
}

// Verbatim attaches a copy-paste-friendly content block to this step.
// Content is preserved character-exact across all renderers. By default
// the TUI renders it outside the bordered box so wrapping cannot inject
// border chars into the middle of a line — call Demo.BoxedVerbatim() to
// render every verbatim inside a styled box with keyboard copy instead.
// Multiple calls render in declaration order. Empty label is allowed
// (skips the heading in markdown, omits the label line in the TUI).
// Use VerbatimLang to specify a fenced-code language hint for markdown.
func (s *StepDef) Verbatim(label, content string) *StepDef {
	return s.VerbatimLang(label, "", content)
}

// VerbatimLang is Verbatim with an explicit fenced-code language hint
// (used by markdown / HTML renderers). Pass an empty lang for an
// unfenced default.
func (s *StepDef) VerbatimLang(label, lang, content string) *StepDef {
	s.verbatim = append(s.verbatim, verbatimBlock{
		Label:    label,
		Variants: []Variant{{Lang: lang, Content: content}},
	})
	return s
}

// Shell is a shorthand for VerbatimLang("", "bash", content) — the
// common case of attaching a copy-pasteable shell snippet without a
// label.
func (s *StepDef) Shell(content string) *StepDef {
	return s.VerbatimLang("", "bash", content)
}

// VerbatimVariants attaches a multi-variant verbatim block to this step
// — one labeled snippet rendered N ways (curl / gcloud / Python ...).
// At most one variant should carry Default=true (via .Default()); if
// none does, the first declared is the implicit default for non-
// interactive output and keyboard copy.
//
// Multi-variant blocks always render inside the TUI's bordered box
// regardless of Demo.BoxedVerbatim() — the per-variant labels need a
// frame to be readable.
//
// Markdown / HTML output emits every variant labeled in declaration
// order; --variant=<label> at the CLI filters to a single one.
func (s *StepDef) VerbatimVariants(label string, variants ...Variant) *StepDef {
	s.verbatim = append(s.verbatim, verbatimBlock{
		Label:    label,
		Variants: append([]Variant(nil), variants...),
	})
	return s
}

// VerbatimBlocks returns a read-only view of this step's attached
// verbatim blocks in declaration order.
func (s *StepDef) VerbatimBlocks() []VerbatimView {
	out := make([]VerbatimView, len(s.verbatim))
	for i, v := range s.verbatim {
		vars := make([]VariantView, len(v.Variants))
		for j, va := range v.Variants {
			vars[j] = VariantView{Label: va.Label, Lang: va.Lang, Content: va.Content, IsDefault: va.IsDefault}
		}
		out[i] = VerbatimView{Label: v.Label, Variants: vars}
	}
	return out
}

// NumberedCopyable is one variant exposed to the interactive copy UI
// with a stable 1-based number. Renderers print "[N] label" alongside
// the variant content; the pause prompt reads N back to identify what
// to copy via demokit.Copy(content). Numbering is global across all
// copyable blocks on the step, in declaration order, after the
// demo's --variant filter is applied.
type NumberedCopyable struct {
	N       int    // 1-based number shown to the user
	Label   string // variant label (or block label fallback) for status messages
	Lang    string // fenced-code hint, for renderers that highlight
	Content string // raw content; copied byte-exact
}

// NumberedCopyables returns the flat numbered list of copyable variants
// on this step, in the order renderers should emit them. Used by both
// PlainRenderer (digit-to-copy line prompt) and the TUI Bubble Tea
// overlay so they share a single numbering scheme.
//
// A variant is "copyable" when its block is:
//   - multi-variant (always copyable — variants are the whole point), or
//   - single-variant inside a demo that opted into Demo.BoxedVerbatim()
//     (single-variant unboxed blocks stay mouse-select-friendly today
//     and don't expose a copy keystroke).
//
// The demo's --variant filter is **not** applied here — interactive
// renderers always show every variant so the user can pick which to
// copy by number. The filter is a documentation concern (markdown /
// HTML / JSON output); reducing the interactive list to a single
// default-marked variant would silently strip the choice the digit
// prompt is supposed to expose.
func (s *StepDef) NumberedCopyables() []NumberedCopyable {
	if s == nil {
		return nil
	}
	demo := s.demo
	boxedDefault := demo != nil && demo.IsBoxedVerbatim()
	var out []NumberedCopyable
	for _, b := range s.verbatim {
		if len(b.Variants) == 0 {
			continue
		}
		copyable := boxedDefault || len(b.Variants) > 1
		if !copyable {
			continue
		}
		for _, va := range b.Variants {
			label := va.Label
			if label == "" {
				label = b.Label
			}
			out = append(out, NumberedCopyable{
				N:       len(out) + 1,
				Label:   label,
				Lang:    va.Lang,
				Content: va.Content,
			})
		}
	}
	return out
}

// Coalesce attaches a function that converts the raw inputs map into a
// single typed payload, available to the step as StepContext.Input. If
// not set, ctx.Input == ctx.Inputs (the map itself).
func (s *StepDef) Coalesce(fn func(map[string]any) any) *StepDef {
	s.coalesce = fn
	return s
}

// Inputs returns a read-only view of this step's declared inputs.
func (s *StepDef) Inputs() []InputDef {
	out := make([]InputDef, len(s.inputs))
	copy(out, s.inputs)
	return out
}

// Timeout sets a deadline for this step's Run. After the duration
// elapses, ctx.Ctx fires its Done() channel; Run is expected to
// notice and return. demokit does NOT abandon a Run that ignores
// the context — the demo will block until Run returns. Zero
// duration (default) means no timeout.
//
// Common pattern for long-running event watchers:
//
//	demo.Step("Watch events").
//	    Timeout(5 * time.Minute).
//	    Run(func(ctx demokit.StepContext) *demokit.StepResult {
//	        for {
//	            select {
//	            case ev := <-events:
//	                fmt.Printf("[event] %v\n", ev)
//	            case <-ctx.Ctx.Done():
//	                return demokit.Info("watched")
//	            }
//	        }
//	    })
func (s *StepDef) Timeout(d time.Duration) *StepDef {
	s.timeout = d
	return s
}

// InputTimeout sets a deadline for collecting this step's declared
// inputs. When the deadline elapses with no submission, renderers
// that honor the contract (currently `web.webRenderer` for `--serve`
// mode) fill in declared defaults and continue. Overrides the
// demo-level Demo.InputTimeout for this one step.
//
// Zero (the default) means inherit the demo-level timeout. To
// explicitly opt out of the demo default for a step, set a very
// large duration; a future enhancement may add a sentinel.
func (s *StepDef) InputTimeout(d time.Duration) *StepDef {
	s.inputTimeout = d
	return s
}

// Cancellable, when true, lets the user press Enter during Run to
// cancel the step's context (in interactive mode only). Run must
// select on ctx.Ctx.Done() to honor the cancellation. Has no effect
// in --non-interactive mode or replay mode.
//
// Combine with Timeout to set both an upper bound and a manual
// escape hatch — whichever fires first cancels the context.
func (s *StepDef) Cancellable(b bool) *StepDef {
	s.cancellable = b
	return s
}

// Run sets the function to execute for this step. The function receives a
// StepContext carrying inputs and visit count, and returns a *StepResult
// (or nil for success with no message). Use named return for brevity:
//
//	step.Run(func(ctx demokit.StepContext) (result *demokit.StepResult) {
//	    fmt.Println("did the thing")
//	    return // nil = success
//	})
//
// Set result.Next to a step ID to jump in graph mode. Empty Next falls
// through to the next item in declaration order.
func (s *StepDef) Run(fn func(StepContext) *StepResult) *StepDef {
	s.runFn = fn
	return s
}

// --- Read accessors for renderers ---

// StepID returns the step's identifier (auto-assigned at Execute time if unset).
func (s *StepDef) StepID() string { return s.id }

// Demo returns the parent demo for this step, or nil if the step was
// constructed standalone (e.g. in tests). Renderers use this to query
// demo-wide flags like IsBoxedVerbatim that affect how this step
// renders.
func (s *StepDef) Demo() *Demo { return s.demo }

// Title returns the step title.
func (s *StepDef) Title() string { return s.title }

// NoteText returns the step's explanatory note (may be empty).
func (s *StepDef) NoteText() string { return s.note }

// Refs returns the step's references.
func (s *StepDef) Refs() []Ref { return s.refs }

// Arrows returns a read-only view of the step's sequence diagram arrows.
func (s *StepDef) Arrows() []ArrowView {
	out := make([]ArrowView, len(s.arrows))
	for i, a := range s.arrows {
		out[i] = ArrowView{From: a.from, To: a.to, Label: a.label, Dashed: a.dashed}
	}
	return out
}

// SectionDef is a non-executable block of explanatory content.
type SectionDef struct {
	title string
	body  string
}

func (s *SectionDef) isItem() {}

// Title returns the section title.
func (s *SectionDef) Title() string { return s.title }

// Body returns the section body text.
func (s *SectionDef) Body() string { return s.body }
