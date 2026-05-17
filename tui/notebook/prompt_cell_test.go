package notebook

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/panyam/demokit/events"
)

// sendKeyToCell pushes a key through the cell's Update and
// returns the updated cell. Convenience wrapper.
func sendKeyToCell(t *testing.T, c Cell, key string) Cell {
	t.Helper()
	var km tea.KeyMsg
	switch key {
	case "enter":
		km = tea.KeyMsg{Type: tea.KeyEnter}
	case "tab":
		km = tea.KeyMsg{Type: tea.KeyTab}
	case "shift+tab":
		km = tea.KeyMsg{Type: tea.KeyShiftTab}
	default:
		km = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}
	got, _ := c.Update(km, ViewMode)
	return got
}

// makePromptCell sets up a queue + a PromptOpen event and
// constructs a PromptCell that resolves on submit. Returns the
// cell, the queue, and the prompt's queue offset.
func makePromptCell(t *testing.T, inputs ...events.Input) (*PromptCell, *events.EventQueue, int) {
	t.Helper()
	q := events.NewQueue()
	offset := q.Append(events.PromptOpen{Visit: 1, Inputs: inputs})
	cell := NewPromptCell("p", inputs, q, offset, DefaultPalette())
	return cell, q, offset
}

// resolutionOf reads the PromptOpen at offset and returns its
// resolution (or nil if unresolved). Helper for assertions.
func resolutionOf(t *testing.T, q *events.EventQueue, offset int) *events.PromptResolution {
	t.Helper()
	e, _ := q.ReadAt(offset)
	p, ok := e.(events.PromptOpen)
	if !ok {
		t.Fatalf("event at %d is %T, want PromptOpen", offset, e)
	}
	return p.Resolution
}

func TestPromptCellTabAdvancesActive(t *testing.T) {
	c, _, _ := makePromptCell(t,
		events.NewChoiceInput("color", "Pick a color", "red", []string{"red", "blue"}),
		events.NewIntInput("age", "Your age", 0),
	)
	if c.active != 0 {
		t.Fatalf("initial active = %d, want 0", c.active)
	}
	c = sendKeyToCell(t, c, "tab").(*PromptCell)
	if c.active != 1 {
		t.Errorf("after tab, active = %d, want 1", c.active)
	}
	c = sendKeyToCell(t, c, "shift+tab").(*PromptCell)
	if c.active != 0 {
		t.Errorf("after shift+tab, active = %d, want 0", c.active)
	}
}

func TestPromptCellSubmitsValidAnswersViaQueue(t *testing.T) {
	c, q, offset := makePromptCell(t,
		events.NewChoiceInput("color", "Color", "red", []string{"red", "blue"}),
	)
	for _, r := range "red" {
		c = sendKeyToCell(t, c, string(r)).(*PromptCell)
	}
	c = sendKeyToCell(t, c, "enter").(*PromptCell)
	if !c.done {
		t.Fatal("cell should be done after valid submit")
	}
	res := resolutionOf(t, q, offset)
	if res == nil {
		t.Fatal("PromptOpen.Resolution still nil after submit")
	}
	if res.Answers["color"] != "red" {
		t.Errorf("answers[color] = %v, want %q", res.Answers["color"], "red")
	}
	if res.Source != "user-submitted" {
		t.Errorf("Source = %q, want %q", res.Source, "user-submitted")
	}
}

func TestPromptCellInvalidJumpsToFirstError(t *testing.T) {
	c, q, offset := makePromptCell(t,
		events.NewIntInput("a", "A", 0),
		events.NewIntInput("b", "B", 0),
	)
	c = sendKeyToCell(t, c, "tab").(*PromptCell)
	for _, r := range "xyz" {
		c = sendKeyToCell(t, c, string(r)).(*PromptCell)
	}
	c = sendKeyToCell(t, c, "enter").(*PromptCell)
	if c.done {
		t.Fatal("cell should not be done — second field is invalid")
	}
	if c.active != 1 {
		t.Errorf("active should jump to invalid field 1; got %d", c.active)
	}
	if c.errors[1] == "" {
		t.Errorf("expected an error message on field 1")
	}
	if res := resolutionOf(t, q, offset); res != nil {
		t.Errorf("invalid submit should NOT resolve the queue; got %+v", res)
	}
}

func TestPromptCellUsesDefaultWhenEmpty(t *testing.T) {
	c, q, offset := makePromptCell(t,
		events.NewIntInput("count", "Count", 42),
	)
	c = sendKeyToCell(t, c, "enter").(*PromptCell)
	if !c.done {
		t.Fatal("cell should submit with default-filled value")
	}
	res := resolutionOf(t, q, offset)
	if res == nil {
		t.Fatal("Resolution nil after default-only submit")
	}
	if res.Answers["count"] != 42 {
		t.Errorf("answers[count] = %v, want 42 (the default)", res.Answers["count"])
	}
}
