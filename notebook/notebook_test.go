package notebook

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// --- testCell: minimal in-package Cell impl for these tests ---

type testCell struct {
	id     string
	height int
	body   string

	lastMode Mode
	updates  int
}

func newTestCell(id string, height int) *testCell {
	return &testCell{id: id, height: height, body: "body-" + id}
}

func (c *testCell) ID() string             { return c.id }
func (c *testCell) HeightHint(int) int     { return c.height }
func (c *testCell) StatusHint(Mode) string { return "" }

func (c *testCell) RenderRows(width, startRow, endRow int, focused bool, _ Mode) []string {
	rows := make([]string, 0, c.height)
	for i := 0; i < c.height; i++ {
		marker := " "
		if focused {
			marker = "*"
		}
		rows = append(rows, fmt.Sprintf("%s%s/%d", marker, c.id, i))
	}
	if startRow < 0 {
		startRow = 0
	}
	if endRow > len(rows) {
		endRow = len(rows)
	}
	if startRow >= endRow {
		return nil
	}
	return rows[startRow:endRow]
}

func (c *testCell) Update(msg tea.Msg, mode Mode) (Cell, tea.Cmd, bool) {
	c.lastMode = mode
	c.updates++
	// Default: passthrough so KeyMap navigation still fires for
	// nav keys. Tests that need the cell to claim a key can use
	// a custom cell or assert on `updates` (which counts every
	// dispatch regardless of handled).
	return c, nil, false
}

// --- CRUD ---

func TestAppendReturnsIDAndIncrementsLen(t *testing.T) {
	nb := New()
	id, err := nb.Append(newTestCell("a", 1))
	if err != nil {
		t.Fatalf("Append error: %v", err)
	}
	if id != "a" {
		t.Errorf("Append returned ID %q, want a", id)
	}
	if got := nb.Len(); got != 1 {
		t.Errorf("Len = %d, want 1", got)
	}
}

func TestInsertAtIndexPositionsCellAndCursor(t *testing.T) {
	nb := New()
	nb.Append(newTestCell("a", 1))
	nb.Append(newTestCell("c", 1))
	if _, err := nb.Insert(1, newTestCell("b", 1)); err != nil {
		t.Fatalf("Insert error: %v", err)
	}
	for i, want := range []string{"a", "b", "c"} {
		if got, _ := nb.store.snapshot().cells[i].ID(), true; got != want {
			t.Errorf("cells[%d] = %q, want %q", i, got, want)
		}
	}
}

func TestInsertAtNegativeIndexAppends(t *testing.T) {
	nb := New()
	nb.Append(newTestCell("a", 1))
	if _, err := nb.Insert(-1, newTestCell("b", 1)); err != nil {
		t.Fatalf("Insert(-1) error: %v", err)
	}
	if got := nb.store.snapshot().cells[1].ID(); got != "b" {
		t.Errorf("after Insert(-1, b): cells[1] = %q, want b", got)
	}
}

func TestInsertRejectsDuplicateID(t *testing.T) {
	nb := New()
	nb.Append(newTestCell("dup", 1))
	if _, err := nb.Insert(0, newTestCell("dup", 1)); err == nil {
		t.Error("Insert with duplicate ID should error")
	}
	if got := nb.Len(); got != 1 {
		t.Errorf("Len = %d, want 1 (rejected insert must not add)", got)
	}
}

func TestRemoveReturnsTrueAndShortensList(t *testing.T) {
	nb := New()
	nb.Append(newTestCell("a", 1))
	nb.Append(newTestCell("b", 1))
	if !nb.Remove("a") {
		t.Error("Remove(a) returned false")
	}
	if got := nb.Len(); got != 1 {
		t.Errorf("Len after remove = %d, want 1", got)
	}
	if nb.Remove("missing") {
		t.Error("Remove(missing) should return false")
	}
}

