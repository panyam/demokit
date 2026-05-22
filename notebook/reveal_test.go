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
