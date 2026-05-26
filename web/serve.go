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
	"github.com/panyam/demokit/events"
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
		demo:          d,
		hub:           newWSHub(),
		inputs:        make(chan map[string]any, 1),
		history:       nil,
		outBuffers:    map[int][]byte{},
		stepIDByVisit: map[int]string{},
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

// shutdown cancels the running demo, force-closes every WS
// connection so gorilla's blocking ReadMessage returns, AND waits
// for runDemo to exit so the drain goroutine + any in-flight
// captureOutput is sequenced before the caller returns. Idempotent
// — safe to call multiple times.
//
// Waiting on runDone here matters under -race: tests that just
// `defer srv.shutdown()` previously let runDemo leak into the next
// test, where its captureOutput's os.Stdout mutation races with
// the next test's stdout snapshot (issue 23's same class).
func (s *liveServer) shutdown() {
	s.runCancel()
	s.hub.closeAll()
	<-s.runDone
}

// --- liveServer ---

// liveServer doubles as the demokit Renderer for --serve mode:
// implements EventAwareRenderer (drains the demo's event queue,
// broadcasts WS frames) and FinishableRenderer (waits for the
// drain on Done). The legacy Render* methods are no-ops because
// demokit gates them for event-aware renderers; they only exist
// to satisfy the interface. This is the notebook-style decomposition
// — the browser player is the view, liveServer is the bridge,
// nothing wraps an inner local renderer (see PR thread).
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

	// --- event-aware path ---
	queue         *events.EventQueue
	drainDone     bool
	drainWG       sync.WaitGroup
	outBuffers    map[int][]byte // per-visit accumulated chunks for the step-end frame's output field
	stepIDByVisit map[int]string // visit -> step id captured from StepStart; lets PromptOpen tag the input-needed frame and lets StepEnd detect missing-StepStart abort paths (synthesise a step-start)
}

