package web

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/panyam/demokit"
	gohttp "github.com/panyam/servicekit/http"
	skmiddleware "github.com/panyam/servicekit/middleware"
	"github.com/panyam/templar"
)

// ServeHTTP runs the demo as an HTTP+WS server on addr. The server
// exposes a `<demokit-demo>` embed page at /, the player + ansi_up
// assets, and a WebSocket at /events that streams structured demo
// events to connected browsers and accepts input/reset commands
// back. The demo's Run/Coalesce/Parse closures continue to live in
// Go; the browser is just a structured-form UI on top.
//
// Blocks until the server is signalled to stop (SIGINT/SIGTERM).
func ServeHTTP(d *demokit.Demo, addr string) error {
	srv, handler := newLiveServer(d)
	server := &http.Server{Addr: addr, Handler: handler}
	skmiddleware.ApplyDefaults(server)

	log.Printf("demokit: serving %q at http://%s/", d.Title(), addr)
	err := gohttp.ListenAndServeGraceful(server,
		gohttp.WithDrainTimeout(5*time.Second),
		gohttp.WithOnShutdown(srv.shutdown),
	)
	if err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("serve %s: %w", addr, err)
	}
	return nil
}

// newLiveServer wires up a liveServer, registers HTTP routes, swaps
// the demo's renderer, and launches the demo run goroutine. Returns
// the server (for shutdown) and the http.Handler ready to be wrapped
// in any listener — production code uses an http.Server bound to
// addr; tests use httptest.NewServer.
func newLiveServer(d *demokit.Demo) (*liveServer, http.Handler) {
	srv := &liveServer{
		demo:    d,
		hub:     newWSHub(),
		inputs:  make(chan map[string]any, 1),
		history: nil,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", srv.handleIndex)
	mux.HandleFunc("/demokit-player.js", srv.handlePlayerJS)
	mux.HandleFunc("/demokit-player.css", srv.handlePlayerCSS)
	mux.HandleFunc("/ansi_up.js", srv.handleAnsiUp)
	mux.HandleFunc("/trace.json", srv.handleTrace)
	mux.Handle("/events", gohttp.WSServe[any, *liveConn](&liveWSHandler{srv: srv}, gohttp.DefaultWSConnConfig()))

	srv.registerRenderer()

	srv.runCtx, srv.runCancel = context.WithCancel(context.Background())
	srv.runDone = make(chan struct{})
	go srv.runDemo()

	return srv, corsMiddleware(mux)
}

// shutdown cancels the running demo and force-closes every WS
// connection so gorilla's blocking ReadMessage returns and
// http.Server.Shutdown can drain. Idempotent — safe to call
// multiple times (cancel of an already-cancelled context is a
// no-op; closing already-closed sockets is harmless).
func (s *liveServer) shutdown() {
	s.runCancel()
	s.hub.closeAll()
}

// --- liveServer ---

type liveServer struct {
	demo *demokit.Demo
	hub  *wsHub

	mu      sync.Mutex
	history []serverEvent // replayed to late-joining clients

	// runMu guards the run-lifecycle fields below. Acquired by
	// reset() to coordinate cancellation, drain, and re-launch
	// without racing concurrent reset/shutdown calls.
	runMu     sync.Mutex
	inputs    chan map[string]any
	runCtx    context.Context
	runCancel context.CancelFunc
	runDone   chan struct{} // closed when the current runDemo goroutine exits
}

func (s *liveServer) registerRenderer() {
	// Tee onto whatever renderer the caller already configured (--tui,
	// a custom one, or the PlainRenderer default) so the operator's
	// terminal sees the same demo the browser does, in their chosen
	// style. Only the framing methods delegate; Prompt / WaitForStep
	// stay WS-only because the inner renderer would otherwise read
	// the operator's stdin.
	inner := s.demo.Renderer()
	if inner == nil {
		inner = &demokit.PlainRenderer{}
	}
	s.demo.WithRenderer(&webRenderer{srv: s, inner: inner})
}

func (s *liveServer) runDemo() {
	defer close(s.runDone)
	defer func() {
		if r := recover(); r != nil {
			log.Printf("demokit: demo run panic: %v", r)
		}
	}()
	// RunLoop instead of Execute so we skip the --serve / --doc
	// flag-dispatch (which is what got us here in the first place
	// and would otherwise recurse).
	s.demo.RunLoop()
	s.broadcast(serverEvent{Kind: "done"})
}

// reset cancels the current run, waits for the goroutine to exit,
// clears history, and re-launches a fresh run from the top. Called
// when a client sends {"kind":"reset"} over WS. Safe to call
// concurrently — runMu serializes the lifecycle transition.
//
// If the current Run is in a long-running operation that ignores
// ctx.Ctx.Done(), reset blocks until it returns naturally — same
// contract as Timeout/Cancellable from issue #5.
func (s *liveServer) reset() {
	s.runMu.Lock()
	defer s.runMu.Unlock()

	// Cancel the current run; any pending Prompt unblocks via its
	// runCtx select.
	s.runCancel()
	<-s.runDone

	// Drain any input value that arrived between the user's reset
	// click and the cancel firing — otherwise it would feed into
	// the next run's first Prompt.
	select {
	case <-s.inputs:
	default:
	}

	// Tell every client to clear its feed, then drop the history
	// buffer so late-joiners that connect *after* reset don't get
	// stale state.
	s.hub.broadcast(serverEvent{Kind: "reset"})
	s.mu.Lock()
	s.history = nil
	s.mu.Unlock()

	// Fresh context + done signal for the next run.
	s.runCtx, s.runCancel = context.WithCancel(context.Background())
	s.runDone = make(chan struct{})
	go s.runDemo()
}

// broadcast pushes an event to every connected WS client AND
// records it in the history buffer so late-joining clients can
// replay state.
func (s *liveServer) broadcast(evt serverEvent) {
	s.mu.Lock()
	s.history = append(s.history, evt)
	s.mu.Unlock()
	s.hub.broadcast(evt)
}

func (s *liveServer) snapshotHistory() []serverEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]serverEvent, len(s.history))
	copy(cp, s.history)
	return cp
}

