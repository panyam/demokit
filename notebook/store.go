package notebook

import (
	"fmt"
	"sync"
)

// CellID identifies a cell within a notebook. It's an alias for
// string (a cell's ID() value) so it reads as documentation in
// signatures without forcing conversions at every call site.
type CellID = string

// store is the shared, RWMutex-guarded notebook state: the cell
// list, the focused-cell cursor, and the header. The Notebook's
// CRUD methods mutate it from caller goroutines; the model reads
// it from the Bubble Tea goroutine. One mutex covers everything
// mutated from more than one goroutine.
//
// viewport/width/height/mode are NOT here — only the BT goroutine
// touches those, so they live on the model without a lock.
type store struct {
	mu         sync.RWMutex
	cells      []Cell
	cursor     int
	header     string
	headerDesc string
	done       bool
}

func newStore() *store { return &store{} }

// snapshot is an immutable point-in-time copy the model renders
// from, so View doesn't hold the lock across a full render. The
// cells slice is copied (cheap — pointers); the cells themselves
// are shared.
type snapshot struct {
	cells  []Cell
	cursor int
	header string
	desc   string
	done   bool
}

func (s *store) snapshot() snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cp := make([]Cell, len(s.cells))
	copy(cp, s.cells)
	return snapshot{cp, s.cursor, s.header, s.headerDesc, s.done}
}

// insert places c at index (index < 0 or past the end appends).
// Returns an error if a cell with the same ID already exists. The
// cursor tracks its cell: an insert at or before the cursor shifts
// it down by one (unless the list was previously empty).
func (s *store) insert(index int, c Cell) (CellID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := c.ID()
	for _, existing := range s.cells {
		if existing.ID() == id {
			return "", fmt.Errorf("notebook: duplicate cell ID %q", id)
		}
	}
	if index < 0 || index > len(s.cells) {
		index = len(s.cells)
	}
	s.cells = append(s.cells, nil)
	copy(s.cells[index+1:], s.cells[index:])
	s.cells[index] = c
	// len(s.cells) > 1 means there was at least one cell before
	// this insert — only then can the cursor be tracking a real
	// cell that just shifted.
	if index <= s.cursor && len(s.cells) > 1 {
		s.cursor++
	}
	return id, nil
}

// remove deletes the cell with the given ID. Returns false if no
// such cell. The cursor follows: a removal before it shifts it up;
// removing the focused cell leaves the cursor on the next cell
// down (or the new last cell if it was the end).
func (s *store) remove(id CellID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := -1
	for i, c := range s.cells {
		if c.ID() == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return false
	}
	s.cells = append(s.cells[:idx], s.cells[idx+1:]...)
	if idx < s.cursor {
		s.cursor--
	}
	s.clampCursorLocked()
	return true
}

// update replaces the cell with the given ID by fn(old). fn runs
// under the store lock — it must be quick and must not call back
// into the Notebook.
func (s *store) update(id CellID, fn func(Cell) Cell) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, c := range s.cells {
		if c.ID() == id {
			s.cells[i] = fn(c)
			return true
		}
	}
	return false
}

func (s *store) get(id CellID) (Cell, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, c := range s.cells {
		if c.ID() == id {
			return c, true
		}
	}
	return nil, false
}

func (s *store) indexOf(id CellID) (int, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i, c := range s.cells {
		if c.ID() == id {
			return i, true
		}
	}
	return -1, false
}

func (s *store) count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.cells)
}

// moveCursor shifts the cursor by delta, clamped to the cell range.
func (s *store) moveCursor(delta int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cursor += delta
	s.clampCursorLocked()
}

// setCursorByIdx sets the cursor to an absolute index, clamped.
func (s *store) setCursorByIdx(idx int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cursor = idx
	s.clampCursorLocked()
}

func (s *store) cursorPos() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cursor
}

func (s *store) setHeader(title, desc string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.header = title
	s.headerDesc = desc
}

func (s *store) setDone() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.done = true
}

// clampCursorLocked pins the cursor to [0, len-1] (or 0 when
// empty). Caller must hold the write lock.
func (s *store) clampCursorLocked() {
	if s.cursor >= len(s.cells) {
		s.cursor = len(s.cells) - 1
	}
	if s.cursor < 0 {
		s.cursor = 0
	}
}
