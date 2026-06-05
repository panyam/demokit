package notebook

import (
	"fmt"
	"strings"
	"testing"
)

// RevealNone must not move the cursor (the viewport's follow anchor);
// RevealBottom must move it to the newly appended cell so the viewport
// follows it. This is the mechanism behind "streamed output stays in
// view" — see TestAppendRevealBottomScrollsIntoView for the rendered
// effect.
func TestAppendRevealControlsCursor(t *testing.T) {
	nb := New()
	for i := 0; i < 5; i++ {
		if _, err := nb.Append(newTestCell(fmt.Sprintf("f%d", i), 1), RevealNone); err != nil {
			t.Fatal(err)
		}
	}
	if nb.store.cursor != 0 {
		t.Fatalf("RevealNone moved the cursor to %d, want 0 (viewport must stay put)", nb.store.cursor)
	}

	id, err := nb.Append(newTestCell("followed", 1), RevealBottom)
	if err != nil {
		t.Fatal(err)
	}
	idx, _ := nb.IndexOf(id)
	if nb.store.cursor != idx {
		t.Fatalf("RevealBottom: cursor = %d, want %d (the new cell, so the viewport follows it)", nb.store.cursor, idx)
	}
}

// The rendered effect: with cells filled past the screen, a RevealNone
// append stays below the fold, while a RevealBottom append scrolls into
// view — the fix for "I appended/streamed a cell but it never shows up".
func TestAppendRevealBottomScrollsIntoView(t *testing.T) {
	nb := New(WithHeadless(), WithSize(40, 10))
	go nb.Run()
	defer nb.Stop()

	for i := 0; i < 30; i++ {
		nb.Append(newTestCell(fmt.Sprintf("f%d", i), 1), RevealNone)
	}
	nb.Append(newTestCell("hidden", 1), RevealNone)
	if strings.Contains(nb.Snapshot(), "hidden/") {
		t.Fatal("RevealNone cell should be below the fold, not rendered")
	}

	nb.Append(newTestCell("shown", 1), RevealBottom)
	if !strings.Contains(nb.Snapshot(), "shown/") {
		t.Fatalf("RevealBottom cell should scroll into view:\n%s", nb.Snapshot())
	}
}

// TestAppendRevealTopPositionsAtTop verifies a RevealTop append
// scrolls so the new cell sits at the top of the body viewport.
// Fill enough cells to push the body well past one screen, then
// append "target" with RevealTop; assert it lands on the first body
// row.
func TestAppendRevealTopPositionsAtTop(t *testing.T) {
	nb := New(WithHeadless(), WithSize(40, 10))
	go nb.Run()
	defer nb.Stop()

	for i := 0; i < 30; i++ {
		nb.Append(newTestCell(fmt.Sprintf("f%d", i), 1), RevealNone)
	}
	nb.Append(newTestCell("target", 1), RevealTop)

	snap := nb.Snapshot()
	if !strings.Contains(snap, "target/") {
		t.Fatalf("RevealTop cell should be visible:\n%s", snap)
	}
	// Body-content rows start after the header. Find the first row
	// that contains a cell-rendered "fN/" or "target/" — that's the
	// top of the body. RevealTop must put "target/" there, not any
	// earlier cell.
	lines := strings.Split(snap, "\n")
	var firstBody string
	for _, ln := range lines {
		if strings.Contains(ln, "/") && !strings.Contains(ln, "·") {
			firstBody = ln
			break
		}
	}
	if !strings.Contains(firstBody, "target/") {
		t.Errorf("RevealTop: first body row should hold target, got %q (full snapshot:\n%s)",
			firstBody, snap)
	}
}

