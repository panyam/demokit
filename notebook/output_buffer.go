package notebook

import (
	"bytes"
	"sync"
)

// OutputBuffer is the append-only, line-indexed sink an OutputCell
// reads from. Streaming chunks flow in via Append; the cell's
// HeightHint reads LineCount; RenderRows reads Lines(start, end).
//
// Lives in the notebook package (not notebook/cells) so the
// Notebook runtime can hand callers an io.Writer over a cell's
// buffer via Stream() without importing the cells subpackage.
//
// Append is safe to call concurrently with the readers (LineCount,
// Lines, AllLines) — readers take the RWMutex read lock.
type OutputBuffer struct {
	mu      sync.RWMutex
	pending []byte   // partial line being assembled — not in lines[] yet
	lines   []string // committed completed lines (no trailing \n)
}

// NewOutputBuffer returns an empty buffer ready to receive Append
// calls.
func NewOutputBuffer() *OutputBuffer {
	return &OutputBuffer{}
}

// Append commits bytes to the buffer, splitting on '\n' into the
// line index. A trailing partial line (no '\n' yet) is held in
// pending and concatenated with the next Append.
func (b *OutputBuffer) Append(p []byte) {
	if len(p) == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	pending := append(b.pending, p...)
	for {
		i := bytes.IndexByte(pending, '\n')
		if i < 0 {
			break
		}
		b.lines = append(b.lines, string(pending[:i]))
		pending = pending[i+1:]
	}
	b.pending = pending
}

// Write implements io.Writer so callers can stream into the buffer
// with fmt.Fprintf and friends. Always reports the full length
// written and a nil error.
func (b *OutputBuffer) Write(p []byte) (int, error) {
	b.Append(p)
	return len(p), nil
}

// LineCount returns the number of committed lines. The in-flight
// partial line (pending) is not counted — it becomes visible only
// after a newline arrives.
func (b *OutputBuffer) LineCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.lines)
}

// Lines returns lines in the half-open range [start, end), clamped
// to the available range. An out-of-range request returns nil
// rather than panicking.
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

// AllLines returns a copy of every committed line. Intended for
// the OutputCell "copy entire cell" action.
func (b *OutputBuffer) AllLines() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]string, len(b.lines))
	copy(out, b.lines)
	return out
}
