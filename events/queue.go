package events

import (
	"errors"
	"sync"
)

// ErrAlreadyResolved is returned by Resolve when the event at
// the given offset has already been resolved by a prior caller.
// Multi-consumer scenarios race here; first writer wins.
var ErrAlreadyResolved = errors.New("events: already resolved")

// ErrNotResolvable is returned by Resolve when the event at the
// given offset isn't a sync event (WaitForAdvance / PromptOpen)
// and so has no Resolution field to fill in.
var ErrNotResolvable = errors.New("events: event is not resolvable")

// EventQueue is the producer-consumer log demokit.Execute appends
// to and renderers drain. Append is non-blocking and fast; Read
// returns events at a given offset → end; Notify signals
// "something appended."
//
// Sync events (WaitForAdvance, PromptOpen) carry nil Resolution
// pointers when appended; Resolve(offset, data) fills them in
// atomically. AwaitResolution(offset) blocks until the event at
// that offset is resolved, then returns the resolution data —
// this is how Execute synchronizes on user input without
// channels in the event payload.
//
// Single-producer / N-consumer in practice. Append + Resolve are
// safe to call from any goroutine; the lock around the slice is
// held only for the duration of the mutation.
type EventQueue struct {
	mu     sync.RWMutex
	events []Event
	notify chan struct{}

	// resolution-wait map: offset → channel closed when the
	// corresponding event's Resolution is filled in. Allocated on
	// first AwaitResolution per offset; cleaned up after the
	// resolve. Map access protected by mu (same lock as events).
	resolveSignals map[int]chan struct{}
}

// NewQueue returns an empty queue ready to receive events.
func NewQueue() *EventQueue {
	return &EventQueue{
		notify:         make(chan struct{}, 1),
		resolveSignals: map[int]chan struct{}{},
	}
}

// Append commits an event to the log and returns its offset.
// Signals waiting consumers via the notify channel (capacity 1
// → coalesced).
func (q *EventQueue) Append(e Event) int {
	q.mu.Lock()
	offset := len(q.events)
	q.events = append(q.events, e)
	q.mu.Unlock()
	select {
	case q.notify <- struct{}{}:
	default:
	}
	return offset
}

// Read returns the events at indices [from, len(events)) and the
// new offset. Callers track the returned offset and pass it as
// `from` on the next drain. Returned slice is a copy of the
// queue's internal slice header for the requested range; events
// themselves are read-only values from a caller's perspective
// (Resolve mutates only the queue's copy, but consumers should
// re-read offsets that might be sync events to see updated
// Resolution fields).
func (q *EventQueue) Read(from int) (events []Event, newOffset int) {
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

// ReadAt returns the event at a specific offset, or nil + false
// if out of range. Used after AwaitResolution returns to inspect
// the updated Resolution field of a sync event.
func (q *EventQueue) ReadAt(offset int) (Event, bool) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	if offset < 0 || offset >= len(q.events) {
		return nil, false
	}
	return q.events[offset], true
}

// Notify returns the wake-up channel for consumers to select on.
// Each receive corresponds to "at least one new event has been
// appended since the last receive."
func (q *EventQueue) Notify() <-chan struct{} {
	return q.notify
}

// Len returns the number of events in the queue.
func (q *EventQueue) Len() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.events)
}

// Resolve fills in the Resolution field of a sync event at the
// given offset. First-writer-wins: returns ErrAlreadyResolved if
// the event was already resolved by an earlier call.
//
// Accepts *AdvanceResolution (for WaitForAdvance) or
// *PromptResolution (for PromptOpen). Any other resolution type
// returns ErrNotResolvable.
//
// On success, wakes any AwaitResolution caller for this offset
// and signals notify so polling consumers can re-read the event
// and see its updated Resolution field.
func (q *EventQueue) Resolve(offset int, resolution any) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if offset < 0 || offset >= len(q.events) {
		return ErrNotResolvable
	}
	switch e := q.events[offset].(type) {
	case WaitForAdvance:
		if e.Resolution != nil {
			return ErrAlreadyResolved
		}
		ar, ok := resolution.(*AdvanceResolution)
		if !ok {
			return ErrNotResolvable
		}
		e.Resolution = ar
		q.events[offset] = e
	case PromptOpen:
		if e.Resolution != nil {
			return ErrAlreadyResolved
		}
		pr, ok := resolution.(*PromptResolution)
		if !ok {
			return ErrNotResolvable
		}
		e.Resolution = pr
		q.events[offset] = e
	default:
		return ErrNotResolvable
	}
	if ch, ok := q.resolveSignals[offset]; ok {
		close(ch)
		delete(q.resolveSignals, offset)
	}
	// Also wake general notify so polling consumers re-read this
	// offset and see its updated Resolution field.
	select {
	case q.notify <- struct{}{}:
	default:
	}
	return nil
}

// AwaitResolution blocks until the sync event at offset has been
// Resolved, then returns the resolution data:
//
//   - WaitForAdvance → returns *AdvanceResolution
//   - PromptOpen     → returns *PromptResolution
//
// If the event at offset isn't a sync event (or offset is out of
// range), returns nil immediately — no blocking, no deadlock. If
// the event is a sync event that's already resolved when called,
// returns the resolution data without blocking; replay supplies
// events with Resolution pre-filled and producers must not
// deadlock on them.
func (q *EventQueue) AwaitResolution(offset int) any {
	q.mu.Lock()
	if offset < 0 || offset >= len(q.events) {
		q.mu.Unlock()
		return nil
	}
	if !isSyncEvent(q.events[offset]) {
		q.mu.Unlock()
		return nil
	}
	// Already resolved? Return data immediately.
	if r := resolutionOf(q.events[offset]); r != nil {
		q.mu.Unlock()
		return r
	}
	// Pending — set up a wait channel for this offset.
	ch, ok := q.resolveSignals[offset]
	if !ok {
		ch = make(chan struct{})
		q.resolveSignals[offset] = ch
	}
	q.mu.Unlock()

	<-ch

	q.mu.RLock()
	defer q.mu.RUnlock()
	return resolutionOf(q.events[offset])
}

// isSyncEvent reports whether an event carries a resolvable
// Resolution field. Used by AwaitResolution to fail fast on
// non-sync events instead of blocking forever.
func isSyncEvent(e Event) bool {
	switch e.(type) {
	case WaitForAdvance, PromptOpen:
		return true
	default:
		return false
	}
}

// resolutionOf returns the resolution pointer of a sync event,
// or nil for non-sync events or unresolved sync events.
func resolutionOf(e Event) any {
	switch e := e.(type) {
	case WaitForAdvance:
		if e.Resolution == nil {
			return nil
		}
		return e.Resolution
	case PromptOpen:
		if e.Resolution == nil {
			return nil
		}
		return e.Resolution
	default:
		return nil
	}
}