// --- HTTP handlers ---

func (s *liveServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	html, err := renderLiveHTML(s.demo)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(html))
}

func (s *liveServer) handlePlayerJS(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	_, _ = w.Write([]byte(playerJS))
}

func (s *liveServer) handlePlayerCSS(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	_, _ = w.Write([]byte(playerCSS))
}

func (s *liveServer) handleAnsiUp(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	_, _ = w.Write([]byte(ansiUpJS))
}

func (s *liveServer) handleTrace(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write([]byte(s.traceJSON()))
}

func (s *liveServer) traceJSON() string {
	entries := []demokit.TraceEntry{}
	for _, e := range s.snapshotHistory() {
		if e.Entry != nil {
			entries = append(entries, *e.Entry)
		}
	}
	return demokit.RenderDocumentJSON(demokit.RenderContext{Demo: s.demo, Trace: entries})
}

// --- WebSocket layer ---

// serverEvent is the shape sent on the /events WebSocket.
type serverEvent struct {
	Kind   string              `json:"kind"`
	Entry  *demokit.TraceEntry `json:"entry,omitempty"`
	Demo   any                 `json:"demo,omitempty"`
	Inputs []demokit.InputDef  `json:"inputs,omitempty"`
	StepID string              `json:"step_id,omitempty"`
	Chunk  string              `json:"chunk,omitempty"`
	Status int                 `json:"status,omitempty"`
	Error  string              `json:"error,omitempty"`
	Extra  map[string]any      `json:"extra,omitempty"`
}

type liveWSHandler struct {
	srv *liveServer
}

func (h *liveWSHandler) Validate(w http.ResponseWriter, r *http.Request) (*liveConn, bool) {
	c := &liveConn{
		JSONConn: gohttp.NewJSONConn(),
		srv:      h.srv,
	}
	c.NameStr = "demokit-live"
	c.ConnIdStr = newConnID()
	return c, true
}

type liveConn struct {
	*gohttp.JSONConn
	srv *liveServer
	ws  *websocket.Conn // captured in OnStart so closeAll can force-close on shutdown
}

// forceClose closes the underlying *websocket.Conn so the WSHandleConn
// reader loop's blocking ReadMessage returns with an error and the
// handler unwinds. Without this, Ctrl-C waits the full drain timeout
// because servicekit's BaseConn.OnClose only stops the Writer goroutine.
func (c *liveConn) forceClose() {
	if c.ws != nil {
		_ = c.ws.Close()
	}
}