// TestAppendRevealMiddleCenters verifies a RevealMiddle append
// scrolls so the new cell sits approximately at the vertical
// center of the body. The exact center row depends on body
// height; we check the cell's row index is in the central band.
func TestAppendRevealMiddleCenters(t *testing.T) {
	nb := New(WithHeadless(), WithSize(40, 12))
	go nb.Run()
	defer nb.Stop()

	for i := 0; i < 30; i++ {
		nb.Append(newTestCell(fmt.Sprintf("f%d", i), 1), RevealNone)
	}
	nb.Append(newTestCell("target", 1), RevealMiddle)

	snap := nb.Snapshot()
	if !strings.Contains(snap, "target/") {
		t.Fatalf("RevealMiddle cell should be visible:\n%s", snap)
	}
	lines := strings.Split(snap, "\n")
	bodyRows := []int{}
	targetRow := -1
	for i, ln := range lines {
		if strings.Contains(ln, "/") && !strings.Contains(ln, "·") {
			bodyRows = append(bodyRows, i)
			if strings.Contains(ln, "target/") {
				targetRow = i
			}
		}
	}
	if len(bodyRows) == 0 {
		t.Fatalf("no body rows in snapshot:\n%s", snap)
	}
	bodyTop, bodyBottom := bodyRows[0], bodyRows[len(bodyRows)-1]
	mid := (bodyTop + bodyBottom) / 2
	// Allow a 2-row band around exact mid for body-height parity
	// and the cell occupying multiple rows.
	if targetRow < mid-2 || targetRow > mid+2 {
		t.Errorf("RevealMiddle: target row %d should be near mid %d (bodyTop=%d, bodyBottom=%d)\n%s",
			targetRow, mid, bodyTop, bodyBottom, snap)
	}
}

// TestRevealTopIsOneShotNotTailFollow verifies the user-facing
// difference between RevealTop and RevealBottom: subsequent
// RevealNone appends do NOT pull the viewport. RevealBottom would
// (cursor sits at the streaming cell, ensureCursorVisible drags
// the viewport down each frame). RevealTop pins the cursor on the
// aligned cell, so ensureCursorVisible is a no-op while that cell
// stays in view — the viewport stays put.
//
// The load-bearing assertion: the anchor cell remains at the top
// body row after a later RevealNone append. If the viewport had
// followed, anchor would scroll off the top.
func TestRevealTopIsOneShotNotTailFollow(t *testing.T) {
	nb := New(WithHeadless(), WithSize(40, 10))
	go nb.Run()
	defer nb.Stop()

	for i := 0; i < 30; i++ {
		nb.Append(newTestCell(fmt.Sprintf("f%d", i), 1), RevealNone)
	}
	nb.Append(newTestCell("anchor", 1), RevealTop)
	nb.Append(newTestCell("later", 1), RevealNone)

	snap := nb.Snapshot()
	lines := strings.Split(snap, "\n")
	var firstBody string
	for _, ln := range lines {
		if strings.Contains(ln, "/") && !strings.Contains(ln, "·") {
			firstBody = ln
			break
		}
	}
	if !strings.Contains(firstBody, "anchor/") {
		t.Errorf("RevealTop should pin anchor at the top body row; a later RevealNone append moved viewport. firstBody=%q\nfull snapshot:\n%s",
			firstBody, snap)
	}
}

// TestAlignCellTopMovesExistingCellToTop verifies the
// post-insertion alignment primitive: an existing cell, far from
// the cursor, can be brought to the top of the viewport by
// AlignCell(id, RevealTop). The cursor moves to the aligned cell
// — same shape as the Insert-time RevealTop path.
func TestAlignCellTopMovesExistingCellToTop(t *testing.T) {
	nb := New(WithHeadless(), WithSize(40, 10))
	go nb.Run()
	defer nb.Stop()

	for i := 0; i < 30; i++ {
		nb.Append(newTestCell(fmt.Sprintf("f%d", i), 1), RevealNone)
	}
	if ok := nb.AlignCell("f25", RevealTop); !ok {
		t.Fatalf("AlignCell should return true for existing cell")
	}

	snap := nb.Snapshot()
	if !strings.Contains(snap, "f25/") {
		t.Fatalf("AlignCell(Top) should bring f25 into view:\n%s", snap)
	}
	lines := strings.Split(snap, "\n")
	var firstBody string
	for _, ln := range lines {
		if strings.Contains(ln, "/") && !strings.Contains(ln, "·") {
			firstBody = ln
			break
		}
	}
	if !strings.Contains(firstBody, "f25/") {
		t.Errorf("AlignCell(Top): first body row should hold f25, got %q\nfull snapshot:\n%s",
			firstBody, snap)
	}
}

