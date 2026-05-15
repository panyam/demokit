package notebook

import (
	"fmt"
	"io"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/panyam/demokit"
)

// Renderer implements demokit.Renderer + demokit.StreamingRenderer
// on top of a running Bubble Tea program. It bridges demokit's
// procedural Execute loop to the model: each Render* method is a
// tea.Msg sent into the program; WaitForStep blocks on a channel
// the model closes when the user presses the advance key.
//
// Contract:
//
//   - Bubble Tea owns the screen for the entire demo lifetime
//     (lazy-started on the first Render call, exits on RenderDone
//     or when the user presses q).
//   - Single-step-on-screen: each RenderStep replaces the cell list
//     (cross-step navigation is Phase B).
//   - Step inputs are not supported in Phase A.2; Phase A.3 wires
//     a PromptCell driven by bubbles/textinput.
type Renderer struct {
	once     sync.Once
	program  *tea.Program
	progDone chan struct{}

	mu             sync.Mutex
	activeBuf      *OutputBuffer
	activeCellID   string
	stepCount      int
	palette        Palette

	// killed is set when the user pressed q (program exited).
	// Subsequent bridge calls become no-ops; WaitForStep returns
	// immediately so demokit's Execute loop can run to completion
	// without trying to drive a dead UI.
	killed bool
}

// NewRenderer constructs a fresh notebook renderer. The Bubble Tea
// program is started lazily on the first Render call so cheap test
// constructions don't grab a terminal. Default palette is auto-
// detected against the terminal background; override via
// WithPalette before the first Render call.
func NewRenderer() *Renderer {
	return &Renderer{palette: DefaultPalette()}
}

// WithPalette overrides the palette used to construct cells. Must
// be called before the first Render call.
func (r *Renderer) WithPalette(p Palette) *Renderer {
	r.palette = p
	return r
}

// ensureProgram lazily starts the tea.Program in a background
// goroutine. Idempotent; safe to call from every Render method.
func (r *Renderer) ensureProgram() {
	r.once.Do(func() {
		m := New(nil)
		r.program = tea.NewProgram(m, tea.WithAltScreen())
		r.progDone = make(chan struct{})
		go func() {
			defer close(r.progDone)
			_, _ = r.program.Run()
			r.mu.Lock()
			r.killed = true
			r.mu.Unlock()
		}()
	})
}

// send is a thin wrapper that suppresses message dispatch after the
// program has exited (user pressed q, or RenderDone closed it).
func (r *Renderer) send(msg tea.Msg) {
	r.mu.Lock()
	dead := r.killed
	r.mu.Unlock()
	if dead || r.program == nil {
		return
	}
	r.program.Send(msg)
}

// --- demokit.Renderer ---

// RenderHeader records the demo title/description for the model's
// top banner. Fires once at the start of the demo.
func (r *Renderer) RenderHeader(title, description string, stepCount int) {
	r.ensureProgram()
	r.stepCount = stepCount
	r.send(BridgeHeaderMsg{Title: title, Description: description, StepCount: stepCount})
}

// RenderStep builds the cell list for the visited step and ships it
// to the model. The OutputCell created here is what subsequent
// StreamOutput chunks feed into.
func (r *Renderer) RenderStep(stepNum, totalSteps int, step *demokit.StepDef) {
	r.ensureProgram()
	cells, buf, outputID := cellsForStep(stepNum, step, r.palette)
	r.mu.Lock()
	r.activeBuf = buf
	r.activeCellID = outputID
	r.mu.Unlock()
	r.send(BridgeStepCellsMsg{Cells: cells, OutputBuf: buf, OutputCellID: outputID})
}

// RenderResult marks the active OutputCell as done. If demokit
// invoked us via the non-streaming path (output passed as a string),
// we flush it into the buffer first so it shows up in the cell.
func (r *Renderer) RenderResult(stepNum int, output string, result *demokit.StepResult) {
	r.ensureProgram()
	r.mu.Lock()
	buf := r.activeBuf
	cellID := r.activeCellID
	r.mu.Unlock()
	if output != "" && buf != nil {
		buf.Append([]byte(output))
	}
	if cellID != "" {
		r.send(BridgeOutputDoneMsg{CellID: cellID})
	}
	if result != nil && result.Err != nil {
		// Surface the error as a trailing line in the output buffer
		// — it's the closest analog to PlainRenderer's "ERROR: ..."
		// suffix and keeps the user looking at the output cell.
		if buf != nil {
			buf.Append([]byte("\n[error] " + result.Err.Error() + "\n"))
		}
	}
}

