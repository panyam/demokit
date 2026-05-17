package notebook

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/term"

	"github.com/panyam/demokit"
)

// Renderer implements demokit.Renderer + demokit.StreamingRenderer
// on top of an event-sourced architecture. Each lifecycle method
// appends an Event to the renderer's queue; the Bubble Tea model
// drains the queue and projects events into cell state.
//
// The queue makes the producer race-free against BT startup:
// events queued before Run() has finished initializing are still
// applied in order when the model's first drain fires.
//
// Sync points (WaitForStep, Prompt) carry their per-call rendezvous
// channels in the event payload; the model installs them in its
// state and the user's keypress closes them. RenderDone blocks
// the producer goroutine until progDone closes.
type Renderer struct {
	once     sync.Once
	program  *tea.Program
	progDone chan struct{}

	queue *eventQueue

	mu                sync.Mutex
	currentVisit      int
	pendingOutputCell Cell          // OutputCell waiting for eventStepReadyToRun flush
	pendingOutputBuf  *OutputBuffer // buffer for the pending OutputCell
	stepCount         int
	palette           Palette

	killed bool
}

// NewRenderer constructs a fresh notebook renderer. Palette is
// auto-detected; override via WithPalette before the first Render
// call.
func NewRenderer() *Renderer {
	return &Renderer{
		queue:   newEventQueue(),
		palette: DefaultPalette(),
	}
}

// WithPalette overrides the palette used to construct cells.
func (r *Renderer) WithPalette(p Palette) *Renderer {
	r.palette = p
	return r
}

// ensureProgram lazily starts the tea.Program in a background
// goroutine.
func (r *Renderer) ensureProgram() {
	r.once.Do(func() {
		// Snapshot termios so we can guarantee restoration even
		// if BT's own cleanup misses a flag on signal-driven exit.
		var origTermState *term.State
		if fd := os.Stdin.Fd(); term.IsTerminal(fd) {
			origTermState, _ = term.GetState(fd)
		}

		m := New(nil).WithQueue(r.queue).WithPalette(r.palette)
		r.program = tea.NewProgram(m, tea.WithAltScreen())
		r.progDone = make(chan struct{})
		go func() {
			defer close(r.progDone)
			_, _ = r.program.Run()
			if origTermState != nil {
				_ = term.Restore(os.Stdin.Fd(), origTermState)
			}
			// CR-LF guards against alt-screen exit leaving the
			// cursor mid-row.
			fmt.Print("\r\n")
			r.mu.Lock()
			r.killed = true
			r.mu.Unlock()
			// BT has exited (q / Ctrl+C / Ctrl+D / Done+Enter).
			// demokit.Execute may still be mid-step — its runFn
			// reading from an empty Inputs map will panic.
			// os.Exit cuts the cord cleanly. Deferred cleanup in
			// main() is skipped, which is acceptable for a demo
			// program; the recorder has already flushed every
			// step before this point.
			os.Exit(0)
		}()
	})
}

// append is a thin wrapper that suppresses event emission after
// the program has exited (Ctrl+C / q during the demo).
func (r *Renderer) append(e Event) {
	r.mu.Lock()
	dead := r.killed
	r.mu.Unlock()
	if dead {
		return
	}
	r.queue.Append(e)
}

// --- demokit.Renderer ---

// RenderHeader emits eventHeader once at the start of the demo.
func (r *Renderer) RenderHeader(title, description string, stepCount int) {
	r.ensureProgram()
	r.stepCount = stepCount
	r.append(eventHeader{Title: title, Description: description, StepCount: stepCount})
}

// RenderStep emits eventStepStart with the step's body cells.
// The OutputCell is stashed for emission at eventStepReadyToRun
// time (after the user has signalled "run").
func (r *Renderer) RenderStep(stepNum, totalSteps int, step *demokit.StepDef) {
	r.ensureProgram()
	bodyCells, outputCell, buf, _ := cellsForStep(stepNum, step, r.palette)
	r.mu.Lock()
	r.currentVisit = stepNum
	r.pendingOutputCell = outputCell
	r.pendingOutputBuf = buf
	r.mu.Unlock()
	r.append(eventStepStart{Visit: stepNum, StepID: step.StepID(), BodyCells: bodyCells})
}

// appendOutputCell emits eventStepReadyToRun with the OutputCell
// stashed by RenderStep. Called by WaitForStep / Prompt after the
// user has signalled "run." Safe to call multiple times — the
// second call is a no-op because pendingOutputCell is nil'd.
func (r *Renderer) appendOutputCell() {
	r.mu.Lock()
	cell := r.pendingOutputCell
	buf := r.pendingOutputBuf
	visit := r.currentVisit
	r.pendingOutputCell = nil
	r.pendingOutputBuf = nil
	r.mu.Unlock()
	if cell == nil {
		return
	}
	r.append(eventStepReadyToRun{Visit: visit, Output: cell, OutputBuf: buf})
}

// RenderResult emits eventStepEnd for the current step.
func (r *Renderer) RenderResult(stepNum int, output string, result *demokit.StepResult) {
	r.ensureProgram()
	// Non-streaming captured output flushed as a single chunk for
	// renderers that didn't get StreamOutput calls.
	if output != "" {
		r.append(eventOutputChunk{Visit: stepNum, Chunk: []byte(output)})
	}
	r.append(eventStepEnd{Visit: stepNum, Result: result})
}

// RenderSection emits eventSection.
func (r *Renderer) RenderSection(section *demokit.SectionDef) {
	r.ensureProgram()
	r.append(eventSection{Title: section.Title(), Body: section.Body()})
}

// RenderDone emits eventDone and blocks until the program exits.
func (r *Renderer) RenderDone() {
	r.ensureProgram()
	r.append(eventDone{})
	<-r.progDone
}

