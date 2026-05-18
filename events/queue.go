package events

import "github.com/panyam/gocurrent"

// EventQueue is the canonical queue type demokit emits events
// into. It's a type alias over gocurrent.Queue[Event] — same
// methods, same semantics, just named for the demokit domain.
//
// The full API surface (Append, AppendBarrier, ReadFrom, ReadAt,
// Len, Subscribe, Resolve, AwaitResolution, Resolution) lives in
// gocurrent. See https://pkg.go.dev/github.com/panyam/gocurrent
// for full docs.
//
// Why the alias: the queue is a pure concurrency primitive —
// nothing about it is demokit-specific. Keeping it in gocurrent
// lets other consumers (servicekit, devloop, future tools) reach
// for the same primitive. The alias preserves the demokit-domain
// name at consumer call sites.
type EventQueue = gocurrent.Queue[Event]

// Subscription is the per-consumer wake-up handle on an
// EventQueue. Type alias for gocurrent.Subscription.
type Subscription = gocurrent.Subscription

// NewQueue returns an empty EventQueue ready to receive events.
func NewQueue() *EventQueue {
	return gocurrent.NewQueue[Event]()
}

// Re-exported error variables for callers that want to match on
// queue-level errors without importing gocurrent directly.
var (
	ErrAlreadyResolved  = gocurrent.ErrAlreadyResolved
	ErrOffsetOutOfRange = gocurrent.ErrOffsetOutOfRange
)