// TestAlignCellMiddleCenters verifies AlignCell(id, RevealMiddle)
// places the target cell near the vertical midpoint of the body.
func TestAlignCellMiddleCenters(t *testing.T) {
	nb := New(WithHeadless(), WithSize(40, 12))
	go nb.Run()
	defer nb.Stop()

	for i := 0; i < 30; i++ {
		nb.Append(newTestCell(fmt.Sprintf("f%d", i), 1), RevealNone)
	}
	if ok := nb.AlignCell("f20", RevealMiddle); !ok {
		t.Fatalf("AlignCell should return true for existing cell")
	}

	snap := nb.Snapshot()
	if !strings.Contains(snap, "f20/") {
		t.Fatalf("AlignCell(Middle) should bring f20 into view:\n%s", snap)
	}
	lines := strings.Split(snap, "\n")
	bodyRows := []int{}
	targetRow := -1
	for i, ln := range lines {
		if strings.Contains(ln, "/") && !strings.Contains(ln, "·") {
			bodyRows = append(bodyRows, i)
			if strings.Contains(ln, "f20/") {
				targetRow = i
			}
		}
	}
	if len(bodyRows) == 0 {
		t.Fatalf("no body rows in snapshot:\n%s", snap)
	}
	bodyTop, bodyBottom := bodyRows[0], bodyRows[len(bodyRows)-1]
	mid := (bodyTop + bodyBottom) / 2
	if targetRow < mid-2 || targetRow > mid+2 {
		t.Errorf("AlignCell(Middle): target row %d should be near mid %d (bodyTop=%d, bodyBottom=%d)\n%s",
			targetRow, mid, bodyTop, bodyBottom, snap)
	}
}

// TestAlignCellBottomPositionsAtBottom verifies AlignCell with
// RevealBottom places the target cell at the bottom of the body
// viewport (matching Insert's RevealBottom semantics).
func TestAlignCellBottomPositionsAtBottom(t *testing.T) {
	nb := New(WithHeadless(), WithSize(40, 10))
	go nb.Run()
	defer nb.Stop()

	for i := 0; i < 30; i++ {
		nb.Append(newTestCell(fmt.Sprintf("f%d", i), 1), RevealNone)
	}
	if ok := nb.AlignCell("f10", RevealBottom); !ok {
		t.Fatalf("AlignCell should return true for existing cell")
	}

	snap := nb.Snapshot()
	if !strings.Contains(snap, "f10/") {
		t.Fatalf("AlignCell(Bottom) should bring f10 into view:\n%s", snap)
	}
	lines := strings.Split(snap, "\n")
	var lastBody string
	for _, ln := range lines {
		if isBodyRow(ln) {
			lastBody = ln
		}
	}
	if !strings.Contains(lastBody, "f10/") {
		t.Errorf("AlignCell(Bottom): last body row should hold f10, got %q\nfull snapshot:\n%s",
			lastBody, snap)
	}
}

// isBodyRow reports whether a snapshot line is a rendered cell
// row (not header, not status). Tests use it to scan snapshots
// without picking up the "NAV cell N/M" status footer that also
// contains a `/`.
func isBodyRow(ln string) bool {
	return strings.Contains(ln, "/") &&
		!strings.Contains(ln, "·") &&
		!strings.Contains(ln, "NAV") &&
		!strings.Contains(ln, "INSERT") &&
		!strings.Contains(ln, "cell ")
}

// TestAlignCellMissingIDReturnsFalse verifies AlignCell on a
// non-existent ID is a no-op and reports false. The cursor and
// viewport must not move.
func TestAlignCellMissingIDReturnsFalse(t *testing.T) {
	nb := New(WithHeadless(), WithSize(40, 10))
	go nb.Run()
	defer nb.Stop()

	for i := 0; i < 5; i++ {
		nb.Append(newTestCell(fmt.Sprintf("f%d", i), 1), RevealNone)
	}
	cursorBefore := nb.store.cursor
	if ok := nb.AlignCell("does-not-exist", RevealTop); ok {
		t.Errorf("AlignCell with unknown ID should return false")
	}
	if nb.store.cursor != cursorBefore {
		t.Errorf("AlignCell with unknown ID should not move cursor (was %d, now %d)",
			cursorBefore, nb.store.cursor)
	}
}
