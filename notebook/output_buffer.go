package notebook

import (
	"bytes"
	"sync"
)

// OutputBuffer is the append-only, line-indexed sink an OutputCell
// reads from. Streaming chunks flow in via Append; the cell's
// HeightHint reads LineCount; RenderRows reads Lines(start, end).
//
// '\n' splits the stream into logical lines but is NOT a
// visibility gate: the in-flight trailing line (bytes written
// since the last '\n') is surfaced by the readers as the last
// logical line, so sub-line streaming — an agent emitting tokens,
// a progress line with no newline yet — is visible immediately
// instead of waiting for a '\n' to commit it. Once a '\n' arrives
// that line is finalized and the next write starts a fresh one.
//
// Lives in the notebook package (not notebook/cells) so the
// Notebook runtime can hand callers an io.Writer over a cell's
// buffer via Stream() without importing the cells subpackage.
//
// Append is safe to call concurrently with the readers (LineCount,
// Lines, AllLines) — readers take the RWMutex read lock.
type OutputBuffer struct {
	mu      sync.RWMutex
	pending []byte   // in-flight trailing line — surfaced as the last logical line
	lines   []string // finalized lines (each had a terminating \n, stored without it)
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

// LineCount returns the number of logical lines, including the
// in-flight trailing line when one is being assembled (bytes
// written since the last '\n'). Counts up the moment a partial
// line has any content, so callers reserve a row for it.
func (b *OutputBuffer) LineCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.lineCountLocked()
}

func (b *OutputBuffer) lineCountLocked() int {
	n := len(b.lines)
	if len(b.pending) > 0 {
		n++
	}
	return n
}

// Lines returns logical lines in the half-open range [start, end),
// clamped to the available range. The in-flight trailing line is
// included as the final line. An out-of-range request returns nil
// rather than panicking.
func (b *OutputBuffer) Lines(start, end int) []string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	total := b.lineCountLocked()
	if start < 0 {
		start = 0
	}
	if end > total {
		end = total
	}
	if start >= end {
		return nil
	}
	out := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		if i < len(b.lines) {
			out = append(out, b.lines[i])
		} else {
			out = append(out, string(b.pending))
		}
	}
	return out
}

// AllLines returns a copy of every logical line, including the
// in-flight trailing line. Intended for the OutputCell "copy
// entire cell" action.
func (b *OutputBuffer) AllLines() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]string, 0, b.lineCountLocked())
	out = append(out, b.lines...)
	if len(b.pending) > 0 {
		out = append(out, string(b.pending))
	}
	return out
}
