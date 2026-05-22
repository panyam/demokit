package notebook

import (
	"testing"
	"time"
)

// Consumer side of the advance/prompt contract: a caller focuses the
// cell (enabling Enter -> resolveInput) and only then calls
// AwaitInputBy (registerInput). If the resolution lands FIRST it must
// not be lost — registerInput delivers the buffered resolution.
func TestRendezvousResolveBeforeRegisterNotLost(t *testing.T) {
	r := newRendezvous()
	r.resolveInput("advance-1", nil, "user-submitted") // Enter arrives first
	ch := r.registerInput("advance-1")                 // AwaitInputBy registers after
	select {
	case resp := <-ch:
		if resp.Source != "user-submitted" {
			t.Fatalf("source = %q, want user-submitted", resp.Source)
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatal("resolution arriving before registerInput was dropped (lost wakeup)")
	}
}

// Normal order still works and isn't disturbed by the early-buffer path.
func TestRendezvousRegisterThenResolve(t *testing.T) {
	r := newRendezvous()
	ch := r.registerInput("p1")
	if !r.resolveInput("p1", map[string]any{"x": 1}, "user-submitted") {
		t.Fatal("resolveInput should report a live waiter")
	}
	select {
	case resp := <-ch:
		if resp.Answers["x"] != 1 {
			t.Fatalf("answers = %v", resp.Answers)
		}
	case <-time.After(300 * time.Millisecond):
		t.Fatal("registered waiter never received its resolution")
	}
}

// A buffered early resolution is discarded by removeInput, so it can't
// leak or be delivered to a later, unrelated waiter for the same id.
func TestRendezvousRemoveClearsEarly(t *testing.T) {
	r := newRendezvous()
	r.resolveInput("gone", nil, "user-submitted") // buffered, no waiter
	r.removeInput("gone")                         // cell removed before any await

	ch := r.registerInput("gone")
	select {
	case resp := <-ch:
		t.Fatalf("expected no buffered resolution after removeInput, got %+v", resp)
	case <-time.After(100 * time.Millisecond):
		// correct: nothing delivered
	}
}
