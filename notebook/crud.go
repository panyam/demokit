package notebook

// CRUD on cells. The Notebook delegates to the store; mutations
// are safe from any goroutine. The model's repaint tick picks
// changes up within one frame.

// Append adds c to the end of the cell list. On a duplicate ID
// returns an error; the cell is not added. Auto-wires the
// configured clipboard into cells that implement SetClipboard.
func (nb *Notebook) Append(c Cell) (CellID, error) {
	nb.injectClipboard(c)
	return nb.store.insert(-1, c)
}

// Insert places c at the given index — index < 0 or past the end
// appends. On a duplicate ID returns an error; the cell is not
// inserted. Auto-wires the configured clipboard.
func (nb *Notebook) Insert(index int, c Cell) (CellID, error) {
	nb.injectClipboard(c)
	return nb.store.insert(index, c)
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
func (nb *Notebook) Remove(id CellID) bool {
	return nb.store.remove(id)
}

// Get returns the cell with the given ID. The bool reports
// presence.
func (nb *Notebook) Get(id CellID) (Cell, bool) { return nb.store.get(id) }

// IndexOf returns the 0-based position of the cell with the given
// ID. The bool reports presence.
func (nb *Notebook) IndexOf(id CellID) (int, bool) { return nb.store.indexOf(id) }

// Len returns the current cell count.
func (nb *Notebook) Len() int { return nb.store.count() }

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
