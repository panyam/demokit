package notebook

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// --- Registry: Set / Get / Clear ---

func TestDock_SetGetClearRoundtrip(t *testing.T) {
	nb := New()
	d := newTestCell("d", 1)
	nb.SetDockedCell(Top, d)
	got, ok := nb.DockedCell(Top)
	if !ok || got != d {
		t.Errorf("DockedCell(Top) = (%v, %v), want (d, true)", got, ok)
	}
	if !nb.ClearDocked(Top) {
		t.Error("ClearDocked(Top) returned false on existing dock")
	}
	if _, ok := nb.DockedCell(Top); ok {
		t.Error("DockedCell(Top) still reports present after Clear")
	}
}

func TestDock_DefaultBottomIsStatusCell(t *testing.T) {
	nb := New()
	got, ok := nb.DockedCell(Bottom)
	if !ok {
		t.Fatal("DockedCell(Bottom) not present after New — expected default StatusCell")
	}
	if _, isStatus := got.(*StatusCell); !isStatus {
		t.Errorf("default Bottom dock = %T, want *StatusCell", got)
	}
}

func TestDock_ClearBottomTrulyEmpties(t *testing.T) {
	nb := New()
	if !nb.ClearDocked(Bottom) {
		t.Fatal("ClearDocked(Bottom) returned false on default-installed dock")
	}
	if _, ok := nb.DockedCell(Bottom); ok {
		t.Error("Bottom dock still present after Clear — default should NOT auto-restore")
	}
}

func TestDock_ReplaceBottomWithCustomThenRestoreDefault(t *testing.T) {
	nb := New()
	custom := newTestCell("custom", 1)
	nb.SetDockedCell(Bottom, custom)
	if got, _ := nb.DockedCell(Bottom); got != custom {
		t.Errorf("after Set, Bottom = %v, want custom", got)
	}
	nb.SetDockedCell(Bottom, NewStatusCell(nb))
	if got, _ := nb.DockedCell(Bottom); got == custom {
		t.Error("after restoring default, Bottom still points at custom")
	}
}

// --- Cell-anchored docks: After / Before ---

func TestDock_AfterAndBeforeStoreDistinctly(t *testing.T) {
	nb := New()
	nb.Append(newTestCell("x", 1), RevealNone)
	a := newTestCell("a", 1)
	b := newTestCell("b", 1)
	nb.SetDockedCell(After("x"), a)
	nb.SetDockedCell(Before("x"), b)
	if got, _ := nb.DockedCell(After("x")); got != a {
		t.Errorf("After(x) = %v, want a", got)
	}
	if got, _ := nb.DockedCell(Before("x")); got != b {
		t.Errorf("Before(x) = %v, want b", got)
	}
}

func TestDock_AfterAutoUnregistersOnAnchorRemoval(t *testing.T) {
	nb := New()
	nb.Append(newTestCell("x", 1), RevealNone)
	nb.SetDockedCell(After("x"), newTestCell("a", 1))
	nb.SetDockedCell(Before("x"), newTestCell("b", 1))
	if !nb.Remove("x") {
		t.Fatal("Remove(x) returned false")
	}
	if _, ok := nb.DockedCell(After("x")); ok {
		t.Error("After(x) dock still present after anchor removal")
	}
	if _, ok := nb.DockedCell(Before("x")); ok {
		t.Error("Before(x) dock still present after anchor removal")
	}
}

func TestDock_RemoveOnlyAffectsMatchingAnchor(t *testing.T) {
	nb := New()
	nb.Append(newTestCell("x", 1), RevealNone)
	nb.Append(newTestCell("y", 1), RevealNone)
	keepA := newTestCell("ka", 1)
	keepB := newTestCell("kb", 1)
	nb.SetDockedCell(After("y"), keepA)
	nb.SetDockedCell(Before("y"), keepB)
	nb.SetDockedCell(After("x"), newTestCell("xa", 1))
	nb.Remove("x")
	if got, _ := nb.DockedCell(After("y")); got != keepA {
		t.Errorf("After(y) lost during Remove(x): got %v want %v", got, keepA)
	}
	if got, _ := nb.DockedCell(Before("y")); got != keepB {
		t.Errorf("Before(y) lost during Remove(x): got %v want %v", got, keepB)
	}
}

// --- bodyHeight & layout ---

func TestDock_TopReducesBodyHeight(t *testing.T) {
	m, st := newSizedModel(t, 40, 10)
	// Default has Bottom dock (1 row); body = 9.
	if got := m.bodyHeight(st.snapshot()); got != 9 {
		t.Fatalf("default bodyHeight = %d, want 9", got)
	}
	st.setDock(Top.positionKey(), newTestCell("top", 2))
	if got := m.bodyHeight(st.snapshot()); got != 7 {
		t.Errorf("with Top(h=2) bodyHeight = %d, want 7", got)
	}
}

