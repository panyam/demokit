package demokit

import (
	"testing"

	"github.com/panyam/demokit/events"
)

// Source side of the advance handshake contract: an event-aware
// renderer calls queue.Resolve when the user advances, while the
// demokit loop sits in AwaitResolution. If Resolve lands FIRST, the
// resolution must not be lost. demokit's queue honors this; the
// notebook rendezvous mirrors it (see notebook.TestRendezvous...).
//
// Characterization test — guards the behavior the WaitForAdvance
// flow depends on against a future queue change.
func TestQueueResolveBeforeAwaitNotLost(t *testing.T) {
	q := events.NewQueue()
	off := q.Append(events.WaitForAdvance{})
	if err := q.Resolve(off, &events.AdvanceResolution{Source: "user"}); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	got := q.AwaitResolution(off) // resolution already present — must return, not block
	res, ok := got.(*events.AdvanceResolution)
	if !ok || res.Source != "user" {
		t.Fatalf("AwaitResolution after Resolve = %#v, want *events.AdvanceResolution{Source:user}", got)
	}
}
