package notebook

import "github.com/panyam/demokit"

// Event is the unit of demokit→renderer communication in the
// event-sourced architecture (issue 18). Every visible state
// change in the notebook UI corresponds to applying an event to
// the Model.
//
// Phase 1 keeps the Event vocabulary notebook-private; Phase 2
// promotes it to the demokit package so all renderers (Plain,
// TUI, Web, --doc) can consume the same stream.
//
// Each concrete type implements isEvent() as a closed-set marker
// — callers switch on the concrete type to apply state changes.
type Event interface {
	isEvent()
}

// eventHeader: emitted once at the start of a demo. The model
// stashes title/description for the banner row.
type eventHeader struct {
	Title       string
	Description string
	StepCount   int
}

// eventSection: emitted for each non-executable explanatory block.
// The model builds a SectionCell from these fields and appends.
type eventSection struct {
	Title string
	Body  string
}

// eventStepStart: emitted at the beginning of each visited step.
// Carries the cells that make up the step's body (Meta + 0..N
// Verbatim) so the model can append them; the OutputCell is
// deferred to eventStepReadyToRun (matching today's renderer
// flow). The model also stashes the step ID + visit so output
// chunks can be routed to the right cell.
type eventStepStart struct {
	Visit     int
	StepID    string
	BodyCells []Cell
}

// eventStepReadyToRun: emitted after the user has signalled "run"
// for this step (WaitForStep released or Prompt submitted). The
// model appends the OutputCell at the tail. Carries the buffer
// pointer the model retains so it can route output chunks via
// Apply(eventOutputChunk).
type eventStepReadyToRun struct {
	Visit    int
	Output   Cell
	OutputBuf *OutputBuffer
}

// eventOutputChunk: emitted for every chunk of step output. The
// model appends to the active OutputCell's buffer.
//
// Critically: chunks can arrive AFTER eventStepEnd for the same
// visit — a step's Run can spawn a background goroutine that
// keeps pushing data (live graph, tail -f, etc.). The model
// treats this uniformly; the "(end)" label on the cell becomes a
// hint, not a hard state.
type eventOutputChunk struct {
	Visit int
	Chunk []byte
}

// eventStepEnd: emitted when a step's Run function returns.
// Carries the result so the model can record errors. Does NOT
// seal the cell — output chunks for the same visit are still
// applied if they arrive.
type eventStepEnd struct {
	Visit  int
	Result *demokit.StepResult
}

// eventDone: emitted once at demo end (renderer.RenderDone). The
// model flips the banner to "Done." and the key handler treats
// Enter as exit.
type eventDone struct{}

// eventPromptOpen: emitted when a step has inputs to collect. The
// model installs a PromptCell with the given inputs and the
// reply channel; on submit, the cell sends the answer map and
// closes the channel. The producer (renderer.Prompt) blocks on
// the channel until that happens.
type eventPromptOpen struct {
	Visit  int
	Inputs []promptInput
	Reply  chan map[string]any
}

// promptInput is the cell-side projection of demokit.InputDef.
// Captures the Parse closure so PromptCell can validate without
// re-importing demokit at the cell layer.
type promptInput struct {
	Name    string
	Prompt  string
	Default any
	Kind    string
	Options []string
	parse   func(string) (any, error)
}

// eventWaitForAdvance: emitted when a step has no inputs and is
// waiting for the user to press Enter. The model stashes the
// channel; the key handler closes it on Enter. The producer
// (renderer.WaitForStep) blocks on the channel until that
// happens.
type eventWaitForAdvance struct {
	Visit int
	Done  chan struct{}
}

func (eventHeader) isEvent()          {}
func (eventSection) isEvent()         {}
func (eventStepStart) isEvent()       {}
func (eventStepReadyToRun) isEvent()  {}
func (eventOutputChunk) isEvent()     {}
func (eventStepEnd) isEvent()         {}
func (eventDone) isEvent()            {}
func (eventPromptOpen) isEvent()      {}
func (eventWaitForAdvance) isEvent()  {}
