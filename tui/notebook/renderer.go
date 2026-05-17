package notebook

import (
	"fmt"
	"io"
	"os"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/term"

	"github.com/panyam/demokit"
	"github.com/panyam/demokit/events"
)

// Renderer is the notebook's event-aware Renderer. It implements
// demokit.Renderer (as no-ops) + demokit.EventAwareRenderer, so
// demokit.Execute attaches the public event queue here. The
// renderer starts the Bubble Tea program lazily on the first
// Render call; the model drains the queue and projects events
// into cells.
//
// Sync rendezvous (Prompt / WaitForAdvance) is handled entirely
// inside the model + cells via queue.Resolve — the Renderer
// interface's WaitForStep / Prompt are unused for the notebook
// path (demokit's event-aware detection skips them).
type Renderer struct {
	once     sync.Once
	program  *tea.Program
	progDone chan struct{}

	mu sync.Mutex

	queue   *events.EventQueue
	palette Palette
	killed  bool
}

// NewRenderer constructs a fresh notebook renderer.
func NewRenderer() *Renderer {
	return &Renderer{palette: DefaultPalette()}
}

// WithPalette overrides the palette used to construct cells.
func (r *Renderer) WithPalette(p Palette) *Renderer {
	r.palette = p
	return r
}

// AttachEventQueue implements demokit.EventAwareRenderer.
// demokit.Execute calls this once at the start of each run.
func (r *Renderer) AttachEventQueue(q *events.EventQueue) {
	r.queue = q
}

// ensureProgram lazily starts the Bubble Tea program. The model
// receives the queue + palette; events drive every state
// change.
func (r *Renderer) ensureProgram() {
	r.once.Do(func() {
		var origTermState *term.State
		if fd := os.Stdin.Fd(); term.IsTerminal(fd) {
			origTermState, _ = term.GetState(fd)
		}

		q := r.queue
		if q == nil {
			// Defensive: AttachEventQueue should have been called
			// before the first Render*. If not, create a private
			// queue so the program still runs (empty).
			q = events.NewQueue()
		}
		m := New(nil).WithQueue(q).WithPalette(r.palette)
		r.program = tea.NewProgram(m, tea.WithAltScreen())
		r.progDone = make(chan struct{})
		go func() {
			defer close(r.progDone)
			_, _ = r.program.Run()
			if origTermState != nil {
				_ = term.Restore(os.Stdin.Fd(), origTermState)
			}
			fmt.Print("\r\n")
			r.mu.Lock()
			r.killed = true
			r.mu.Unlock()
			// Cut demokit's Execute loop short if it's still
			// running; otherwise it would proceed with empty
			// Inputs maps from skipped Prompt calls.
			os.Exit(0)
		}()
	})
}

// --- demokit.Renderer (no-ops; the queue does the work) ---

// RenderHeader triggers BT startup. The event is already in the
// queue when demokit.Execute calls us; the model has it.
func (r *Renderer) RenderHeader(string, string, int) { r.ensureProgram() }

// RenderStep ensures BT is up; event arrived via the queue.
func (r *Renderer) RenderStep(int, int, *demokit.StepDef) { r.ensureProgram() }

// RenderResult: no-op for event-aware renderers. demokit skips
// the legacy output-display path for us (displayOutput == "").
func (r *Renderer) RenderResult(int, string, *demokit.StepResult) {}

// RenderSection: no-op; event arrived via the queue.
func (r *Renderer) RenderSection(*demokit.SectionDef) {}

// RenderDone emits the Done event (already done by demokit) and
// blocks until the user quits via q / Ctrl+C / Enter-on-done.
func (r *Renderer) RenderDone() {
	r.ensureProgram()
	<-r.progDone
}

// WaitForStep: demokit's event-aware detection skips this. Kept
// to satisfy the Renderer interface.
func (r *Renderer) WaitForStep(demokit.WaitOpts) {}

// Prompt: demokit's event-aware detection skips this. Returns
// nil (demokit handles the empty-map case).
func (r *Renderer) Prompt(string, []demokit.InputDef) map[string]any { return nil }

// StreamOutput: demokit emits OutputChunk events directly to
// the queue, but if the renderer also satisfies
// StreamingRenderer demokit will still call this. Keep it
// no-op so we don't double-process.
func (r *Renderer) StreamOutput(int, []byte, io.Writer) {}
