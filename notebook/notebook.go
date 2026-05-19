package notebook

import (
	"io"
	"os"
	"strconv"
	"sync"
	"sync/atomic"

	tea "github.com/charmbracelet/bubbletea"
)

// Notebook is the standalone cell-based TUI component. It owns a
// Bubble Tea program, a shared store of cells, and a Clipboard
// injected into appended cells that support one.
//
// Lifecycle: New → optional Append/Stream/etc. before Run → Run
// (blocks) → Stop. CRUD methods are safe to call from any
// goroutine; they mutate the store under its RWMutex. The model's
// repaint tick picks up store changes within one frame, so
// mutations don't need to Send (which would block before Run).
//
// Snapshot is a synchronous query into the running model — it
// waits for the program loop to be live (signalled via the ready
// channel) before issuing its Send.
type Notebook struct {
	store         *store
	program       *tea.Program
	clip          Clipboard
	rdv           *rendezvous
	promptFactory PromptFactory

	ready   chan struct{}
	stopped chan struct{}

	promptSeq atomic.Uint64

	startOnce sync.Once
	stopOnce  sync.Once
	blockIn   *blockingReader
}

// Option configures a Notebook at construction.
type Option func(*notebookOpts)

type notebookOpts struct {
	headless      bool
	width         int
	height        int
	clip          Clipboard
	promptFactory PromptFactory
}

// WithHeadless runs the Bubble Tea program against a blocking
// input reader and an io.Discard output. Use for tests: the
// program loop runs, processes Sends, and never quits on its own;
// Snapshot still returns rendered views.
func WithHeadless() Option {
	return func(o *notebookOpts) { o.headless = true }
}

// WithSize sets the initial viewport size — useful in headless
// tests where no WindowSizeMsg arrives. Real terminals get their
// size from BT's WindowSizeMsg and don't need this.
func WithSize(width, height int) Option {
	return func(o *notebookOpts) { o.width = width; o.height = height }
}

// WithClipboard injects the clipboard the notebook auto-wires
// into every appended cell that implements SetClipboard.
func WithClipboard(c Clipboard) Option {
	return func(o *notebookOpts) { o.clip = c }
}

// WithPromptFactory configures the function AwaitInput uses to
// build a prompt cell from a list of Inputs. Without one,
// AwaitInput panics — callers that don't supply a factory can
// still use AwaitInputBy with their own cells.
//
// The notebook package can't import its own cells subpackage, so
// real consumers wire this via cells.PromptFactory() (or a
// custom function).
func WithPromptFactory(f PromptFactory) Option {
	return func(o *notebookOpts) { o.promptFactory = f }
}

// New constructs a Notebook. The Bubble Tea program is wired up
// but not started; call Run to start it (Run blocks).
func New(opts ...Option) *Notebook {
	cfg := notebookOpts{clip: NoClipboard}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.clip == nil {
		cfg.clip = NoClipboard
	}

	st := newStore()
	ready := make(chan struct{})
	stopped := make(chan struct{})
	rdv := newRendezvous()
	m := newModel(st, rdv, ready, cfg.width, cfg.height)

	progOpts := []tea.ProgramOption{}
	nb := &Notebook{
		store:         st,
		clip:          cfg.clip,
		rdv:           rdv,
		promptFactory: cfg.promptFactory,
		ready:         ready,
		stopped:       stopped,
	}
	if cfg.headless {
		nb.blockIn = newBlockingReader()
		progOpts = append(progOpts,
			tea.WithInput(nb.blockIn),
			tea.WithOutput(io.Discard),
		)
	} else {
		progOpts = append(progOpts,
			tea.WithAltScreen(),
			tea.WithInput(os.Stdin),
			tea.WithOutput(os.Stdout),
		)
	}
	nb.program = tea.NewProgram(m, progOpts...)
	return nb
}

// Run starts the Bubble Tea program and blocks until the user
// quits or Stop is called. Safe to call exactly once per Notebook.
func (nb *Notebook) Run() error {
	var err error
	nb.startOnce.Do(func() {
		_, err = nb.program.Run()
	})
	return err
}

// Stop signals the program to quit and drains any pending
// AwaitAdvance / AwaitInput callers with Source: "cancelled".
// Run returns shortly after. Idempotent.
func (nb *Notebook) Stop() {
	nb.stopOnce.Do(func() {
		close(nb.stopped)
		nb.rdv.cancelAll()
		nb.program.Quit()
		if nb.blockIn != nil {
			nb.blockIn.close()
		}
	})
}

// nextPromptID returns a monotonically-increasing ID for prompt
// cells created by AwaitInput.
func (nb *Notebook) nextPromptID() CellID {
	return "prompt-" + strconv.FormatUint(nb.promptSeq.Add(1), 10)
}

// Snapshot returns the rendered View as it would appear right now.
// Synchronous: blocks until the program loop is up and produces a
// fresh frame (viewport recomputed before rendering). Intended
// for headless tests.
func (nb *Notebook) Snapshot() string {
	<-nb.ready
	reply := make(chan string, 1)
	nb.program.Send(snapshotMsg{reply: reply})
	return <-reply
}

// blockingReader is the input for headless mode: Read blocks
// until close is called, then returns io.EOF. Keeps BT's input
// goroutine parked without burning CPU.
type blockingReader struct {
	ch chan struct{}
}

func newBlockingReader() *blockingReader {
	return &blockingReader{ch: make(chan struct{})}
}

func (r *blockingReader) Read([]byte) (int, error) {
	<-r.ch
	return 0, io.EOF
}

func (r *blockingReader) close() {
	select {
	case <-r.ch:
	default:
		close(r.ch)
	}
}