// RenderSection appends a SectionCell to the current cell list.
// SectionDefs sit between steps in the demo declaration and don't
// trigger Run, so there's no associated OutputCell.
func (r *Renderer) RenderSection(section *demokit.SectionDef) {
	r.ensureProgram()
	id := fmt.Sprintf("section#%s", slugify(section.Title()))
	cell := NewSectionCell(id, section.Title(), section.Body())
	cell.SetPalette(r.palette)
	r.send(BridgeSectionCellMsg{Cell: cell})
}

// RenderDone tells the model to flip the header banner to a "Done."
// state, then blocks until the user presses q (or the program is
// already dead, in which case progDone is already closed).
func (r *Renderer) RenderDone() {
	r.ensureProgram()
	r.send(BridgeDoneMsg{})
	<-r.progDone
}

// WaitForStep blocks demokit's Execute loop until the user presses
// the advance key in the model. The model closes the channel on
// receipt of Space (or Shift+Enter); WaitForStep returns
// immediately afterward so the next step can run.
//
// If the program has already exited (user pressed q mid-demo),
// WaitForStep returns immediately so Execute can finish naturally.
func (r *Renderer) WaitForStep(opts demokit.WaitOpts) {
	r.mu.Lock()
	dead := r.killed
	r.mu.Unlock()
	if dead {
		return
	}
	r.ensureProgram()
	ch := make(chan struct{})
	r.send(BridgeWaitMsg{Ch: ch})
	select {
	case <-ch:
	case <-r.progDone:
	}
}

// Prompt appends a PromptCell to the current cell list and blocks
// until the user submits valid answers. The cell drives the
// textinput-based UI; the model closes Reply after sending the
// answer map.
//
// If the user quits the program (q / Ctrl+C) before submitting,
// Reply never receives and Prompt unblocks via the program-done
// channel, returning an empty map. demokit's Execute treats that
// as "no answers" — the step's Run sees an empty Inputs map.
func (r *Renderer) Prompt(stepID string, inputs []demokit.InputDef) map[string]any {
	r.ensureProgram()
	reply := make(chan map[string]any, 1)
	r.send(BridgePromptMsg{Inputs: promptInputsFrom(inputs), Reply: reply})
	select {
	case ans, ok := <-reply:
		if !ok || ans == nil {
			return map[string]any{}
		}
		return ans
	case <-r.progDone:
		return map[string]any{}
	}
}

// promptInputsFrom projects demokit.InputDef into the notebook's
// per-field shape, capturing the Parse closure so the PromptCell
// can validate each entry without depending on the demokit type.
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

// --- demokit.StreamingRenderer ---

// StreamOutput feeds each chunk into the active OutputBuffer. The
// model receives debounced OutputAppendedMsg events via the
// SubscribeOutputBuffer cmd installed on the current step.
//
// Writes never go to `out` (the user's real stdout) because Bubble
// Tea owns the screen — emitting bytes directly would corrupt the
// alt-screen rendering. demokit's interface gives us `out` for
// renderers that prefer to passthrough; the notebook is buffer-
// only.
func (r *Renderer) StreamOutput(stepNum int, chunk []byte, out io.Writer) {
	r.mu.Lock()
	buf := r.activeBuf
	r.mu.Unlock()
	if buf == nil {
		return
	}
	buf.Append(chunk)
}

// --- helpers ---

// cellsForStep builds the cell list for one step visit. Output is
// (cells, buffer, outputCellID); the renderer holds buffer +
// outputCellID for subsequent StreamOutput / RenderResult routing.
// The supplied palette is propagated to every cell so they all
// render against the same theme.
func cellsForStep(visit int, s *demokit.StepDef, palette Palette) ([]Cell, *OutputBuffer, string) {
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
	cells = append(cells, oc)
	return cells, buf, outputID
}

// buildMetaBody renders the step's note + arrows + refs into a
// single body string for the MetaCell. Order: note first (the
// human-readable summary), then sequence diagram arrows, then refs
// (RFC / CVE links).
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

// variantsFromView converts demokit's JSON-projection VariantView
// (what *StepDef.VerbatimBlocks() exposes) into the canonical
// Variant struct VerbatimCell consumes. The fields line up 1:1;
// this exists only to bridge the type names.
func variantsFromView(vs []demokit.VariantView) []demokit.Variant {
	out := make([]demokit.Variant, len(vs))
	for i, v := range vs {
		out[i] = demokit.Variant{Label: v.Label, Lang: v.Lang, Content: v.Content, IsDefault: v.IsDefault}
	}
	return out
}

// slugify lowercases and dash-normalizes a string so cell IDs stay
// terminal-friendly without spaces / special chars. Empty input
// returns empty so callers can fall back to a positional ID.
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
	out := b.String()
	out = strings.TrimRight(out, "-")
	return out
}
