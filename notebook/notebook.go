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
	keymap        KeyMap
	mouseConfig   MouseConfig

	ready   chan struct{}
	stopped chan struct{}

	promptSeq atomic.Uint64

	// dockFocusKey points at the positionKey of the currently
	// focused dock, or nil when focus is on the main list. Reads
	// from the BT goroutine (key dispatch) and writes from any
	// goroutine (FocusDock / ReleaseDockFocus) — atomic.Pointer
	// keeps it lock-free and easy to swap.
	dockFocusKey atomic.Pointer[positionKey]

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
	keymap        *KeyMap      // nil = DefaultKeyMap
	mouseConfig   *MouseConfig // nil = DefaultMouseConfig
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

// WithKeyMap overrides the notebook-level key bindings. Without
// it, DefaultKeyMap is used. Apps can start from the default and
// extend, or define a completely custom map.
func WithKeyMap(km KeyMap) Option {
	return func(o *notebookOpts) { o.keymap = &km }
}

// WithMouseConfig overrides the notebook-level mouse handlers.
// Without it, DefaultMouseConfig is used (left-click activates,
// wheel falls back to cell-cursor nav). See MouseConfig in
// mouse.go for the customization shape.
func WithMouseConfig(mc MouseConfig) Option {
	return func(o *notebookOpts) { o.mouseConfig = &mc }
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

	km := DefaultKeyMap()
	if cfg.keymap != nil {
		km = *cfg.keymap
	}

	mc := DefaultMouseConfig()
	if cfg.mouseConfig != nil {
		mc = *cfg.mouseConfig
	}

	st := newStore()
	ready := make(chan struct{})
	stopped := make(chan struct{})
	rdv := newRendezvous()

	progOpts := []tea.ProgramOption{}
	nb := &Notebook{
		store:         st,
		clip:          cfg.clip,
		rdv:           rdv,
		promptFactory: cfg.promptFactory,
		keymap:        km,
		mouseConfig:   mc,
		ready:         ready,
		stopped:       stopped,
	}
	m := newModel(nb, cfg.width, cfg.height)
	if cfg.headless {
		nb.blockIn = newBlockingReader()
		progOpts = append(progOpts,
			tea.WithInput(nb.blockIn),
			tea.WithOutput(io.Discard),
		)
	} else {
		progOpts = append(progOpts,
			tea.WithAltScreen(),
			tea.WithMouseCellMotion(),
			tea.WithInput(os.Stdin),
			tea.WithOutput(os.Stdout),
		)
	}
	nb.program = tea.NewProgram(m, progOpts...)
	// The Bottom dock has a built-in default — the StatusCell that
	// reproduces the legacy status row ("NAV  cell N/M"). Apps that
	// want vim-style command bars or richer chrome replace it via
	// SetDockedCell(Bottom, ...). Apps that want NO bottom row at
	// all call ClearDocked(Bottom).
	nb.store.setDock(Bottom.positionKey(), NewStatusCell(nb))
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

// SetMode requests a mode change. Safe to call from any
// goroutine — internally Sends a tea.Msg that the model applies.
func (nb *Notebook) SetMode(m Mode) {
	nb.program.Send(setModeMsg{mode: m})
}

// SetDockedCell installs c at pos, replacing any prior occupant.
// Docked cells live in a separate registry from the main cell
// list — they don't appear in nb.Get / IndexOf / Len and the main
// cursor never steps onto them. Use FocusDock(pos) to route keys
// to a docked cell.
//
// SetDockedCell auto-wires the configured clipboard into cells
// that implement SetClipboard, matching Append's behavior.
//
// Cell-anchored Positions (After / Before) automatically
// unregister when the anchor cell is removed via nb.Remove.
func (nb *Notebook) SetDockedCell(pos Position, c Cell) {
	nb.injectClipboard(c)
	nb.store.setDock(pos.positionKey(), c)
}

// ClearDocked removes the dock at pos. Returns true if a dock was
// present. Note: ClearDocked(Bottom) truly empties the slot — the
// default StatusCell is NOT auto-restored. Re-install it with
// SetDockedCell(Bottom, NewStatusCell(nb)) if you want it back.
//
// If the focused dock is being cleared, the dock-focus pointer
// drops back to nil (main list). The notebook's Mode is NOT
// changed — callers that need to drop from CellActiveMode emit
// notebook.ReleaseFocus from a cell or return notebook.ModeCmd
// from an action. (Sending here would deadlock when called from
// inside a keymap action; see safeSend's doc.)
func (nb *Notebook) ClearDocked(pos Position) bool {
	k := pos.positionKey()
	if cur := nb.dockFocusKey.Load(); cur != nil && *cur == k {
		nb.dockFocusKey.Store(nil)
	}
	return nb.store.clearDock(k)
}

// DockedCell returns the cell at pos, if any. The bool reports
// presence.
func (nb *Notebook) DockedCell(pos Position) (Cell, bool) {
	return nb.store.getDock(pos.positionKey())
}

// UpdateDocked replaces the dock at pos by fn(old). Returns false
// if no dock is present at pos. fn runs under the store lock — it
// must be quick and must not call back into the Notebook.
func (nb *Notebook) UpdateDocked(pos Position, fn func(Cell) Cell) bool {
	return nb.store.updateDock(pos.positionKey(), fn)
}

// FocusDock routes subsequent keystrokes to the docked cell at
// pos. Returns false if no dock is installed at pos. The main
// cursor stays where it is; ReleaseDockFocus / a ReleaseFocusMsg
// from the docked cell returns focus to the main list.
//
// FocusDock also flips the notebook into CellActiveMode so the
// docked cell behaves like a focused main cell: it sees every
// keystroke, KeyMap.Modes[NavigationMode] bindings (j/k/enter)
// don't fire underneath it.
// FocusDock points the notebook's dock-focus marker at pos so
// subsequent keystrokes route to that docked cell instead of the
// main cursor cell. Returns false if no dock is installed at pos.
//
// FocusDock does NOT change Mode — callers compose that:
//
//   - From a KeyMap Action: return notebook.ModeCmd(CellActiveMode)
//     alongside the FocusDock call. The action constructor
//     notebook.FocusDock(pos) bundles both.
//   - From a non-BT goroutine: call nb.SetMode(CellActiveMode)
//     after FocusDock.
//
// Splitting them this way avoids deadlock — Send-from-inside-an-
// action blocks because the BT goroutine is the same one that
// drains the channel.
func (nb *Notebook) FocusDock(pos Position) bool {
	k := pos.positionKey()
	if _, ok := nb.store.getDock(k); !ok {
		return false
	}
	nb.dockFocusKey.Store(&k)
	return true
}

// ReleaseDockFocus clears the dock-focus marker so subsequent
// keystrokes route to the main cursor cell. Does NOT change Mode
// — for the same deadlock-avoidance reasons as FocusDock. Idempotent.
func (nb *Notebook) ReleaseDockFocus() {
	nb.dockFocusKey.Store(nil)
}

// ModeCmd returns a tea.Cmd that switches the notebook into mode
// m. Use from KeyMap actions and convenience helpers that run on
// the BT goroutine — Sending directly would deadlock.
func ModeCmd(m Mode) tea.Cmd {
	return func() tea.Msg { return setModeMsg{mode: m} }
}

// focusedDockKey returns the currently focused dock's key and
// whether a dock is focused. Used by the model on the BT goroutine.
func (nb *Notebook) focusedDockKey() (positionKey, bool) {
	p := nb.dockFocusKey.Load()
	if p == nil {
		return positionKey{}, false
	}
	return *p, true
}

// safeSend Sends msg to the BT program loop iff it is up and
// draining. Mutations from caller goroutines that need to trigger
// a model-side state change (mode flip, focus shift) go through
// safeSend; the model's repaint tick picks up store changes
// without a Send.
//
// Without this guard, callers that exercise the public API before
// (or without) Run — unit tests, fast-fail paths — would deadlock
// on tea.Program.Send (which is unbuffered; see ARCHITECTURE.md
// § Concurrency model). With the program nil or its loop not yet
// live, safeSend silently drops; mode state doesn't matter
// without a running renderer anyway.
func (nb *Notebook) safeSend(msg tea.Msg) {
	if nb.program == nil {
		return
	}
	select {
	case <-nb.ready:
		nb.program.Send(msg)
	default:
	}
}

// Stop signals the program to quit and drains any pending
// AwaitInput callers with Source: "cancelled". Run returns
// shortly after. Idempotent.
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
