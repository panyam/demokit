package notebook

import "sync"

// eventQueue is the append-only event log the renderer writes into
// and the model consumes from. Single-producer (demokit's Execute
// goroutine via the renderer) and single-consumer (Bubble Tea's
// goroutine via the model's drain), but the lock makes it
// safe to call Append from any goroutine — useful for the
// captureOutput reader, which streams chunks from its own
// goroutine.
//
// The contract: events are durable in memory from the moment
// Append returns. Consumers track an offset (the index of the
// next event to read) and read forward from there. A notify
// channel signals "new events available" — coalesced (capacity 1)
// so a burst of Appends causes at most one wake-up.
//
// This is the substrate for the event-sourced architecture
// described in issue 18: every renderer eventually consumes from
// a queue like this. Phase 1 keeps it notebook-private.
type eventQueue struct {
	mu     sync.RWMutex
	events []Event
	notify chan struct{}
}

// newEventQueue returns a queue ready to receive events. The
// notify channel is buffered with capacity 1 so a producer burst
// collapses into a single consumer wake-up.
func newEventQueue() *eventQueue {
	return &eventQueue{
		notify: make(chan struct{}, 1),
	}
}

// Append commits an event to the log and signals any waiting
// consumer. Safe to call concurrently with Read / Len.
func (q *eventQueue) Append(e Event) {
	q.mu.Lock()
	q.events = append(q.events, e)
	q.mu.Unlock()
	select {
	case q.notify <- struct{}{}:
	default:
	}
}

// Read returns the events at indices [from, len(events)) and the
// new offset (== len(events) at call time). Callers track the
// returned offset and pass it as `from` on the next call to drain
// only what's new.
//
// Returned slice is a copy of the queue's internal slice header
// for the requested range; the events themselves are immutable
// values held by both queue and caller. Safe to call from any
// goroutine.
func (q *eventQueue) Read(from int) (events []Event, newOffset int) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	if from < 0 {
		from = 0
	}
	if from >= len(q.events) {
		return nil, len(q.events)
	}
	out := make([]Event, len(q.events)-from)
	copy(out, q.events[from:])
	return out, len(q.events)
}

// Notify returns the wake-up channel for consumers to select on.
// Each receive corresponds to "at least one new event has been
// appended since the last receive." The consumer should then call
// Read to drain — the channel is coalesced, not a count.
func (q *eventQueue) Notify() <-chan struct{} {
	return q.notify
}

// Len returns the number of events in the queue. Used by the
// initial drain so the consumer reads everything that's already
// queued (events appended before the consumer started listening).
func (q *eventQueue) Len() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.events)
}