// WaitForStep emits eventWaitForAdvance and blocks on the channel
// the model closes when the user presses Enter. Flushes the
// pending OutputCell on release so it appears just before the
// step's output starts.
func (r *Renderer) WaitForStep(opts demokit.WaitOpts) {
	r.mu.Lock()
	dead := r.killed
	r.mu.Unlock()
	if dead {
		return
	}
	r.ensureProgram()
	ch := make(chan struct{})
	r.append(eventWaitForAdvance{Visit: r.currentVisit, Done: ch})
	select {
	case <-ch:
		r.appendOutputCell()
	case <-r.progDone:
	}
}

// Prompt emits eventPromptOpen and blocks on the reply channel.
// Flushes the pending OutputCell after submission.
func (r *Renderer) Prompt(stepID string, inputs []demokit.InputDef) map[string]any {
	r.mu.Lock()
	dead := r.killed
	r.mu.Unlock()
	if dead {
		return map[string]any{}
	}
	r.ensureProgram()
	reply := make(chan map[string]any, 1)
	r.append(eventPromptOpen{Visit: r.currentVisit, Inputs: promptInputsFrom(inputs), Reply: reply})
	select {
	case ans, ok := <-reply:
		r.appendOutputCell()
		if !ok || ans == nil {
			return map[string]any{}
		}
		return ans
	case <-r.progDone:
		return map[string]any{}
	}
}

// --- demokit.StreamingRenderer ---

// StreamOutput emits eventOutputChunk for every captured chunk.
// The model's Apply routes by Visit to the right OutputCell's
// buffer — and keeps routing even after eventStepEnd, so a step
// that spawns a background goroutine can keep feeding its cell.
func (r *Renderer) StreamOutput(stepNum int, chunk []byte, out io.Writer) {
	if len(chunk) == 0 {
		return
	}
	// Defensive copy: captureOutput may reuse the chunk slice
	// across calls; events are stored in the queue indefinitely.
	c := make([]byte, len(chunk))
	copy(c, chunk)
	r.append(eventOutputChunk{Visit: stepNum, Chunk: c})
}

// --- helpers ---

// cellsForStep builds the cell list for one step visit, split
// into body + OutputCell. Body always shown immediately at
// eventStepStart; OutputCell deferred to eventStepReadyToRun.
func cellsForStep(visit int, s *demokit.StepDef, palette Palette) ([]Cell, Cell, *OutputBuffer, string) {
	base := slugify(s.StepID())
	if base == "" {
		base = fmt.Sprintf("step%d", visit)
	}

	body := buildMetaBody(s)
	metaID := fmt.Sprintf("%s#%d.meta", base, visit)
	meta := NewMetaCell(metaID, s.Title(), body)
	meta.SetPalette(palette)
	cells := []Cell{meta}

	for i, vb := range s.VerbatimBlocks() {
		vid := fmt.Sprintf("%s#%d.verbatim%d", base, visit, i)
		vc := NewVerbatimCell(vid, vb.Label, variantsFromView(vb.Variants))
		vc.SetPalette(palette)
		cells = append(cells, vc)
	}

	buf := NewOutputBuffer()
	outputID := fmt.Sprintf("%s#%d.output", base, visit)
	oc := NewOutputCell(outputID, buf, 12)
	oc.SetPalette(palette)
	return cells, oc, buf, outputID
}

// buildMetaBody renders the step's note + arrows + refs into a
// single body string for the MetaCell.
func buildMetaBody(s *demokit.StepDef) string {
	var parts []string
	if note := strings.TrimSpace(s.NoteText()); note != "" {
		parts = append(parts, note)
	}
	if arrows := s.Arrows(); len(arrows) > 0 {
		var lines []string
		for _, a := range arrows {
			arrow := "->"
			if a.Dashed {
				arrow = "-->"
			}
			label := ""
			if a.Label != "" {
				label = ": " + a.Label
			}
			lines = append(lines, fmt.Sprintf("%s %s %s%s", a.From, arrow, a.To, label))
		}
		parts = append(parts, strings.Join(lines, "\n"))
	}
	if refs := s.Refs(); len(refs) > 0 {
		var lines []string
		for _, ref := range refs {
			line := ref.Name
			if ref.URL != "" {
				if line == "" {
					line = ref.URL
				} else {
					line = line + " (" + ref.URL + ")"
				}
			}
			lines = append(lines, "ref: "+line)
		}
		parts = append(parts, strings.Join(lines, "\n"))
	}
	return strings.Join(parts, "\n\n")
}

// variantsFromView converts demokit.VariantView to demokit.Variant.
func variantsFromView(vs []demokit.VariantView) []demokit.Variant {
	out := make([]demokit.Variant, len(vs))
	for i, v := range vs {
		out[i] = demokit.Variant{Label: v.Label, Lang: v.Lang, Content: v.Content, IsDefault: v.IsDefault}
	}
	return out
}

// promptInputsFrom projects demokit.InputDef into the cell-side
// promptInput shape, capturing the Parse closure.
func promptInputsFrom(inputs []demokit.InputDef) []promptInput {
	out := make([]promptInput, len(inputs))
	for i, in := range inputs {
		out[i] = promptInput{
			Name:    in.Name,
			Prompt:  in.Prompt,
			Default: in.Default,
			Kind:    in.Kind,
			Options: in.Options,
			parse:   in.Parse,
		}
	}
	return out
}

// slugify lowercases and dash-normalizes a string for cell IDs.
func slugify(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return ""
	}
	var b strings.Builder
	prevDash := true
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevDash = false
		case r == '-' || r == '_':
			b.WriteByte('-')
			prevDash = true
		default:
			if !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	return strings.TrimRight(b.String(), "-")
}