// HandleMessage decodes incoming client events and routes them.
// Override of BaseConn's default (which just logs).
func (c *liveConn) HandleMessage(msg any) error {
	raw, ok := msg.(map[string]any)
	if !ok {
		return nil
	}
	kind, _ := raw["kind"].(string)
	switch kind {
	case "input":
		values, _ := raw["values"].(map[string]any)
		select {
		case c.srv.inputs <- values:
		default:
			log.Printf("demokit: input received but no step waiting (dropped: %v)", values)
		}
	case "reset":
		// Run reset in its own goroutine — it blocks on runDone, and
		// HandleMessage is called from the WS reader loop which we
		// don't want to stall (the next message we receive may be
		// another reset, or an input from a different client).
		go c.srv.reset()
	}
	return nil
}

// OnStart replays history to the new client so late joiners see
// past state, then registers with the hub for future broadcasts.
func (c *liveConn) OnStart(conn *websocket.Conn) error {
	c.ws = conn
	if err := c.JSONConn.OnStart(conn); err != nil {
		return err
	}
	for _, e := range c.srv.snapshotHistory() {
		c.JSONConn.SendOutput(e)
	}
	c.srv.hub.register(c)
	return nil
}

func (c *liveConn) OnClose() {
	c.srv.hub.unregister(c.ConnId())
	c.JSONConn.OnClose()
}

// --- wsHub ---

type wsHub struct {
	mu    sync.RWMutex
	conns map[string]*liveConn
}

func newWSHub() *wsHub {
	return &wsHub{conns: make(map[string]*liveConn)}
}

func (h *wsHub) register(c *liveConn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.conns[c.ConnId()] = c
}

func (h *wsHub) unregister(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.conns, id)
}

func (h *wsHub) broadcast(evt serverEvent) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, c := range h.conns {
		c.SendOutput(evt)
	}
}

func (h *wsHub) closeAll() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, c := range h.conns {
		c.forceClose()
	}
	h.conns = make(map[string]*liveConn)
}

// --- webRenderer ---

// Compile-time assertion that webRenderer satisfies StreamingRenderer
// — without it, demokit silently falls back to the buffered path.
var _ demokit.StreamingRenderer = (*webRenderer)(nil)

type webRenderer struct {
	srv      *liveServer
	inner    demokit.Renderer // mirrors the full demo to the operator's terminal — whatever the user picked (PlainRenderer, tui.Renderer, ...)
	stepOpen bool             // true between RenderStep and RenderResult
}

func (r *webRenderer) RenderHeader(title, description string, stepCount int) {
	r.inner.RenderHeader(title, description, stepCount)
	r.srv.broadcast(serverEvent{
		Kind: "header",
		Demo: map[string]any{
			"title":       title,
			"description": description,
			"step_count":  stepCount,
		},
	})
}

func (r *webRenderer) RenderStep(stepNum, totalSteps int, step *demokit.StepDef) {
	r.inner.RenderStep(stepNum, totalSteps, step)
	r.stepOpen = true
	r.srv.broadcast(serverEvent{
		Kind: "step-start",
		Extra: map[string]any{
			"step_num":    stepNum,
			"total_steps": totalSteps,
			"id":          step.StepID(),
			"title":       step.Title(),
			"note":        step.NoteText(),
			"refs":        step.Refs(),
			"arrows":      step.Arrows(),
			"verbatim":    step.VerbatimBlocks(),
		},
	})
}

func (r *webRenderer) RenderResult(stepNum int, output string, result *demokit.StepResult) {
	r.inner.RenderResult(stepNum, output, result)
	// MaxSteps / unknown-Next aborts call RenderResult without a
	// preceding RenderStep. Synthesize an open step so the player has
	// something to mark as errored — otherwise the abort is silent
	// and the prior successful "→ jumped to X" looks like the end.
	if !r.stepOpen {
		r.srv.broadcast(serverEvent{
			Kind: "step-start",
			Extra: map[string]any{
				"step_num": stepNum,
				"id":       "__demokit_aborted__",
				"title":    "Aborted",
			},
		})
		r.stepOpen = true
	}
	evt := serverEvent{Kind: "step-end"}
	if result != nil {
		evt.Status = int(result.Status)
		evt.Extra = map[string]any{
			"label":   result.DisplayLabel(),
			"message": result.Message,
			"next":    result.Next,
			"output":  output,
		}
	}
	r.srv.broadcast(evt)
	r.stepOpen = false
}

func (r *webRenderer) RenderSection(section *demokit.SectionDef) {
	r.inner.RenderSection(section)
	r.srv.broadcast(serverEvent{
		Kind: "section",
		Extra: map[string]any{
			"title": section.Title(),
			"body":  section.Body(),
		},
	})
}

