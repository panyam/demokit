package cells

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/panyam/demokit/notebook"
)

// submitMsg drives the cell's Enter handler and returns the
// PromptSubmittedMsg if one was emitted, or nil.
func submitMsg(t *testing.T, c *PromptCell) *notebook.PromptSubmittedMsg {
	t.Helper()
	_, cmd, _ := c.Update(tea.KeyMsg{Type: tea.KeyEnter}, notebook.ViewMode)
	if cmd == nil {
		return nil
	}
	msg := cmd()
	sub, ok := msg.(notebook.PromptSubmittedMsg)
	if !ok {
		t.Fatalf("emitted cmd produced %T, want notebook.PromptSubmittedMsg", msg)
	}
	return &sub
}

func TestPromptCellSubmitEmitsParsedAnswers(t *testing.T) {
	c := NewPrompt("p", []notebook.Input{
		notebook.NewStringInput("name", "Name?", nil),
		notebook.NewIntInput("count", "Count?", nil),
	})
	c.fields[0].SetValue("alice")
	c.fields[1].SetValue("42")

	sub := submitMsg(t, c)
	if sub == nil {
		t.Fatal("valid submit emitted no PromptSubmittedMsg")
	}
	if sub.CellID != "p" {
		t.Errorf("CellID = %q, want p", sub.CellID)
	}
	if sub.Answers["name"] != "alice" {
		t.Errorf("name = %v, want alice", sub.Answers["name"])
	}
	if sub.Answers["count"] != 42 {
		t.Errorf("count = %v (%T), want 42 (int)", sub.Answers["count"], sub.Answers["count"])
	}
	if !c.done {
		t.Error("cell should be done after successful submit")
	}
}

func TestPromptCellInvalidIntBlocksSubmit(t *testing.T) {
	c := NewPrompt("p", []notebook.Input{
		notebook.NewIntInput("count", "Count?", nil),
	})
	c.fields[0].SetValue("not-a-number")

	if sub := submitMsg(t, c); sub != nil {
		t.Errorf("invalid int should block submit; got %+v", sub)
	}
	if c.errors[0] == "" {
		t.Error("expected an error recorded for the invalid field")
	}
	if c.done {
		t.Error("cell should not be done after failed submit")
	}
}

func TestPromptCellEmptySubmitUsesDefault(t *testing.T) {
	c := NewPrompt("p", []notebook.Input{
		notebook.NewStringInput("name", "Name?", "default-name"),
	})
	// field left empty
	sub := submitMsg(t, c)
	if sub == nil {
		t.Fatal("empty submit with default should succeed")
	}
	if sub.Answers["name"] != "default-name" {
		t.Errorf("name = %v, want default-name", sub.Answers["name"])
	}
}

func TestPromptCellChoiceIsCaseInsensitive(t *testing.T) {
	c := NewPrompt("p", []notebook.Input{
		notebook.NewChoiceInput("button", "Pick", nil, []string{"black", "sugar", "wild"}),
	})
	c.fields[0].SetValue("BLACK")

	sub := submitMsg(t, c)
	if sub == nil {
		t.Fatal("case-insensitive choice should parse")
	}
	if sub.Answers["button"] != "black" {
		t.Errorf("button = %v, want canonical 'black'", sub.Answers["button"])
	}
}

func TestPromptCellChoiceRejectsUnknownOption(t *testing.T) {
	c := NewPrompt("p", []notebook.Input{
		notebook.NewChoiceInput("button", "Pick", nil, []string{"black", "sugar"}),
	})
	c.fields[0].SetValue("teal")

	if sub := submitMsg(t, c); sub != nil {
		t.Errorf("unknown choice should block submit; got %+v", sub)
	}
	if c.errors[0] == "" {
		t.Error("expected error for unknown choice")
	}
}

func TestPromptCellTabCyclesActiveField(t *testing.T) {
	c := NewPrompt("p", []notebook.Input{
		notebook.NewStringInput("a", "A", nil),
		notebook.NewStringInput("b", "B", nil),
		notebook.NewStringInput("c", "C", nil),
	})
	if c.active != 0 {
		t.Fatalf("initial active = %d, want 0", c.active)
	}
	c.Update(tea.KeyMsg{Type: tea.KeyTab}, notebook.ViewMode)
	if c.active != 1 {
		t.Errorf("after Tab: active = %d, want 1", c.active)
	}
	c.Update(tea.KeyMsg{Type: tea.KeyTab}, notebook.ViewMode)
	c.Update(tea.KeyMsg{Type: tea.KeyTab}, notebook.ViewMode) // wrap
	if c.active != 0 {
		t.Errorf("Tab wrap: active = %d, want 0", c.active)
	}
	c.Update(tea.KeyMsg{Type: tea.KeyShiftTab}, notebook.ViewMode)
	if c.active != 2 {
		t.Errorf("Shift+Tab from 0: active = %d, want 2", c.active)
	}
}

func TestPromptCellInvalidFieldBecomesActive(t *testing.T) {
	c := NewPrompt("p", []notebook.Input{
		notebook.NewStringInput("name", "Name?", nil),
		notebook.NewIntInput("count", "Count?", nil),
	})
	c.fields[0].SetValue("ok")
	c.fields[1].SetValue("bad")
	// active is field 0; submit should jump active to the invalid field 1
	submitMsg(t, c)
	if c.active != 1 {
		t.Errorf("after failed submit: active = %d, want 1 (the invalid field)", c.active)
	}
}

func TestPromptCellIgnoresKeysInSelectMode(t *testing.T) {
	c := NewPrompt("p", []notebook.Input{
		notebook.NewStringInput("a", "A", nil),
		notebook.NewStringInput("b", "B", nil),
	})
	c.Update(tea.KeyMsg{Type: tea.KeyTab}, notebook.SelectMode)
	if c.active != 0 {
		t.Errorf("Tab in SelectMode should be ignored; active = %d", c.active)
	}
}
