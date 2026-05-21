package cells

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/panyam/demokit/notebook"
)

func TestAdvancePromptRendersLabel(t *testing.T) {
	c := NewAdvance("a", "Press Enter to continue")
	out := strings.Join(allRows(c, 60, false, notebook.NavigationMode), "\n")
	if !strings.Contains(out, "Press Enter to continue") {
		t.Errorf("render missing label:\n%s", out)
	}
}

func TestAdvancePromptDefaultsLabelWhenEmpty(t *testing.T) {
	c := NewAdvance("a", "")
	out := strings.Join(allRows(c, 60, false, notebook.NavigationMode), "\n")
	if !strings.Contains(out, "Press Enter to continue") {
		t.Errorf("empty label should default; got:\n%s", out)
	}
}

func TestAdvancePromptEnterEmitsSubmittedUserSource(t *testing.T) {
	c := NewAdvance("a", "")
	_, cmd, handled := c.Update(tea.KeyMsg{Type: tea.KeyEnter}, notebook.CellActiveMode)
	if !handled {
		t.Fatal("Enter should be handled")
	}
	if cmd == nil {
		t.Fatal("Enter should emit a cmd")
	}
	msg := cmd()
	sub, ok := msg.(notebook.PromptSubmittedMsg)
	if !ok {
		t.Fatalf("cmd produced %T, want PromptSubmittedMsg", msg)
	}
	if sub.CellID != "a" {
		t.Errorf("CellID = %q, want a", sub.CellID)
	}
	if sub.Source != "user-submitted" {
		t.Errorf("Source = %q, want user-submitted", sub.Source)
	}
	if sub.Answers != nil {
		t.Errorf("Answers = %v, want nil", sub.Answers)
	}
}

func TestAdvancePromptEscReleasesFocus(t *testing.T) {
	c := NewAdvance("a", "")
	_, cmd, handled := c.Update(tea.KeyMsg{Type: tea.KeyEsc}, notebook.CellActiveMode)
	if !handled {
		t.Fatal("Esc should be handled")
	}
	if cmd == nil {
		t.Fatal("Esc should return a cmd (ReleaseFocus)")
	}
	if msg := cmd(); msg != (notebook.ReleaseFocusMsg{}) {
		t.Errorf("Esc emitted %v, want ReleaseFocusMsg{}", msg)
	}
}

func TestAdvancePromptOtherKeysPassthrough(t *testing.T) {
	c := NewAdvance("a", "")
	_, _, handled := c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}, notebook.CellActiveMode)
	if handled {
		t.Error("'j' should passthrough (handled=false) so notebook KeyMap can act")
	}
}

func TestAdvancePromptIgnoresKeysInNavigationMode(t *testing.T) {
	c := NewAdvance("a", "")
	_, _, handled := c.Update(tea.KeyMsg{Type: tea.KeyEnter}, notebook.NavigationMode)
	if handled {
		t.Error("Enter in NavigationMode should passthrough — cell only handles in CellActiveMode")
	}
}

func TestAdvancePromptDeadlineSchedulesAutoAdvance(t *testing.T) {
	c := NewAdvance("a", "")
	c.Deadline = time.Now().Add(20 * time.Millisecond)
	// First Update of any kind schedules the timer. Send a
	// non-key msg so the schedule cmd is the only return.
	_, cmd, _ := c.Update(struct{}{}, notebook.CellActiveMode)
	if cmd == nil {
		t.Fatal("Deadline-set cell did not schedule a tick on first Update")
	}
	// Wait for the tick + a small buffer.
	time.Sleep(40 * time.Millisecond)
	msg := cmd()
	sub, ok := msg.(notebook.PromptSubmittedMsg)
	if !ok {
		t.Fatalf("scheduled cmd produced %T, want PromptSubmittedMsg", msg)
	}
	if sub.Source != "auto-advance" {
		t.Errorf("Source = %q, want auto-advance", sub.Source)
	}
}

func TestAdvancePromptDeadlineRendersBar(t *testing.T) {
	c := NewAdvance("a", "")
	c.Deadline = time.Now().Add(5 * time.Second)
	out := strings.Join(allRows(c, 60, false, notebook.CellActiveMode), "\n")
	// Bar uses U+2588 (full block) and U+2591 (light shade).
	if !strings.ContainsAny(out, "█░") {
		t.Errorf("countdown bar missing from render:\n%s", out)
	}
	// Seconds suffix.
	if !strings.Contains(out, "s") {
		t.Errorf("countdown seconds suffix missing:\n%s", out)
	}
}

func TestAdvancePromptDoneIgnoresFurtherKeys(t *testing.T) {
	c := NewAdvance("a", "")
	// First Enter — accepted.
	if _, _, handled := c.Update(tea.KeyMsg{Type: tea.KeyEnter}, notebook.CellActiveMode); !handled {
		t.Fatal("first Enter should be handled")
	}
	// Second Enter — cell is done; should passthrough.
	if _, _, handled := c.Update(tea.KeyMsg{Type: tea.KeyEnter}, notebook.CellActiveMode); handled {
		t.Error("second Enter on done cell should passthrough")
	}
}
