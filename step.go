package demokit

// ActorDef defines a participant in the sequence diagram.
type ActorDef struct {
	ID    string // Short identifier used in arrows (e.g., "AS")
	Label string // Display label (e.g., "Auth Server")
}

// Actor creates an ActorDef.
func Actor(id, label string) ActorDef {
	return ActorDef{ID: id, Label: label}
}

// Ref is a named reference (RFC, CVE, blog post, spec section, etc.).
type Ref struct {
	Name string // e.g., "RFC 7519 (JWT)" or "CVE-2015-9235"
	URL  string // e.g., "https://www.rfc-editor.org/rfc/rfc7519"
}

// item is a union type for the ordered sequence of steps and sections.
type item interface {
	isItem()
}

// StepDef defines one executable step in the demo.
type StepDef struct {
	id       string
	title    string
	arrows   []arrowDef
	refs     []Ref
	note     string
	inputs   []InputDef
	coalesce func(map[string]any) any
	runFn    func(StepContext) *StepResult
}

type arrowDef struct {
	from, to, label string
	dashed          bool // -->> vs ->>
}

// ArrowView is a read-only view of an arrow for use by renderers.
type ArrowView struct {
	From, To, Label string
	Dashed          bool
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

// Note adds explanatory text shown in both CLI and README.
func (s *StepDef) Note(text string) *StepDef {
	s.note = text
	return s
}

// Input declares an input the renderer should collect before this step's
// Run executes. Inputs prompt in declaration order and the parsed values
// are placed into StepContext.Inputs keyed by InputDef.Name.
func (s *StepDef) Input(d InputDef) *StepDef {
	s.inputs = append(s.inputs, d)
	return s
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