func TestDock_BodyHeightClampsToOne(t *testing.T) {
	m, st := newSizedModel(t, 40, 3)
	st.setDock(Top.positionKey(), newTestCell("top", 5))
	if got := m.bodyHeight(st.snapshot()); got != 1 {
		t.Errorf("oversubscribed bodyHeight = %d, want 1 (clamp)", got)
	}
}

func TestDock_AfterAnchorAppearsInView(t *testing.T) {
	m, st := newSizedModel(t, 40, 12)
	st.insert(-1, newTestCell("x", 1))
	st.setDock((cellAnchor{rel: relAfter, cellID: "x"}).positionKey(), newTestCell("a", 1))
	out := m.View()
	if !strings.Contains(out, "x/0") {
		t.Errorf("View missing anchor x:\n%s", out)
	}
	if !strings.Contains(out, "a/0") {
		t.Errorf("View missing After(x) dock body 'a/0':\n%s", out)
	}
	// Order: x must appear before a in the rendered output.
	if strings.Index(out, "x/0") > strings.Index(out, "a/0") {
		t.Errorf("After(x) dock rendered BEFORE anchor:\n%s", out)
	}
}

func TestDock_BeforeAnchorAppearsBeforeCell(t *testing.T) {
	m, st := newSizedModel(t, 40, 12)
	st.insert(-1, newTestCell("x", 1))
	st.setDock((cellAnchor{rel: relBefore, cellID: "x"}).positionKey(), newTestCell("b", 1))
	out := m.View()
	if strings.Index(out, "b/0") > strings.Index(out, "x/0") {
		t.Errorf("Before(x) dock rendered AFTER anchor:\n%s", out)
	}
}

// --- Focus routing ---

func TestDock_FocusDockReceivesKeys(t *testing.T) {
	m, st := newSizedModel(t, 40, 12)
	main := newTestCell("main", 1)
	dock := newTestCell("dock", 1)
	st.insert(-1, main)
	st.setDock(Bottom.positionKey(), dock)

	// Simulate FocusDock: set focus key directly (no tea.Program in this helper).
	k := Bottom.positionKey()
	m.nb.dockFocusKey.Store(&k)

	// Pre-focus updates baseline.
	mainBefore := main.updates
	dockBefore := dock.updates
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("z")})

	if dock.updates == dockBefore {
		t.Error("dock cell did not receive key while focused")
	}
	if main.updates != mainBefore {
		t.Errorf("main cursor cell received key while dock focused (updates went %d → %d)",
			mainBefore, main.updates)
	}
}

func TestDock_FocusDockReturnsFalseWhenAbsent(t *testing.T) {
	nb := New()
	if nb.FocusDock(Top) {
		t.Error("FocusDock(Top) returned true with no Top dock installed")
	}
}

func TestDock_ReleaseFocusReturnsToMainList(t *testing.T) {
	m, st := newSizedModel(t, 40, 12)
	main := newTestCell("main", 1)
	dock := newTestCell("dock", 1)
	st.insert(-1, main)
	st.setDock(Bottom.positionKey(), dock)
	k := Bottom.positionKey()
	m.nb.dockFocusKey.Store(&k)

	// Emit ReleaseFocusMsg directly (cells return ReleaseFocus cmd
	// in real flows; we shortcut by sending the msg).
	m.Update(ReleaseFocusMsg{})

	if cur := m.nb.dockFocusKey.Load(); cur != nil {
		t.Errorf("dockFocusKey not cleared after ReleaseFocusMsg: %v", cur)
	}

	mainBefore := main.updates
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	if main.updates == mainBefore {
		t.Error("main cell did not receive key after release")
	}
}

func TestDock_CtrlWTogglesBottomFocus(t *testing.T) {
	nb := New()
	// Default Bottom dock present.
	cmd := ToggleBottomDockFocus(nb)
	if cmd == nil {
		t.Fatal("ToggleBottomDockFocus returned nil cmd; want CellActiveMode cmd")
	}
	if _, ok := cmd().(setModeMsg); !ok {
		t.Errorf("first toggle cmd msg type %T, want setModeMsg", cmd())
	}
	if cur := nb.dockFocusKey.Load(); cur == nil || *cur != Bottom.positionKey() {
		t.Errorf("after first toggle, dockFocusKey = %v, want Bottom", cur)
	}
	cmd2 := ToggleBottomDockFocus(nb)
	if cmd2 == nil {
		t.Fatal("second toggle returned nil cmd; want NavigationMode cmd")
	}
	if cur := nb.dockFocusKey.Load(); cur != nil {
		t.Errorf("after second toggle, dockFocusKey = %v, want nil", cur)
	}
}