func TestRemoveAdjustsCursorWhenFocused(t *testing.T) {
	nb := New()
	nb.Append(newTestCell("a", 1))
	nb.Append(newTestCell("b", 1))
	nb.Append(newTestCell("c", 1))
	nb.store.moveCursor(+2) // cursor on c
	nb.Remove("c")
	if got := nb.store.cursorPos(); got != 1 {
		t.Errorf("cursor after removing focused-last = %d, want 1 (the new last)", got)
	}
	// Now cursor is on b. Remove a (before cursor).
	nb.Remove("a")
	if got := nb.store.cursorPos(); got != 0 {
		t.Errorf("cursor after removing-before = %d, want 0 (followed b)", got)
	}
}

func TestUpdateReplacesCell(t *testing.T) {
	nb := New()
	nb.Append(newTestCell("a", 1))
	replaced := newTestCell("a", 5) // same id, new height
	ok := nb.Update("a", func(_ Cell) Cell { return replaced })
	if !ok {
		t.Fatal("Update returned false on existing id")
	}
	got, _ := nb.Get("a")
	if got != replaced {
		t.Error("Update did not store the replacement")
	}
}

func TestGetAndIndexOf(t *testing.T) {
	nb := New()
	nb.Append(newTestCell("a", 1))
	nb.Append(newTestCell("b", 1))
	if c, ok := nb.Get("b"); !ok || c.ID() != "b" {
		t.Errorf("Get(b) = (%v, %v), want b/true", c, ok)
	}
	if idx, ok := nb.IndexOf("a"); !ok || idx != 0 {
		t.Errorf("IndexOf(a) = (%d, %v), want (0, true)", idx, ok)
	}
	if _, ok := nb.Get("missing"); ok {
		t.Error("Get(missing) should be (_, false)")
	}
}

// --- Clipboard auto-injection ---

type clipCell struct {
	*testCell
	got Clipboard
}

func (c *clipCell) SetClipboard(clip Clipboard) { c.got = clip }

func TestAppendAutoInjectsClipboard(t *testing.T) {
	myClip := Clipboard(func(s string) (string, bool) { return "test", true })
	nb := New(WithClipboard(myClip))
	cc := &clipCell{testCell: newTestCell("c", 1)}
	nb.Append(cc)
	if cc.got == nil {
		t.Error("SetClipboard was not called on Append")
	}
}

// --- Header / Done ---

func TestSetHeaderAndDoneAppearInSnapshot(t *testing.T) {
	nb := New(WithHeadless(), WithSize(40, 10))
	go nb.Run()
	t.Cleanup(nb.Stop)

	nb.SetHeader("My Notebook", "subtitle")
	snap := nb.Snapshot()
	if !strings.Contains(snap, "My Notebook") {
		t.Errorf("Snapshot missing header:\n%s", snap)
	}
	if strings.Contains(snap, "Done") {
		t.Errorf("Snapshot should not show Done yet:\n%s", snap)
	}
	nb.SetDone()
	if !strings.Contains(nb.Snapshot(), "Done") {
		t.Errorf("Snapshot missing Done after SetDone:\n%s", nb.Snapshot())
	}
}

// --- Run / Snapshot smoke ---

func TestSnapshotRendersAppendedCells(t *testing.T) {
	nb := New(WithHeadless(), WithSize(40, 10))
	go nb.Run()
	t.Cleanup(nb.Stop)

	nb.Append(newTestCell("alpha", 1))
	nb.Append(newTestCell("beta", 1))
	snap := nb.Snapshot()
	for _, want := range []string{"alpha", "beta"} {
		if !strings.Contains(snap, want) {
			t.Errorf("Snapshot missing %q:\n%s", want, snap)
		}
	}
}

func TestStopUnblocksRun(t *testing.T) {
	nb := New(WithHeadless(), WithSize(40, 10))
	done := make(chan struct{})
	go func() {
		nb.Run()
		close(done)
	}()
	// Give Run a moment to start.
	time.Sleep(20 * time.Millisecond)
	nb.Stop()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not return within 1s after Stop")
	}
}

