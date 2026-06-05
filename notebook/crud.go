package notebook

// CRUD on cells. The Notebook delegates to the store; mutations
// are safe from any goroutine. The model's repaint tick picks
// changes up within one frame.

// Reveal tells Append / Insert whether to scroll the new cell into
// view. It is a mandatory parameter (not an option with a silent
// default) so every caller makes a deliberate choice — the common
// "I appended a streaming cell but it never shows up" bug comes from
// forgetting to follow it, and a required arg surfaces that at the
// call site instead of at runtime on a short terminal.
type Reveal int

const (
	// RevealNone leaves the viewport where it is. The new cell may be
	// off-screen until the user scrolls to it. Pass this explicitly
	// when the cell isn't what the user should be looking at.
	RevealNone Reveal = iota
	// RevealBottom scrolls the new cell into view and makes it the
	// viewport's follow anchor (the cursor), so it stays visible as it
	// streams/grows — the `tail -f` behavior. Use for output a caller
	// streams into, prompts, and anything the user should track.
	RevealBottom
	// RevealTop scrolls the new cell so it sits at the top of the
	// body viewport. One-shot, not sticky: unlike RevealBottom this
	// does NOT establish a follow anchor (cursor is untouched), so
	// the user can scroll freely afterward and a subsequent
	// streaming append won't pull the viewport away. Suits
	// navigation-style "show me this cell" jumps.
	RevealTop
	// RevealMiddle scrolls the new cell so it sits vertically
	// centered in the body viewport. Same one-shot semantics as
	// RevealTop. Cells taller than the body clamp to top
	// alignment (Middle has no meaningful answer there).
	RevealMiddle
)

// Append adds c to the end of the cell list and reveals it per the
// given Reveal. On a duplicate ID returns an error; the cell is not
// added. Auto-wires the configured clipboard into cells that
// implement SetClipboard.
func (nb *Notebook) Append(c Cell, reveal Reveal) (CellID, error) {
	return nb.Insert(-1, c, reveal)
}

// Insert places c at the given index — index < 0 or past the end
// appends — and reveals it per the given Reveal. On a duplicate ID
// returns an error; the cell is not inserted. Auto-wires the
// configured clipboard.
func (nb *Notebook) Insert(index int, c Cell, reveal Reveal) (CellID, error) {
	nb.injectClipboard(c)
	id, err := nb.store.insert(index, c)
	if err != nil {
		return id, err
	}
	switch reveal {
	case RevealBottom:
		// Make the new cell the cursor; the model's per-frame
		// ensureCursorVisible then keeps it in view as it grows.
		nb.store.setCursorByIdx(nb.indexOrEndLocked(id))
	case RevealTop, RevealMiddle:
		// Move the cursor to the new cell (so the user navigates
		// from where they're looking) AND queue a one-shot
		// alignment that overrides ensureCursorVisible's default
		// placement — Top puts the cell at the viewport's top
		// row, Middle centers it. Because the cursor lands on the
		// aligned cell, subsequent RevealNone appends below
		// don't pull the viewport: ensureCursorVisible is a
		// no-op while the cursor cell stays in view, so the
		// alignment is one-shot in the user-visible sense.
		newIdx := nb.indexOrEndLocked(id)
		nb.store.setCursorByIdx(newIdx)
		nb.store.setPendingAlign(newIdx, reveal)
	}
	return id, nil
}

// indexOrEndLocked returns the current index of id, or the last
// index if not found (shouldn't happen right after a successful
// insert). Used to anchor RevealBottom on the just-inserted cell.
func (nb *Notebook) indexOrEndLocked(id CellID) int {
	if idx, ok := nb.store.indexOf(id); ok {
		return idx
	}
	return nb.store.count() - 1
}

// Update replaces the cell with the given ID by fn(old). Returns
// false if no such cell. fn runs under the store lock — it must
// be quick and must not call back into the Notebook.
func (nb *Notebook) Update(id CellID, fn func(Cell) Cell) bool {
	return nb.store.update(id, fn)
}

// Remove deletes the cell with the given ID. Returns false if no
// such cell. Adjusts the cursor: a removal before it shifts it
// up; removing the focused cell leaves the cursor on the next
// cell down (or the new last cell if the removed cell was the
// end).
//
// If there's a pending AwaitInputBy on the removed cell, it
// resolves with Source: "cancelled" so the caller goroutine
// doesn't block forever.
func (nb *Notebook) Remove(id CellID) bool {
	ok := nb.store.remove(id)
	if ok {
		nb.rdv.resolveInput(id, nil, "cancelled")
	}
	return ok
}

