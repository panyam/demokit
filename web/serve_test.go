package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/panyam/demokit"
)

// dialEvents opens a WebSocket to the test server's /events endpoint
// and returns the conn plus a cleanup. Tests then use readEvent /
// sendInput / sendReset to drive the protocol.
func dialEvents(t *testing.T, ts *httptest.Server) *websocket.Conn {
	t.Helper()
	u, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("parse test url: %v", err)
	}
	u.Scheme = "ws"
	u.Path = "/events"
	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("dial /events: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// readEvent reads one JSON message from the WS, with a deadline so
// stuck tests fail fast instead of timing out the whole test binary.
func readEvent(t *testing.T, c *websocket.Conn) map[string]any {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data, err := c.ReadMessage()
	if err != nil {
		t.Fatalf("read event: %v", err)
	}
	var evt map[string]any
	if err := json.Unmarshal(data, &evt); err != nil {
		t.Fatalf("decode event: %v (raw: %s)", err, data)
	}
	return evt
}

// readUntil drains events until one with kind == want is seen,
// returning that event. Skips ping frames (servicekit sends keepalive
// JSON pings on the same channel) so test assertions can focus on
// demo-protocol kinds.
func readUntil(t *testing.T, c *websocket.Conn, want string) map[string]any {
	t.Helper()
	for i := 0; i < 50; i++ {
		evt := readEvent(t, c)
		if evt["type"] == "ping" {
			continue
		}
		if evt["kind"] == want {
			return evt
		}
	}
	t.Fatalf("never saw event kind=%q after 50 reads", want)
	return nil
}

// sendInput posts an "input" command to the server.
func sendInput(t *testing.T, c *websocket.Conn, values map[string]any) {
	t.Helper()
	if err := c.WriteJSON(map[string]any{"kind": "input", "values": values}); err != nil {
		t.Fatalf("send input: %v", err)
	}
}

// sendReset posts a "reset" command to the server.
func sendReset(t *testing.T, c *websocket.Conn) {
	t.Helper()
	if err := c.WriteJSON(map[string]any{"kind": "reset"}); err != nil {
		t.Fatalf("send reset: %v", err)
	}
}

// twoStepDemo builds a tiny demo with one input-bearing step and one
// terminal step, suitable for round-trip tests. Returns the demo and
// a slice that records each step's observed inputs (for assertions).
func twoStepDemo() (*demokit.Demo, *[]map[string]any) {
	var mu sync.Mutex
	seen := []map[string]any{}
	d := demokit.New("Test Demo").
		Description("for unit tests").
		MaxSteps(10).
		MaxVisits(2)

	d.Step("Pick").ID("pick").
		Input(demokit.Choice("a", "b").Named("choice", "Pick").WithDefault("a")).
		Run(func(ctx demokit.StepContext) *demokit.StepResult {
			mu.Lock()
			seen = append(seen, ctx.Inputs)
			mu.Unlock()
			return nil
		})
	d.Step("End").ID("end").
		Run(func(ctx demokit.StepContext) *demokit.StepResult {
			mu.Lock()
			seen = append(seen, ctx.Inputs)
			mu.Unlock()
			return nil
		})
	return d, &seen
}

// TestRoundTrip_HeaderInputResultDone verifies the canonical event
// sequence for a simple demo with one Choice prompt: header,
// step-start (pick), input-needed, step-end (pick), step-start
// (end), step-end (end), done. Confirms inputs round-trip from
// browser-equivalent WS message into the demo's StepContext.Inputs.
func TestRoundTrip_HeaderInputResultDone(t *testing.T) {
	demo, seen := twoStepDemo()
	srv, handler := newLiveServer(demo)
	defer srv.shutdown()
	ts := httptest.NewServer(handler)
	defer ts.Close()

	c := dialEvents(t, ts)

	if got := readUntil(t, c, "header")["demo"].(map[string]any)["title"]; got != "Test Demo" {
		t.Fatalf("header title: got %v want %v", got, "Test Demo")
	}
	readUntil(t, c, "step-start")    // pick
	readUntil(t, c, "input-needed")  // for pick

	sendInput(t, c, map[string]any{"choice": "b"})

	readUntil(t, c, "step-end")   // pick closed
	readUntil(t, c, "step-start") // end
	readUntil(t, c, "step-end")   // end closed
	readUntil(t, c, "done")

	if len(*seen) != 2 {
		t.Fatalf("expected 2 step runs, got %d", len(*seen))
	}
	if (*seen)[0]["choice"] != "b" {
		t.Fatalf("first step inputs: got %v want choice=b", (*seen)[0])
	}
}

// TestAbortVisible_SyntheticStepStart verifies that when MaxVisits
// trips and demokit calls RenderResult without a preceding
// RenderStep, the webRenderer synthesizes a "step-start" with id
// "__demokit_aborted__" so the player has something to mark with
// the error status. Without this, the abort would be silent and
// the trailing step-end would have no element to attach to.
func TestAbortVisible_SyntheticStepStart(t *testing.T) {
	demo := demokit.New("Loop").MaxSteps(20).MaxVisits(2)
	demo.Step("Loop").ID("loop").
		Run(func(ctx demokit.StepContext) *demokit.StepResult {
			return &demokit.StepResult{Next: "loop"}
		})
	srv, handler := newLiveServer(demo)
	defer srv.shutdown()
	ts := httptest.NewServer(handler)
	defer ts.Close()

	c := dialEvents(t, ts)

	// First two visits succeed; on the 3rd, MaxVisits trips and the
	// abort path fires RenderResult without a preceding RenderStep.
	// We expect an "Aborted" step to appear before the final step-end.
	sawAborted := false
	for i := 0; i < 30; i++ {
		evt := readEvent(t, c)
		if evt["kind"] == "step-start" {
			if extra, ok := evt["extra"].(map[string]any); ok {
				if extra["id"] == "__demokit_aborted__" {
					sawAborted = true
				}
			}
		}
		if evt["kind"] == "done" || evt["kind"] == "step-end" && sawAborted {
			break
		}
	}
	if !sawAborted {
		t.Fatal("never saw synthetic step-start{id:__demokit_aborted__} on MaxVisits abort")
	}
}

// TestLateJoiner_GetsHistory verifies that a client connecting after
// the demo has already emitted some events still receives those
// events on connect — the server replays its history buffer to
// every new WS client so late embedders aren't confused by missing
// state. Without history replay, opening the page mid-run would
// show a blank widget until the next event happens.
func TestLateJoiner_GetsHistory(t *testing.T) {
	demo, _ := twoStepDemo()
	srv, handler := newLiveServer(demo)
	defer srv.shutdown()
	ts := httptest.NewServer(handler)
	defer ts.Close()

	// First client drives the first step to completion so history
	// has header + step-start + step-end + step-start (the second
	// step is now waiting for input — but the second step has no
	// inputs, so it just runs to completion).
	c1 := dialEvents(t, ts)
	readUntil(t, c1, "input-needed")
	sendInput(t, c1, map[string]any{"choice": "a"})
	readUntil(t, c1, "done")

	// Second client connects after the demo finished. It should
	// receive the full history, including the final "done".
	c2 := dialEvents(t, ts)
	readUntil(t, c2, "header")
	readUntil(t, c2, "done")
}

// TestStreamingChunks_ArriveBeforeStepEnd verifies a step that
// fmt.Println()s during Run produces "chunk" events on the WS
// before the "step-end" event. Catches regressions in the
// captureOutput → onChunk → StreamOutput path that would
// otherwise only surface as user-visible "drips don't drip"
// in the browser.
func TestStreamingChunks_ArriveBeforeStepEnd(t *testing.T) {
	demo := demokit.New("Stream").MaxSteps(5)
	demo.Step("Drip").ID("drip").
		Run(func(ctx demokit.StepContext) *demokit.StepResult {
			fmt.Println("HELLO_MARKER")
			return nil
		})
	srv, handler := newLiveServer(demo)
	defer srv.shutdown()
	ts := httptest.NewServer(handler)
	defer ts.Close()

	c := dialEvents(t, ts)

	// Iterate to "done", asserting that a chunk event with the
	// marker arrives along the way. The chunk MUST appear before
	// step-end — captureOutput's drain feeds onChunk before
	// closing, so step-end's broadcast comes after.
	sawChunk := false
	sawStepEnd := false
	for i := 0; i < 30; i++ {
		evt := readEvent(t, c)
		switch evt["kind"] {
		case "chunk":
			if s, _ := evt["chunk"].(string); strings.Contains(s, "HELLO_MARKER") {
				if sawStepEnd {
					t.Fatal("chunk arrived AFTER step-end — capture/broadcast ordering is wrong")
				}
				sawChunk = true
			}
		case "step-end":
			sawStepEnd = true
		case "done":
			i = 30
		}
	}
	if !sawChunk {
		t.Fatal("never saw chunk event with HELLO_MARKER")
	}
}

// TestShutdown_UnblocksPendingPrompt verifies that calling
// liveServer.shutdown() while a step is blocked on Prompt
// (waiting for WS input) returns control to the run goroutine
// promptly via the runCtx select. Regression catch for the
// hang-on-Ctrl-C bug — without runCtx in Prompt's select,
// shutdown blocks the full drain timeout.
func TestShutdown_UnblocksPendingPrompt(t *testing.T) {
	demo, _ := twoStepDemo()
	srv, handler := newLiveServer(demo)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	c := dialEvents(t, ts)
	readUntil(t, c, "input-needed") // blocked on Prompt

	done := make(chan struct{})
	go func() {
		srv.shutdown()
		<-srv.runDone
		close(done)
	}()

	select {
	case <-done:
		// good
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown + runDone took longer than 2s — Prompt likely didn't observe runCtx.Done()")
	}
}

// TestInputTimeout_FillsDefaults verifies Demo.InputTimeout fires
// after the configured deadline, broadcasts an input-timeout event,
// and supplies declared defaults to the demo's Run so the loop
// continues. Without this, an unattended kiosk would block forever.
func TestInputTimeout_FillsDefaults(t *testing.T) {
	demo, seen := twoStepDemo()
	demo.InputTimeout(150 * time.Millisecond)
	srv, handler := newLiveServer(demo)
	defer srv.shutdown()
	ts := httptest.NewServer(handler)
	defer ts.Close()

	c := dialEvents(t, ts)
	readUntil(t, c, "input-needed")

	// Don't send input — wait for the timeout to fire.
	evt := readUntil(t, c, "input-timeout")
	if evt["step_id"] != "pick" {
		t.Fatalf("input-timeout step_id: got %v want pick", evt["step_id"])
	}

	readUntil(t, c, "step-end")
	readUntil(t, c, "done")

	if len(*seen) < 1 {
		t.Fatal("first step never ran after input-timeout")
	}
	if (*seen)[0]["choice"] != "a" {
		t.Fatalf("expected default choice=a after timeout, got %v", (*seen)[0])
	}
}

// TestReset_ClearsHistoryAndRestarts verifies that sending
// {"kind":"reset"} cancels the current run, clears history, and
// re-launches from the top. The reset event itself is broadcast so
// connected clients clear their feeds; the next "header" appears
// on the fresh run.
func TestReset_ClearsHistoryAndRestarts(t *testing.T) {
	demo, _ := twoStepDemo()
	srv, handler := newLiveServer(demo)
	defer srv.shutdown()
	ts := httptest.NewServer(handler)
	defer ts.Close()

	c := dialEvents(t, ts)
	readUntil(t, c, "input-needed") // blocked on Prompt

	sendReset(t, c)

	// Server emits reset, then the new run starts: header again.
	readUntil(t, c, "reset")
	readUntil(t, c, "header")
	readUntil(t, c, "step-start")
	readUntil(t, c, "input-needed")

	// History was cleared — a brand-new client connecting now
	// should NOT see the events from before the reset.
	c2 := dialEvents(t, ts)
	first := readUntil(t, c2, "header")
	if first["demo"].(map[string]any)["title"] != "Test Demo" {
		t.Fatalf("late client got unexpected header: %v", first)
	}
}

// TestServerInternals_NewLiveServerExits verifies that a
// no-op demo with no steps runs to completion through
// newLiveServer + shutdown without leaking the runDemo
// goroutine. Belt-and-suspenders against future changes
// to runDemo's defer chain.
func TestServerInternals_NewLiveServerExits(t *testing.T) {
	demo := demokit.New("Empty")
	srv, handler := newLiveServer(demo)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	select {
	case <-srv.runDone:
		// demo with zero steps exits immediately
	case <-time.After(2 * time.Second):
		t.Fatal("empty-demo runDemo did not exit within 2s")
	}

	srv.shutdown()
	// idempotent
	srv.shutdown()
	_ = context.Background()
}