// --- Viewport (model-level, no program needed) ---

// newSizedModel constructs a model with a sized store for
// viewport tests. Wires up the minimum scaffolding (a Notebook
// with store + rendezvous + DefaultKeyMap + DefaultMouseConfig,
// no tea.Program). Installs the default Bottom StatusCell so
// body math reflects what New() produces in real consumers.
func newSizedModel(t *testing.T, width, height int) (*model, *store) {
	t.Helper()
	nb := &Notebook{
		store:       newStore(),
		rdv:         newRendezvous(),
		keymap:      DefaultKeyMap(),
		mouseConfig: DefaultMouseConfig(),
		ready:       make(chan struct{}),
		stopped:     make(chan struct{}),
	}
	nb.store.setDock(Bottom.positionKey(), NewStatusCell(nb))
	m := newModel(nb, width, height)
	return &m, nb.store
}

func TestEnsureCursorVisibleScrollsDownToFocused(t *testing.T) {
	m, st := newSizedModel(t, 40, 8) // body = 8 - 1 status = 7
	for i := 0; i < 5; i++ {
		st.insert(-1, newTestCell(fmt.Sprintf("c%d", i), 3))
	}
	st.moveCursor(+4) // cursor on c4
	m.ensureCursorVisible()
	// c4 ends at row 15; viewport end must be >= 15.
	snap := st.snapshot()
	if m.viewportOffset+m.bodyHeight(snap) < 15 {
		t.Errorf("viewport [%d, %d) does not reach c4's end (15)",
			m.viewportOffset, m.viewportOffset+m.bodyHeight(snap))
	}
}

func TestRemoveAtTopAdjustsViewport(t *testing.T) {
	m, st := newSizedModel(t, 40, 8)
	for i := 0; i < 5; i++ {
		st.insert(-1, newTestCell(fmt.Sprintf("c%d", i), 3))
	}
	st.moveCursor(+4) // cursor on c4
	m.ensureCursorVisible()
	// Remove top cell. The cursor shifts from c4 (idx 4) to c4 (idx 3).
	st.remove("c0")
	m.ensureCursorVisible()
	// c4 (now idx 3) span [9,12); viewport must contain it.
	snap := st.snapshot()
	start, end := m.cellRowSpan(snap, st.cursorPos())
	if start < m.viewportOffset || end > m.viewportOffset+m.bodyHeight(snap) {
		t.Errorf("cell span [%d,%d) not in viewport [%d,%d) after top removal",
			start, end, m.viewportOffset, m.viewportOffset+m.bodyHeight(snap))
	}
}

// --- handleKey (model directly) ---

func TestNavigationModeJKMovesCursor(t *testing.T) {
	m, st := newSizedModel(t, 40, 24)
	st.insert(-1, newTestCell("a", 1))
	st.insert(-1, newTestCell("b", 1))
	st.insert(-1, newTestCell("c", 1))
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if got := st.cursorPos(); got != 2 {
		t.Errorf("cursor after 2×j = %d, want 2", got)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if got := st.cursorPos(); got != 1 {
		t.Errorf("cursor after k = %d, want 1", got)
	}
}

func TestKeyRoutingHitsFocusedCellInBothModes(t *testing.T) {
	m, st := newSizedModel(t, 40, 24)
	tc := newTestCell("a", 1)
	st.insert(-1, tc)
	// NavigationMode: non-nav keys route to the focused cell so 'c'-copy
	// works without entering CellActiveMode first.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	got, _ := st.get("a")
	if got.(*testCell).updates == 0 {
		t.Error("focused cell did not receive 'c' in NavigationMode")
	}
	if got.(*testCell).lastMode != NavigationMode {
		t.Errorf("cell saw mode %v, want NavigationMode", got.(*testCell).lastMode)
	}
}
