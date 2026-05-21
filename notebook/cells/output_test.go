package cells

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/panyam/demokit/notebook"
)

// feed writes lines to the cell's buffer. The cell's render-time
// clampScroll handles follow-mode scrolling — no explicit "append
// happened" hook is needed.
func feed(c *OutputCell, lines ...string) {
	for _, l := range lines {
		c.Buffer().Append([]byte(l + "\n"))
	}
}

// render triggers a render so clampScroll runs and (when
// follow is on) scrollOffset settles to the buffer end. Tests
// that inspect scrollOffset directly call this before asserting.
func render(c *OutputCell) {
	_ = c.RenderRows(80, 0, c.HeightHint(80), false, notebook.ViewMode)
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

func TestOutputCellFollowsBufferEndOnRender(t *testing.T) {
	c := NewOutput("o", 3)
	c.Buffer().Append([]byte("a\n"))
	render(c)
	if c.scrollOffset != 0 {
		t.Errorf("1 line < maxBody: scrollOffset = %d, want 0", c.scrollOffset)
	}
	for i := 0; i < 10; i++ {
		c.Buffer().Append([]byte("x\n"))
	}
	render(c)
	if want := 11 - 3; c.scrollOffset != want {
		t.Errorf("follow scrollOffset = %d, want %d", c.scrollOffset, want)
	}
}

func TestOutputCellManualScrollDisablesFollow(t *testing.T) {
	c := NewOutput("o", 3)
	for i := 0; i < 10; i++ {
		c.Buffer().Append([]byte("x\n"))
	}
	render(c) // initial follow-driven scroll
	c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")}, notebook.ViewMode)
	if c.follow {
		t.Error("follow should be off after k")
	}
	frozen := c.scrollOffset
	for i := 0; i < 5; i++ {
		c.Buffer().Append([]byte("y\n"))
	}
	render(c)
	if c.scrollOffset != frozen {
		t.Errorf("follow off: render advanced scrollOffset %d→%d", frozen, c.scrollOffset)
	}
	c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")}, notebook.ViewMode)
	if !c.follow {
		t.Error("follow should be on after G")
	}
	render(c)
	if want := c.buf.LineCount() - 3; c.scrollOffset != want {
		t.Errorf("after G + render: scrollOffset = %d, want %d", c.scrollOffset, want)
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

func TestOutputCellSetMaxBodyShrinks(t *testing.T) {
	c := NewOutput("o", 12)
	for i := 0; i < 20; i++ {
		c.Buffer().Append([]byte("y\n"))
	}
	// HeightHint = 3 chrome + maxBody body + 1 status = maxBody+4.
	if got, want := c.HeightHint(80), 12+4; got != want {
		t.Fatalf("initial HeightHint = %d, want %d", got, want)
	}
	c.SetMaxBody(5)
	if got := c.MaxBody(); got != 5 {
		t.Errorf("MaxBody = %d after Set(5), want 5", got)
	}
	if got, want := c.HeightHint(80), 5+4; got != want {
		t.Errorf("HeightHint after shrink = %d, want %d", got, want)
	}
}

func TestOutputCellSetMaxBodyClampsScroll(t *testing.T) {
	c := NewOutput("o", 12)
	for i := 0; i < 30; i++ {
		c.Buffer().Append([]byte("y\n"))
	}
	// Follow drives scrollOffset to 30-12 = 18 at this point.
	_ = allRows(c, 80, false, notebook.ViewMode)
	c.SetMaxBody(4)
	// New ceiling: 30 - 4 = 26. scrollOffset was 18, still ≤ 26.
	// To exercise the clamp, scroll past the new max first:
	c.scrollOffset = 100
	c.SetMaxBody(4)
	if c.scrollOffset > 30-4 {
		t.Errorf("scrollOffset = %d after Set(4) with 30 lines; want ≤ %d", c.scrollOffset, 30-4)
	}
}

func TestOutputCellDefaultEdgesAreHorizontalOnly(t *testing.T) {
	c := NewOutput("o", 10)
	if got := c.Style.Edges; got != HorizontalEdges() {
		t.Errorf("default OutputStyle.Edges = %+v, want HorizontalEdges (top+bottom only)", got)
	}
	// Render should NOT contain '│' (left/right border char).
	feed(c, "hello")
	out := strings.Join(allRows(c, 80, false, notebook.ViewMode), "\n")
	if strings.Contains(out, "│") {
		t.Errorf("default OutputCell render contains '│' (L/R border) — should be HorizontalEdges:\n%s", out)
	}
}

func TestOutputCellEdgesAllShowsLeftRightBorders(t *testing.T) {
	c := NewOutput("o", 10)
	c.Style.Edges = AllEdges()
	feed(c, "hello")
	out := strings.Join(allRows(c, 80, false, notebook.ViewMode), "\n")
	if !strings.Contains(out, "│") {
		t.Errorf("AllEdges render missing '│' (L/R border):\n%s", out)
	}
}

func TestOutputCellWheelScrollsBodyInViewMode(t *testing.T) {
	c := NewOutput("o", 3)
	for i := 0; i < 20; i++ {
		c.Buffer().Append([]byte(fmt.Sprintf("line%d\n", i)))
	}
	render(c) // initial follow scroll to bottom: scrollOffset = 17
	before := c.scrollOffset
	_, _, handled := c.Update(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonWheelUp,
	}, notebook.ViewMode)
	if !handled {
		t.Fatal("wheel-up in ViewMode on overflowing buffer should be handled by the cell")
	}
	if c.follow {
		t.Error("wheel-up should turn follow off")
	}
	if c.scrollOffset != before-1 {
		t.Errorf("wheel-up scrollOffset = %d, want %d (one less)", c.scrollOffset, before-1)
	}
	_, _, _ = c.Update(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonWheelDown,
	}, notebook.ViewMode)
	if c.scrollOffset != before {
		t.Errorf("wheel-down scrollOffset = %d, want %d", c.scrollOffset, before)
	}
}

func TestOutputCellWheelPassthroughInSelectMode(t *testing.T) {
	c := NewOutput("o", 3)
	for i := 0; i < 20; i++ {
		c.Buffer().Append([]byte(fmt.Sprintf("line%d\n", i)))
	}
	render(c)
	_, _, handled := c.Update(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonWheelUp,
	}, notebook.SelectMode)
	if handled {
		t.Error("wheel in SelectMode should passthrough (notebook moves the cell cursor); click to activate first")
	}
}

func TestOutputCellWheelPassthroughWhenBufferFits(t *testing.T) {
	c := NewOutput("o", 12)
	feed(c, "line1", "line2", "line3") // 3 lines, maxBody 12 → fits
	_, _, handled := c.Update(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonWheelUp,
	}, notebook.ViewMode)
	if handled {
		t.Error("wheel on non-overflowing buffer should passthrough even in ViewMode")
	}
}

func TestOutputCellSetMaxBodyIgnoresNonPositive(t *testing.T) {
	c := NewOutput("o", 12)
	c.SetMaxBody(0)
	if c.MaxBody() != 12 {
		t.Errorf("SetMaxBody(0) changed MaxBody to %d; want unchanged 12", c.MaxBody())
	}
	c.SetMaxBody(-3)
	if c.MaxBody() != 12 {
		t.Errorf("SetMaxBody(-3) changed MaxBody to %d; want unchanged 12", c.MaxBody())
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