// Get returns the cell with the given ID. The bool reports
// presence.
func (nb *Notebook) Get(id CellID) (Cell, bool) { return nb.store.get(id) }

// IndexOf returns the 0-based position of the cell with the given
// ID. The bool reports presence.
func (nb *Notebook) IndexOf(id CellID) (int, bool) { return nb.store.indexOf(id) }

// Len returns the current cell count.
func (nb *Notebook) Len() int { return nb.store.count() }

// AlignCell moves the cursor to the cell with the given ID and
// adjusts the viewport so the cell sits at the requested
// position. Returns false (no-op) if no such cell.
//
// Honors the same alignment semantics as Append/Insert:
//
//   - RevealNone: no-op (returns true if the cell exists).
//   - RevealTop / RevealMiddle: cursor moves to the cell and a
//     one-shot viewport alignment fires next frame, placing the
//     cell at the top of the body or vertically centered. The
//     cursor pin keeps subsequent RevealNone appends from
//     pulling the viewport — the "jump-and-focus" semantic
//     PR 60 introduced.
//   - RevealBottom: cursor moves to the cell and the model's
//     per-frame ensureCursorVisible pulls it to the bottom edge.
//     Subsequent appends keep dragging the viewport down
//     (`tail -f`).
//
// Safe to call from any goroutine. Cell lookup, cursor mutation,
// and pending-align queue are all guarded by the store mutex.
// No tea.Msg is sent, so it's safe inside KeyMap action handlers
// (unlike SetMode / FocusCell).
func (nb *Notebook) AlignCell(id CellID, alignment Reveal) bool {
	idx, ok := nb.store.indexOf(id)
	if !ok {
		return false
	}
	switch alignment {
	case RevealBottom:
		nb.store.setCursorByIdx(idx)
	case RevealTop, RevealMiddle:
		nb.store.setCursorByIdx(idx)
		nb.store.setPendingAlign(idx, alignment)
	}
	return true
}

// SetCursor moves the cursor to the cell with the given ID and
// returns true. Returns false (no-op) if no such cell.
func (nb *Notebook) SetCursor(id CellID) bool {
	idx, ok := nb.store.indexOf(id)
	if !ok {
		return false
	}
	nb.store.setCursorByIdx(idx)
	return true
}

// FocusCell sets the cursor to the named cell AND switches to
// CellActiveMode so the cell has focus immediately. Useful right after
// Appending a prompt cell when you want the user to type into it
// without manually navigating + entering focus first.
//
// Returns false if no such cell.
//
// NOT safe to call from inside a KeyMap action handler — it
// transitively calls [Notebook.SetMode], which Sends to the BT
// loop and deadlocks when invoked on the BT goroutine itself
// (silent UI freeze). From a KeyMap action, do the two parts
// directly: call [Notebook.SetCursor](id) (safe store mutation) and
// return [ModeCmd](CellActiveMode) so the mode flips as a tea.Cmd.
// See ARCHITECTURE.md § Concurrency model.
func (nb *Notebook) FocusCell(id CellID) bool {
	if !nb.SetCursor(id) {
		return false
	}
	nb.SetMode(CellActiveMode)
	return true
}

// SetHeader sets the notebook's top-of-screen title + subtitle.
func (nb *Notebook) SetHeader(title, desc string) {
	nb.store.setHeader(title, desc)
}

// SetDone flips the "·Done" indicator next to the header. Doesn't
// quit the program — call Stop for that.
func (nb *Notebook) SetDone() {
	nb.store.setDone()
}

// clipboardSetter is the optional interface a Cell implements if
// it wants the Notebook's clipboard auto-injected on Append/Insert.
// Built-in cells (Note, Verbatim, Output) implement it; custom
// cells can too, or ignore the convention and wire their own.
type clipboardSetter interface {
	SetClipboard(Clipboard)
}

func (nb *Notebook) injectClipboard(c Cell) {
	if nb.clip == nil {
		return
	}
	if setter, ok := c.(clipboardSetter); ok {
		setter.SetClipboard(nb.clip)
	}
}
