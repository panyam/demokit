package notebook

import (
	"bytes"
	"sync"
)

// OutputBuffer is the append-only, line-indexed sink an OutputCell
// reads from. captureOutput chunks from a running step flow in via
// Append; the cell's HeightHint reads LineCount; RenderRows reads
// Lines(start, end). Phase A: pure in-memory. Phase D adds optional
// spill-to-temp-file when over a byte threshold — the contract here
// is shaped so cells never see the difference.
//
// Subscribers (the Bubble Tea program) receive a debounced wake-up
// signal on the channel returned by Subscribe so the viewport can
// schedule a redraw without ticking on every byte. A single
// channel is shared across all subscribers in this phase — there is
// at most one (the program), so no fan-out yet.
type OutputBuffer struct {
	mu        sync.RWMutex
	pending   []byte   // partial line being assembled — not in lines[] yet
	lines     []string // committed completed lines (no trailing \n)
	wakeup    chan struct{}
}

// NewOutputBuffer returns an empty buffer ready to receive Append
// calls. The wakeup channel is buffered with capacity 1 so a fast
// burst of Appends collapses into a single redraw notification —
// the subscriber drains the channel non-blockingly on each tick.
func NewOutputBuffer() *OutputBuffer {
	return &OutputBuffer{
		wakeup: make(chan struct{}, 1),
	}
}

// Append commits bytes to the buffer, splitting on '\n' into the
// line index. A trailing partial line (no '\n' yet) is held in
// `pending` and concatenated with the next Append. After committing
// at least one new line, sends a non-blocking wakeup so any
// subscriber knows to schedule a viewport refresh.
//
// Safe to call from a goroutine concurrent with readers (LineCount,
// Lines) — those take the read lock.
func (b *OutputBuffer) Append(p []byte) {
	if len(p) == 0 {
		return
	}
	b.mu.Lock()
	pending := append(b.pending, p...)
	wroteLine := false
	for {
		i := bytes.IndexByte(pending, '\n')
		if i < 0 {
			break
		}
		b.lines = append(b.lines, string(pending[:i]))
		pending = pending[i+1:]
		wroteLine = true
	}
	b.pending = pending
	b.mu.Unlock()

	if wroteLine {
		select {
		case b.wakeup <- struct{}{}:
		default:
		}
	}
}

// LineCount returns the number of committed lines. The in-flight
// partial line (`pending`) is not counted — RenderRows would
// otherwise have to display half-rendered tails. The pending line
// becomes visible only after a newline arrives.
func (b *OutputBuffer) LineCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.lines)
}

// Lines returns lines in the half-open range [start, end). Clamped
// to the available range; an out-of-range request returns an empty
// slice rather than panicking. Returns copies of the slice header
// (the strings themselves are immutable in Go) so callers can hold
// the result without locking.
func (b *OutputBuffer) Lines(start, end int) []string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if start < 0 {
		start = 0
	}
	if end > len(b.lines) {
		end = len(b.lines)
	}
	if start >= end {
		return nil
	}
	out := make([]string, end-start)
	copy(out, b.lines[start:end])
	return out
}

// Subscribe returns the wakeup channel. The subscriber drains
// non-blockingly each tick — every receive collapses any burst of
// Appends that happened since the last drain into a single redraw.
// Phase A has at most one subscriber (the Bubble Tea program); the
// channel is exposed rather than registered to keep the contract
// minimal.
func (b *OutputBuffer) Subscribe() <-chan struct{} {
	return b.wakeup
}

// AllLines returns a copy of every committed line. Intended for the
// OutputCell's "copy entire cell" action — c on a focused OutputCell.
// Cheap allocation; not on the hot redraw path.
func (b *OutputBuffer) AllLines() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]string, len(b.lines))
	copy(out, b.lines)
	return out
}
