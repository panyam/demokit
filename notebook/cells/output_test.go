package cells

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/panyam/demokit/notebook"
)

func feed(c *OutputCell, lines ...string) {
	for _, l := range lines {
		c.Buffer().Append([]byte(l + "\n"))
		c.OnAppend()
	}
}

func TestOutputCellRendersBufferContent(t *testing.T) {
	c := NewOutput("o", 10)
	feed(c, "line one", "line two")
	joined := strings.Join(allRows(c, 80, false, notebook.ViewMode), "\n")
	for _, want := range []string{"line one", "line two"} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected %q in render, got:\n%s", want, joined)
		}
	}
}

func TestOutputCellEmptyShowsPlaceholder(t *testing.T) {
	c := NewOutput("o", 10)
	joined := strings.Join(allRows(c, 80, false, notebook.ViewMode), "\n")
	if !strings.Contains(joined, "(no output yet)") {
		t.Errorf("empty cell should show placeholder, got:\n%s", joined)
	}
}

func TestOutputCellHeightCappedByMaxBody(t *testing.T) {
	c := NewOutput("o", 5)
	for i := 0; i < 100; i++ {
		c.Buffer().Append([]byte("y\n"))
	}
	// 3 header/border + 5 body + 1 status = 9
	if got, want := c.HeightHint(80), 9; got != want {
		t.Errorf("HeightHint = %d, want %d (capped by maxBody=5)", got, want)
	}
}

func TestOutputCellAutoFollowsOnAppend(t *testing.T) {
	c := NewOutput("o", 3)
	c.Buffer().Append([]byte("a\n"))
	c.OnAppend()
	if c.scrollOffset != 0 {
		t.Errorf("1 line < maxBody: scrollOffset = %d, want 0", c.scrollOffset)
	}
	for i := 0; i < 10; i++ {
		c.Buffer().Append([]byte("x\n"))
	}
	c.OnAppend()
	if want := 11 - 3; c.scrollOffset != want {
		t.Errorf("auto-follow scrollOffset = %d, want %d", c.scrollOffset, want)
	}
}

func TestOutputCellManualScrollDisablesFollow(t *testing.T) {
	c := NewOutput("o", 3)
	for i := 0; i < 10; i++ {
		c.Buffer().Append([]byte("x\n"))
	}
	c.OnAppend()
	c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")}, notebook.ViewMode)
	if c.follow {
		t.Error("follow should be off after k")
	}
	frozen := c.scrollOffset
	for i := 0; i < 5; i++ {
		c.Buffer().Append([]byte("y\n"))
	}
	c.OnAppend()
	if c.scrollOffset != frozen {
		t.Errorf("follow off: OnAppend moved scrollOffset %d→%d", frozen, c.scrollOffset)
	}
	c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")}, notebook.ViewMode)
	if !c.follow {
		t.Error("follow should be on after G")
	}
}

func TestOutputCellRenderShowsLatestLinesWhenFollowing(t *testing.T) {
	c := NewOutput("o", 3)
	feed(c, "line1", "line2", "line3", "line4", "line5")
	joined := strings.Join(allRows(c, 80, false, notebook.ViewMode), "\n")
	for _, want := range []string{"line3", "line4", "line5"} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected latest line %q, got:\n%s", want, joined)
		}
	}
	for _, off := range []string{"line1", "line2"} {
		if strings.Contains(joined, off) {
			t.Errorf("earliest line %q should be scrolled off, got:\n%s", off, joined)
		}
	}
}

func TestOutputCellCopyInSelectMode(t *testing.T) {
	var got string
	c := NewOutput("o", 10)
	c.SetClipboard(captureClipboard(&got))
	feed(c, "hello", "world")
	c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")}, notebook.SelectMode)
	if got != "hello\nworld" {
		t.Errorf("clipboard = %q, want %q", got, "hello\nworld")
	}
	if c.copyMsg == "" {
		t.Error("expected copyMsg after copy")
	}
}

func TestOutputCellMarkDoneChangesState(t *testing.T) {
	c := NewOutput("o", 10)
	feed(c, "x")
	before := strings.Join(allRows(c, 80, false, notebook.ViewMode), "\n")
	c.MarkDone()
	after := strings.Join(allRows(c, 80, false, notebook.ViewMode), "\n")
	if before == after {
		t.Error("MarkDone produced no visible state change")
	}
}

func TestOutputBufferWriteImplementsIOWriter(t *testing.T) {
	buf := notebook.NewOutputBuffer()
	n, err := buf.Write([]byte("one\ntwo\n"))
	if err != nil || n != 8 {
		t.Fatalf("Write = (%d, %v), want (8, nil)", n, err)
	}
	if buf.LineCount() != 2 {
		t.Errorf("LineCount = %d, want 2", buf.LineCount())
	}
}