// registerRenderer wires liveServer as the demo's renderer.
// Notebook-style: the browser player is the view; liveServer drains
// the event queue and broadcasts. No inner local-terminal renderer
// is composed — for a local view, run the demo without --serve.
func (s *liveServer) registerRenderer() {
	s.demo.WithRenderer(s)
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

// --- liveServer as demokit.Renderer / EventAwareRenderer / FinishableRenderer ---

// Compile-time assertions: liveServer is the renderer demokit calls
// (every method on the legacy Renderer/StreamingRenderer surface),
// AND the event-aware drain that does the real work; demokit gates
// the legacy methods for event-aware renderers (Phase 3a) so the
// no-ops below only exist to satisfy the interface.
var (
	_ demokit.Renderer           = (*liveServer)(nil)
	_ demokit.StreamingRenderer  = (*liveServer)(nil)
	_ demokit.EventAwareRenderer = (*liveServer)(nil)
	_ demokit.FinishableRenderer = (*liveServer)(nil)
)

// --- Renderer interface stubs (never called when event-aware) ---

func (s *liveServer) RenderHeader(string, string, int)                 {}
func (s *liveServer) RenderStep(int, int, *demokit.StepDef)            {}
func (s *liveServer) RenderResult(int, string, *demokit.StepResult)    {}
func (s *liveServer) RenderSection(*demokit.SectionDef)                {}
func (s *liveServer) RenderDone()                                      {}
func (s *liveServer) WaitForStep(demokit.WaitOpts)                     {}
func (s *liveServer) Prompt(string, []demokit.InputDef) map[string]any { return nil }
func (s *liveServer) StreamOutput(int, []byte, io.Writer)              {}

// --- EventAwareRenderer + FinishableRenderer ---

// AttachEventQueue wires the demo's event queue and spawns the drain.
// Called once per RunLoop. reset() re-launches runDemo with a fresh
// AttachEventQueue, so prior-run state (drainDone, buffers) resets
// here too.
func (s *liveServer) AttachEventQueue(q *events.EventQueue) {
	s.queue = q
	s.drainDone = false
	s.outBuffers = map[int][]byte{}
	s.stepIDByVisit = map[int]string{}
	s.drainWG.Add(1)
	go func() {
		defer s.drainWG.Done()
		s.drainEvents()
	}()
}

// Finish blocks until the drain goroutine exits — same race-detector
// hygiene as PlainRenderer's Finish (Phase 3a). Demokit calls this
// after emitting Done.
func (s *liveServer) Finish() {
	s.drainWG.Wait()
}

// drainEvents subscribes + catch-up drains (same lesson as the
// notebookbridge: events appended before subscribe are missed if
// you only block on Notify) + loops until Done sets drainDone.
func (s *liveServer) drainEvents() {
	sub := s.queue.Subscribe()
	defer sub.Close()
	offset := s.drainFrom(0)
	for !s.drainDone {
		<-sub.Notify()
		offset = s.drainFrom(offset)
	}
}

func (s *liveServer) drainFrom(offset int) int {
	evs, newOff := s.queue.ReadFrom(offset)
	for i, ev := range evs {
		s.handleEvent(offset+i, ev)
	}
	return newOff
}

// handleEvent translates one demokit event into a WS frame (or
// blocks for sync events). Mirrors what the pre-3b webRenderer did
// in its Render*/Prompt/StreamOutput methods, but driven by the
// queue rather than by Execute's method calls.
func (s *liveServer) handleEvent(off int, ev events.Event) {
	switch e := ev.(type) {
	case events.Header:
		s.broadcast(serverEvent{
			Kind: "header",
			Demo: map[string]any{
				"title":       e.Title,
				"description": e.Description,
				"step_count":  e.StepCount,
			},
		})
	case events.Section:
		s.broadcast(serverEvent{
			Kind: "section",
			Extra: map[string]any{
				"title": e.Title,
				"body":  e.Body,
			},
		})
	case events.StepStart:
		s.stepIDByVisit[e.Visit] = e.StepID
		s.broadcast(serverEvent{
			Kind: "step-start",
			Extra: map[string]any{
				"step_num":    e.Visit,
				"total_steps": e.Declared,
				"id":          e.StepID,
				"title":       e.Title,
				"note":        e.Note,
				"refs":        e.Refs,
				"arrows":      e.Arrows,
				"verbatim":    e.Verbatims,
			},
		})
	case events.OutputChunk:
		s.outBuffers[e.Visit] = append(s.outBuffers[e.Visit], e.Chunk...)
		s.broadcast(serverEvent{
			Kind:  "chunk",
			Chunk: string(e.Chunk),
			Extra: map[string]any{"step_num": e.Visit},
		})
	case events.StepEnd:
		// max-visits / max-steps aborts emit StepEnd for a visit
		// that never had a StepStart (Phase 3a's error-path emits).
		// Synthesise a step-start so the player has something to
		// mark as errored — same intent as the pre-3b webRenderer's
		// !stepOpen branch.
		if _, seen := s.stepIDByVisit[e.Visit]; !seen {
			abortedID := "__demokit_aborted__"
			s.broadcast(serverEvent{
				Kind: "step-start",
				Extra: map[string]any{
					"step_num": e.Visit,
					"id":       abortedID,
					"title":    "Aborted",
				},
			})
			s.stepIDByVisit[e.Visit] = abortedID
		}
		evt := serverEvent{Kind: "step-end"}
		out := string(s.outBuffers[e.Visit])
		delete(s.outBuffers, e.Visit)
		// Preserve the legacy frame shape: status/label/message/next
		// only when there's a non-trivial result. stepEndEvent on
		// demokit emits Status="ok" with empty Message/ErrorText for
		// successes, so mirror that into the int Status the JS player
		// expects.
		if res := demokit.StepResultFromEvent(e); res != nil {
			evt.Status = int(res.Status)
			evt.Extra = map[string]any{
				"label":   res.DisplayLabel(),
				"message": res.Message,
				"next":    "", // not in StepEnd event; player handles absence
				"output":  out,
			}
		} else if out != "" {
			evt.Extra = map[string]any{"output": out}
		}
		s.broadcast(evt)
	case events.Done:
		// runDemo emits the final "done" after RunLoop returns;
		// nothing extra to broadcast here. Setting drainDone exits
		// the drain loop so Finish() can return.
		s.drainDone = true
	case events.WaitForAdvance:
		// --serve doesn't set stdinAttached, so demokit shouldn't
		// emit WaitForAdvance in this mode. Defensive auto-Resolve
		// in case the invariant changes.
		_ = s.queue.Resolve(off, &events.AdvanceResolution{
			Source: "web-auto", Timestamp: time.Now(),
		})
	case events.PromptOpen:
		// Already resolved (e.g. non-interactive defaults) — skip.
		if _, resolved := s.queue.Resolution(off); resolved {
			return
		}
		stepID := s.stepIDByVisit[e.Visit]
		s.broadcast(serverEvent{
			Kind:   "input-needed",
			StepID: stepID,
			Inputs: inputDefsFromEvents(e.Inputs),
		})
		answers, source := s.awaitPromptAnswers(stepID, e.Inputs)
		_ = s.queue.Resolve(off, &events.PromptResolution{
			Answers: answers, Source: source, Timestamp: time.Now(),
		})
	}
}

// awaitPromptAnswers blocks on the inputs channel / runCtx / per-step
// deadline; matches the legacy webRenderer.Prompt select shape.
func (s *liveServer) awaitPromptAnswers(stepID string, inputs []events.Input) (map[string]any, string) {
	timeout := s.demo.EffectiveInputTimeout(stepID)
	var deadline <-chan time.Time
	if timeout > 0 {
		t := time.NewTimer(timeout)
		defer t.Stop()
		deadline = t.C
	}
	select {
	case values, ok := <-s.inputs:
		if !ok || values == nil {
			return map[string]any{}, "cancelled"
		}
		return values, "user-submitted"
	case <-s.runCtx.Done():
		return map[string]any{}, "cancelled"
	case <-deadline:
		s.broadcast(serverEvent{
			Kind:   "input-timeout",
			StepID: stepID,
			Extra:  map[string]any{"timeout_ms": timeout.Milliseconds()},
		})
		return defaultsForEventInputs(inputs), "timeout"
	}
}

// inputDefsFromEvents projects events.Input back into the legacy
// demokit.InputDef shape the WS frame's `Inputs` field carries (the
// JS player consumes that JSON shape).
func inputDefsFromEvents(in []events.Input) []demokit.InputDef {
	out := make([]demokit.InputDef, 0, len(in))
	for _, ev := range in {
		def := demokit.InputDef{
			Name:    ev.InputName(),
			Prompt:  ev.InputPrompt(),
			Default: ev.InputDefault(),
		}
		switch v := ev.(type) {
		case events.IntInput:
			def.Kind = "int"
		case events.ChoiceInput:
			def.Kind = "choice"
			def.Options = v.Options
		default:
			def.Kind = "string"
		}
		out = append(out, def)
	}
	return out
}

// defaultsForEventInputs builds the timeout-fallback answer map from
// the event projection — InputDefault() returns whatever the author
// set (typed value or nil).
func defaultsForEventInputs(inputs []events.Input) map[string]any {
	out := make(map[string]any, len(inputs))
	for _, in := range inputs {
		if d := in.InputDefault(); d != nil {
			out[in.InputName()] = d
		}
	}
	return out
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
