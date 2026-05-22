// Package notebookbridge wires demokit's event queue to the
// standalone notebook component. It implements demokit.Renderer
// (mostly as no-ops) + demokit.EventAwareRenderer; when demokit
// detects the EventAwareRenderer interface it skips the legacy
// renderer-driven path and just appends events. The bridge drains
// those events in a background goroutine and translates each one
// into the equivalent notebook.* call.
//
// Replaces the in-tree tui/notebook/ event-aware renderer.
package notebookbridge

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/panyam/demokit"
	"github.com/panyam/demokit/events"
	"github.com/panyam/demokit/notebook"
	"github.com/panyam/demokit/notebook/cells"
)

// Bridge wires the demokit event queue to a notebook.Notebook.
// One Bridge per Demo execution; safe to reuse the notebook
// instance across runs would require Reset support (deferred).
type Bridge struct {
	once  sync.Once
	queue *events.EventQueue
	nb    *notebook.Notebook

	nbDone chan struct{}

	visitMu        sync.Mutex
	outCellByVisit map[int]notebook.CellID

	// configurable knobs
	theme      *cells.Theme
	maxOutBody int
}

// defaultMaxOutputBody is the OutputCell row cap the bridge
// installs on every step's output cell. Set effectively-unlimited
// (1<<30 rows) so the bridge stays a data adapter — it routes
// stream chunks into a buffer, full stop, and the cell grows to
// fit. View concerns (in-cell scroll vs grow-with-data) live on
// the notebook side. Apps that explicitly want in-cell scroll
// for very long streams set a smaller cap via WithMaxOutputBody.
//
// Why this matters: a low cap (the old default of 12) silently
// turns on follow-mode tailing — early lines scroll off the top
// before the user can read them, making a long ASCII reveal look
// like it "appeared all at once at the end" when streaming was
// actually working. Removing the cap by default makes the
// streaming visible.
const defaultMaxOutputBody = 1 << 30

// New returns an unstarted Bridge. demokit.Execute discovers the
// event-aware path via the AttachEventQueue method.
func New() *Bridge {
	return &Bridge{}
}

// WithTheme overrides the cell-style theme used for newly
// appended cells. nil uses cells.DefaultTheme() (dark).
func (b *Bridge) WithTheme(t cells.Theme) *Bridge {
	b.theme = &t
	return b
}

// WithMaxOutputBody overrides the per-step OutputCell row cap.
// n <= 0 is treated as "use the default" (currently 100).
// Apps with steps that produce very large streams may want a
// smaller cap so in-cell scroll (j/k/wheel/PgUp/PgDn) becomes
// the natural way to look back; apps with finite reveals can
// leave the generous default.
func (b *Bridge) WithMaxOutputBody(n int) *Bridge {
	if n > 0 {
		b.maxOutBody = n
	}
	return b
}

// --- demokit.EventAwareRenderer ---

// AttachEventQueue stores the queue. demokit calls this once at
// the start of each run, before any Render* call.
func (b *Bridge) AttachEventQueue(q *events.EventQueue) { b.queue = q }

// --- demokit.Renderer (no-ops; events drive everything) ---

// RenderHeader is the first lifecycle hook; we use it as the
// trigger to start the notebook program lazily.
func (b *Bridge) RenderHeader(string, string, int) { b.ensureBridge() }

func (b *Bridge) RenderStep(int, int, *demokit.StepDef)         { b.ensureBridge() }
func (b *Bridge) RenderResult(int, string, *demokit.StepResult) {}
func (b *Bridge) RenderSection(*demokit.SectionDef)             {}
func (b *Bridge) WaitForStep(demokit.WaitOpts)                  {}
func (b *Bridge) Prompt(string, []demokit.InputDef) map[string]any {
	return nil
}
func (b *Bridge) StreamOutput(int, []byte, io.Writer) {}

// RenderDone blocks until the notebook program exits (user
// pressed Ctrl+C / q / etc.). Idempotent.
func (b *Bridge) RenderDone() {
	b.ensureBridge()
	<-b.nbDone
}

// ensureBridge spins up the notebook program + the event-drain
// goroutine the first time it's called.
func (b *Bridge) ensureBridge() {
	b.once.Do(func() {
		b.outCellByVisit = map[int]notebook.CellID{}

		nb := notebook.New(
			notebook.WithPromptFactory(cells.PromptFactory()),
			notebook.WithClipboard(notebook.OSC52Clipboard()),
		)
		b.nb = nb
		b.nbDone = make(chan struct{})

		// Drain the event queue in a goroutine. The drainer
		// blocks on sync events (WaitForAdvance, PromptOpen) so
		// it's serialised against demokit's RunLoop — by
		// construction (demokit's loop blocks on AwaitResolution
		// for those events too).
		go b.drainEvents()

		// Run the notebook in another goroutine; when it exits
		// (user quit), os.Exit so demokit's loop doesn't proceed
		// with empty inputs from a now-dead renderer.
		go func() {
			defer close(b.nbDone)
			_ = nb.Run()
			fmt.Print("\r\n")
			os.Exit(0)
		}()
	})
}

func (b *Bridge) drainEvents() {
	sub := b.queue.Subscribe()
	defer sub.Close()
	offset := 0
	for {
		select {
		case <-sub.Notify():
			evs, newOff := b.queue.ReadFrom(offset)
			for i, ev := range evs {
				b.handleEvent(offset+i, ev)
			}
			offset = newOff
		case <-b.nbDone:
			return
		}
	}
}