func TestDock_CtrlWNoopWhenBottomCleared(t *testing.T) {
	nb := New()
	nb.ClearDocked(Bottom)
	cmd := ToggleBottomDockFocus(nb)
	if cmd != nil {
		t.Errorf("Ctrl+W with empty Bottom returned cmd %v, want nil", cmd)
	}
	if cur := nb.dockFocusKey.Load(); cur != nil {
		t.Error("Ctrl+W focused a non-existent Bottom dock")
	}
}

// --- StatusCell ---

func TestStatusCell_RendersLegacyFormat(t *testing.T) {
	nb := New()
	nb.Append(newTestCell("a", 1), RevealNone)
	nb.Append(newTestCell("b", 1), RevealNone)
	nb.Append(newTestCell("c", 1), RevealNone)
	cell, _ := nb.DockedCell(Bottom)
	rows := cell.RenderRows(40, 0, 1, false, NavigationMode)
	if len(rows) != 1 {
		t.Fatalf("RenderRows returned %d rows, want 1", len(rows))
	}
	want := "NAV  cell 1/3"
	if rows[0] != want {
		t.Errorf("StatusCell row = %q, want %q", rows[0], want)
	}
}

func TestStatusCell_EmptyShowsDash(t *testing.T) {
	nb := New()
	cell, _ := nb.DockedCell(Bottom)
	rows := cell.RenderRows(40, 0, 1, false, NavigationMode)
	if rows[0] != "NAV  cell —" {
		t.Errorf("empty StatusCell = %q, want 'NAV  cell —'", rows[0])
	}
}

// --- View height invariant ---

func TestDock_ViewHasExactHeight(t *testing.T) {
	m, st := newSizedModel(t, 40, 8)
	for i := 0; i < 3; i++ {
		st.insert(-1, newTestCell("c"+string(rune('0'+i)), 2))
	}
	st.setDock(Top.positionKey(), newTestCell("top", 1))
	out := m.View()
	got := strings.Count(out, "\n") + 1
	if got != 8 {
		t.Errorf("View produced %d rows, want 8 (h=8 with Top+Bottom)", got)
	}
}

// --- Auto-grow / clamp / tail behavior ---

// hugeCell reports HeightHint(width)=H regardless of width; used
// to exercise the layout clamp.
type hugeCell struct {
	*testCell
	h int
}

func (c *hugeCell) HeightHint(int) int { return c.h }
func (c *hugeCell) RenderRows(_ int, startRow, endRow int, _ bool, _ Mode) []string {
	rows := make([]string, 0, c.h)
	for i := 0; i < c.h; i++ {
		rows = append(rows, fmt.Sprintf("%s/%d", c.id, i))
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

func TestDock_BottomYieldsToBodyWhenOversubscribed(t *testing.T) {
	m, st := newSizedModel(t, 40, 10)
	st.insert(-1, newTestCell("main", 1))
	st.setDock(Bottom.positionKey(), &hugeCell{
		testCell: newTestCell("bot", 1),
		h:        100,
	})
	_, _, botH, bodyH := m.edgeAllotments(st.snapshot())
	if bodyH < 1 {
		t.Errorf("body starved (h=%d) — must always have >=1 row", bodyH)
	}
	if botH > 9 {
		t.Errorf("Bottom claimed %d rows of 10 — must yield at least 1 for body", botH)
	}
}

func TestDock_BottomRendersTailWhenClamped(t *testing.T) {
	m, st := newSizedModel(t, 40, 5)
	st.insert(-1, newTestCell("main", 1))
	st.setDock(Bottom.positionKey(), &hugeCell{
		testCell: newTestCell("bot", 1),
		h:        10,
	})
	out := m.View()
	// Tail rows are the highest-indexed; we expect "bot/9" (last row)
	// in the output and NOT "bot/0" (first row).
	if !strings.Contains(out, "bot/9") {
		t.Errorf("tail of clamped Bottom dock missing — expected 'bot/9' in:\n%s", out)
	}
	if strings.Contains(out, "bot/0") {
		t.Errorf("head of clamped Bottom dock rendered — expected tail-only:\n%s", out)
	}
}

func TestDock_TopRendersHeadWhenClamped(t *testing.T) {
	m, st := newSizedModel(t, 40, 5)
	st.insert(-1, newTestCell("main", 1))
	st.setDock(Top.positionKey(), &hugeCell{
		testCell: newTestCell("top", 1),
		h:        10,
	})
	out := m.View()
	if !strings.Contains(out, "top/0") {
		t.Errorf("head of clamped Top dock missing — expected 'top/0' in:\n%s", out)
	}
	if strings.Contains(out, "top/9") {
		t.Errorf("tail of clamped Top dock rendered — expected head-only:\n%s", out)
	}
}
