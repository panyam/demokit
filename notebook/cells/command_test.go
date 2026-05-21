package cells

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/panyam/demokit/notebook"
)

// stripAnsi removes ANSI escape sequences from s. Tests assert on
// rendered text; the per-rune lipgloss styling embeds escape
// codes that would otherwise split substrings.
func stripAnsi(s string) string { return ansi.Strip(s) }

func TestCommandCell_ShortInputStaysOneRow(t *testing.T) {
	c := NewCommandCell(":", nil, nil)
	c.Update(runesKey("l"), notebook.CellActiveMode)
	c.Update(runesKey("s"), notebook.CellActiveMode)
	if got := c.HeightHint(80); got != 1 {
		t.Errorf("short input HeightHint = %d, want 1", got)
	}
	rows := c.RenderRows(80, 0, 1, true, notebook.CellActiveMode)
	if len(rows) != 1 || !strings.Contains(stripAnsi(rows[0]), "ls") {
		t.Errorf("RenderRows = %v, want one row containing 'ls'", rows)
	}
}

func TestCommandCell_LongInputWrapsAndGrowsHeight(t *testing.T) {
	c := NewCommandCell("$", nil, nil)
	// Width 10 → "$ " (2 chars) prefix + buffer + cursor. 25 chars
	// of buffer pushes past one row at width 10.
	for _, r := range strings.Repeat("a", 25) {
		c.Update(runesKey(string(r)), notebook.CellActiveMode)
	}
	h := c.HeightHint(10)
	if h < 3 {
		t.Errorf("HeightHint(10) = %d, want >=3 for 25-char buffer at width 10", h)
	}
	rows := c.RenderRows(10, 0, h, true, notebook.CellActiveMode)
	if len(rows) != h {
		t.Errorf("RenderRows returned %d rows, want %d", len(rows), h)
	}
}

func TestCommandCell_RenderWindowClippedToTail(t *testing.T) {
	c := NewCommandCell("$", nil, nil)
	for _, r := range strings.Repeat("x", 60) {
		c.Update(runesKey(string(r)), notebook.CellActiveMode)
	}
	h := c.HeightHint(10)
	// Ask for the last 2 rows only — the cell should hand them back.
	rows := c.RenderRows(10, h-2, h, true, notebook.CellActiveMode)
	if len(rows) != 2 {
		t.Fatalf("RenderRows tail = %d rows, want 2", len(rows))
	}
}

func TestCommandCell_EnterFiresOnSubmitAndReleases(t *testing.T) {
	var got string
	c := NewCommandCell(":", func(s string) { got = s }, nil)
	c.Update(runesKey("h"), notebook.CellActiveMode)
	c.Update(runesKey("i"), notebook.CellActiveMode)
	_, cmd, handled := c.Update(tea.KeyMsg{Type: tea.KeyEnter}, notebook.CellActiveMode)
	if !handled {
		t.Error("Enter not handled")
	}
	if cmd == nil {
		t.Fatal("Enter returned nil cmd")
	}
	sawRelease, sawSubmit := drainBatch(cmd)
	if !sawSubmit {
		t.Error("onSubmit not invoked from the Enter batch")
	}
	if got != "hi" {
		t.Errorf("onSubmit received %q, want %q", got, "hi")
	}
	if !sawRelease {
		t.Error("Enter batch missing ReleaseFocusMsg child")
	}
}

func TestCommandCell_EscFiresOnCancelAndReleases(t *testing.T) {
	cancelled := false
	c := NewCommandCell(":", nil, func() { cancelled = true })
	c.Update(runesKey("x"), notebook.CellActiveMode)
	_, cmd, handled := c.Update(tea.KeyMsg{Type: tea.KeyEsc}, notebook.CellActiveMode)
	if !handled {
		t.Error("Esc not handled")
	}
	if cmd == nil {
		t.Fatal("Esc returned nil cmd")
	}
	sawRelease, _ := drainBatch(cmd)
	if !cancelled {
		t.Error("onCancel not fired on Esc")
	}
	if !sawRelease {
		t.Error("Esc batch missing ReleaseFocusMsg child")
	}
	if c.Text() != "" {
		t.Errorf("buffer not cleared on Esc: %q", c.Text())
	}
}

// drainBatch invokes the cmd as if tea were dispatching it,
// recursively drains tea.BatchMsg children, and reports whether
// any branch yielded a ReleaseFocusMsg (sawRelease) and whether
// any branch yielded a nil msg from a non-empty Cmd (sawSubmit —
// the submit/cancel callback path returns nil after firing its
// side effect).
func drainBatch(cmd tea.Cmd) (sawRelease bool, sawSubmit bool) {
	if cmd == nil {
		return
	}
	msg := cmd()
	switch m := msg.(type) {
	case tea.BatchMsg:
		for _, child := range m {
			r, s := drainBatch(child)
			sawRelease = sawRelease || r
			sawSubmit = sawSubmit || s
		}
	case notebook.ReleaseFocusMsg:
		sawRelease = true
	case nil:
		sawSubmit = true
	}
	return
}

func TestCommandCell_BackspaceTrimsLastRune(t *testing.T) {
	c := NewCommandCell(":", nil, nil)
	c.Update(runesKey("a"), notebook.CellActiveMode)
	c.Update(runesKey("b"), notebook.CellActiveMode)
	c.Update(tea.KeyMsg{Type: tea.KeyBackspace}, notebook.CellActiveMode)
	if c.Text() != "a" {
		t.Errorf("after backspace, Text = %q, want %q", c.Text(), "a")
	}
}

func TestCommandCell_NavigationModeIgnoresKeys(t *testing.T) {
	c := NewCommandCell(":", nil, nil)
	_, _, handled := c.Update(runesKey("j"), notebook.NavigationMode)
	if handled {
		t.Error("CommandCell claimed key in NavigationMode — must passthrough so nav still works")
	}
}

func TestOpenCommandBar_InstallsAndFocuses(t *testing.T) {
	nb := notebook.New()
	notebook.NewStatusCell(nb) // satisfy import for tests
	OpenCommandBar(nb, ":", nil)
	got, ok := nb.DockedCell(notebook.Bottom)
	if !ok {
		t.Fatal("Bottom dock missing after OpenCommandBar")
	}
	if _, isCmd := got.(*CommandCell); !isCmd {
		t.Errorf("Bottom dock = %T, want *CommandCell", got)
	}
}

func TestOpenCommandBar_RestoresPriorDockOnEsc(t *testing.T) {
	nb := notebook.New()
	priorBefore, _ := nb.DockedCell(notebook.Bottom)
	OpenCommandBar(nb, ":", nil)
	// Esc returns a tea.Batch whose first child fires onCancel
	// (which calls restore) — drain it the same way tea would.
	cmdCell, _ := nb.DockedCell(notebook.Bottom)
	_, cmd, _ := cmdCell.Update(tea.KeyMsg{Type: tea.KeyEsc}, notebook.CellActiveMode)
	drainBatch(cmd)
	got, _ := nb.DockedCell(notebook.Bottom)
	if got != priorBefore {
		t.Errorf("after Esc, Bottom = %v, want prior instance %v", got, priorBefore)
	}
}

// runesKey is the common-case constructor for a single-rune key
// msg — every CommandCell input test wants this shape.
func runesKey(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}