func (b *Bridge) handleEvent(off int, ev events.Event) {
	switch e := ev.(type) {
	case events.Header:
		b.nb.SetHeader(e.Title, e.Description)

	case events.Section:
		id := "section#" + slugify(e.Title)
		note := cells.NewNote(id, e.Title, e.Body)
		if b.theme != nil {
			note.Style = b.theme.Note
		}
		b.nb.Append(note)

	case events.StepStart:
		for _, c := range b.buildCellsFromStepStart(e) {
			b.nb.Append(c)
		}

	case events.StepReadyToRun:
		oid := outputID(e.Visit, "" /* StepID not on StepReadyToRun */)
		max := b.maxOutBody
		if max <= 0 {
			max = defaultMaxOutputBody
		}
		oc := cells.NewOutput(oid, max)
		if b.theme != nil {
			oc.Style = b.theme.Output
		}
		b.nb.Append(oc)
		b.visitMu.Lock()
		b.outCellByVisit[e.Visit] = oid
		b.visitMu.Unlock()

	case events.OutputChunk:
		if id, ok := b.outIDForVisit(e.Visit); ok {
			io.WriteString(b.nb.Stream(id), string(e.Chunk))
		}

	case events.StepEnd:
		if id, ok := b.outIDForVisit(e.Visit); ok {
			b.nb.Update(id, func(c notebook.Cell) notebook.Cell {
				if oc, ok := c.(*cells.OutputCell); ok {
					oc.MarkDone()
				}
				return c
			})
			if e.Status == "error" && e.ErrorText != "" {
				io.WriteString(b.nb.Stream(id), "\n[error] "+e.ErrorText+"\n")
			}
		}

	case events.WaitForAdvance:
		id := fmt.Sprintf("advance-%d", off)
		ac := cells.NewAdvance(id, "Press Enter to continue")
		if b.theme != nil {
			ac.Style = b.theme.Advance
		}
		ac.Deadline = e.Deadline // zero = no auto-advance
		b.nb.Append(ac)
		b.nb.FocusCell(id)
		resp := b.nb.AwaitInputBy(id)
		_ = b.queue.Resolve(off, &events.AdvanceResolution{
			Source: resp.Source, Timestamp: resp.At,
		})

	case events.PromptOpen:
		ins := convertInputs(e.Inputs)
		resp := b.nb.AwaitInput(ins)
		_ = b.queue.Resolve(off, &events.PromptResolution{
			Answers: resp.Answers, Source: resp.Source, Timestamp: resp.At,
		})

	case events.Done:
		b.nb.SetDone()
	}
}

func (b *Bridge) outIDForVisit(v int) (notebook.CellID, bool) {
	b.visitMu.Lock()
	defer b.visitMu.Unlock()
	id, ok := b.outCellByVisit[v]
	return id, ok
}

// buildCellsFromStepStart projects events.StepStart into the
// notebook's cell representation: HeaderCell (title + body
// joining note + arrows + refs) plus one VerbatimCell per
// declared verbatim block.
func (b *Bridge) buildCellsFromStepStart(e events.StepStart) []notebook.Cell {
	base := slugify(e.StepID)
	if base == "" {
		base = fmt.Sprintf("step%d", e.Visit)
	}
	body := buildHeaderBody(e)
	metaID := fmt.Sprintf("%s#%d.meta", base, e.Visit)
	header := cells.NewHeader(metaID, e.Title, body)
	if b.theme != nil {
		header.Style = b.theme.Header
	}
	out := []notebook.Cell{header}
	for i, vb := range e.Verbatims {
		vid := fmt.Sprintf("%s#%d.verbatim%d", base, e.Visit, i)
		variants := make([]cells.Variant, len(vb.Variants))
		for j, v := range vb.Variants {
			variants[j] = cells.Variant{
				Label: v.Label, Lang: v.Lang, Content: v.Content, IsDefault: v.IsDefault,
			}
		}
		vc := cells.NewVerbatim(vid, vb.Label, variants)
		if b.theme != nil {
			vc.Style = b.theme.Verbatim
		}
		out = append(out, vc)
	}
	return out
}

// buildHeaderBody joins a step's note + arrows + refs into the
// HeaderCell body.
func buildHeaderBody(e events.StepStart) string {
	var parts []string
	if note := strings.TrimSpace(e.Note); note != "" {
		parts = append(parts, note)
	}
	if len(e.Arrows) > 0 {
		var lines []string
		for _, a := range e.Arrows {
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
	if len(e.Refs) > 0 {
		var lines []string
		for _, ref := range e.Refs {
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

// convertInputs translates the public events.Input values into
// notebook.Input. The closed-set type switch is the only place
// the bridge knows the concrete event input shapes; the notebook
// only sees its own Input interface.
func convertInputs(in []events.Input) []notebook.Input {
	out := make([]notebook.Input, len(in))
	for i, e := range in {
		switch v := e.(type) {
		case events.IntInput:
			out[i] = notebook.NewIntInput(v.InputName(), v.InputPrompt(), v.InputDefault())
		case events.ChoiceInput:
			out[i] = notebook.NewChoiceInput(v.InputName(), v.InputPrompt(), v.InputDefault(), v.Options)
		default:
			out[i] = notebook.NewStringInput(e.InputName(), e.InputPrompt(), e.InputDefault())
		}
	}
	return out
}

// outputID returns the canonical OutputCell ID for a step visit.
func outputID(visit int, stepID string) string {
	base := slugify(stepID)
	if base == "" {
		base = fmt.Sprintf("step%d", visit)
	}
	return fmt.Sprintf("%s#%d.output", base, visit)
}

// slugify normalizes step IDs into stable cell-id prefixes
// (lowercase, alphanumeric + dashes only).
func slugify(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	lastDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteRune('-')
				lastDash = true
			}
		}
	}
	out := b.String()
	return strings.TrimRight(out, "-")
}
