package notebook

import "io"

// outputBuffered is the optional interface a Cell implements when
// it exposes an OutputBuffer for streaming writes. Stream(id) is
// the supported entry point — it returns an io.Writer over the
// buffer (or io.Discard if the cell is missing or doesn't buffer).
//
// The buffer's own RWMutex makes writes safe from any goroutine;
// the model's repaint tick + the cell's render-time follow logic
// make new content visible within one frame, with no extra
// notification needed.
type outputBuffered interface {
	Buffer() *OutputBuffer
}

// Stream returns an io.Writer over the cell's OutputBuffer.
// Writes are safe from any goroutine and become visible on the
// next render tick. If the cell is missing or doesn't implement
// outputBuffered, Stream returns io.Discard so the caller never
// has to nil-check — a stream to a cell that was Removed mid-flight
// silently drops chunks.
func (nb *Notebook) Stream(id CellID) io.Writer {
	c, ok := nb.store.get(id)
	if !ok {
		return io.Discard
	}
	ob, ok := c.(outputBuffered)
	if !ok {
		return io.Discard
	}
	return ob.Buffer()
}