func (r *webRenderer) RenderDone() {
	r.inner.RenderDone()
	// runDemo emits the final "done" after Execute returns; that
	// covers the broadcast side, so no broadcast here.
}

func (r *webRenderer) WaitForStep(opts demokit.WaitOpts) {
	// Live mode auto-advances; no terminal pause.
}

// Prompt collects inputs for the current step. Broadcasts an
// "input-needed" event with the declared input shapes, then blocks
// reading from the server's inputs channel (filled by WS "input"
// messages). Selects on runCtx so shutdown unblocks the demo
// goroutine instead of leaking it. If Demo.EffectiveInputTimeout
// is non-zero for this step, falls back to declared defaults
// after the deadline and broadcasts "input-timeout" so the
// player can dismiss its form.
func (r *webRenderer) Prompt(stepID string, inputs []demokit.InputDef) map[string]any {
	r.srv.broadcast(serverEvent{
		Kind:   "input-needed",
		StepID: stepID,
		Inputs: inputs,
	})

	timeout := r.srv.demo.EffectiveInputTimeout(stepID)
	var deadline <-chan time.Time
	if timeout > 0 {
		t := time.NewTimer(timeout)
		defer t.Stop()
		deadline = t.C
	}

	select {
	case values, ok := <-r.srv.inputs:
		if !ok || values == nil {
			return map[string]any{}
		}
		return values
	case <-r.srv.runCtx.Done():
		return map[string]any{}
	case <-deadline:
		defaults := defaultsForInputs(inputs)
		r.srv.broadcast(serverEvent{
			Kind:   "input-timeout",
			StepID: stepID,
			Extra:  map[string]any{"timeout_ms": timeout.Milliseconds()},
		})
		return defaults
	}
}

// defaultsForInputs builds the same map collectInputs would build
// in non-interactive mode — any input with a Default contributes
// to the result; inputs without a Default are absent. Used as the
// fallback when an input prompt times out.
func defaultsForInputs(inputs []demokit.InputDef) map[string]any {
	out := make(map[string]any, len(inputs))
	for _, in := range inputs {
		if in.Default != nil {
			out[in.Name] = in.Default
		}
	}
	return out
}

// StreamOutput broadcasts a chunk event so the live page can render
// streaming step output in real time, and mirrors the chunk to the
// operator's terminal in the inner renderer's preferred style (TUI
// users get TUI-styled chunks; plain users get raw bytes). out is
// the stdout snapshotted before captureOutput's redirect — writing
// to os.Stdout directly would loop back into the capture pipe.
func (r *webRenderer) StreamOutput(stepNum int, chunk []byte, out io.Writer) {
	if sr, ok := r.inner.(demokit.StreamingRenderer); ok {
		sr.StreamOutput(stepNum, chunk, out)
	} else if out != nil {
		_, _ = out.Write(chunk)
	}
	r.srv.broadcast(serverEvent{
		Kind:  "chunk",
		Chunk: string(chunk),
		Extra: map[string]any{"step_num": stepNum},
	})
}

// --- helpers ---

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// newConnID returns a short opaque id for a WS connection. Not
// security-sensitive — just for hub bookkeeping.
func newConnID() string {
	connIDMu.Lock()
	defer connIDMu.Unlock()
	connIDCounter++
	return fmt.Sprintf("c%d", connIDCounter)
}

var (
	connIDMu      sync.Mutex
	connIDCounter int64
)

// renderLiveHTML composes the embed-ready HTML for GET /. Uses the
// templar pattern (matches bundle.html) — html/template's auto-
// escape handles the title field; no hand-rolled escape needed.
func renderLiveHTML(d *demokit.Demo) (string, error) {
	title := "Demo"
	if d != nil && d.Title() != "" {
		title = d.Title()
	}
	group := templar.NewTemplateGroup()
	group.Loader = templar.NewFileSystemLoader(templar.FSFolder{FS: tmplFS, Path: "templates"})
	tmpls, err := group.Loader.Load("live.html", "")
	if err != nil {
		return "", fmt.Errorf("load live.html: %w", err)
	}
	if len(tmpls) == 0 {
		return "", fmt.Errorf("live.html template not found")
	}
	var buf bytes.Buffer
	data := struct{ Title string }{Title: title}
	if err := group.RenderHtmlTemplate(&buf, tmpls[0], "", data, nil); err != nil {
		return "", fmt.Errorf("render live.html: %w", err)
	}
	return buf.String(), nil
}
