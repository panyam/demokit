package notebook

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// sendKeyToCell pushes a key through the cell's Update and returns
// the updated cell. Convenience wrapper over the tea.KeyMsg shapes.
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

func choiceInput(name, prompt string, opts ...string) promptInput {
	return promptInput{
		Name:    name,
		Prompt:  prompt,
		Kind:    "choice",
		Options: opts,
		parse: func(s string) (any, error) {
			got := strings.TrimSpace(s)
			for _, o := range opts {
				if strings.EqualFold(got, o) {
					return o, nil
				}
			}
			return nil, fmt.Errorf("must be one of: %s", strings.Join(opts, ", "))
		},
	}
}

func intInput(name, prompt string, def int) promptInput {
	return promptInput{
		Name:    name,
		Prompt:  prompt,
		Default: def,
		Kind:    "int",
		parse: func(s string) (any, error) {
			n, err := strconv.Atoi(strings.TrimSpace(s))
			if err != nil {
				return nil, fmt.Errorf("not an integer: %q", s)
			}
			return n, nil
		},
	}
}

func TestPromptCellTabAdvancesActive(t *testing.T) {
	c := NewPromptCell("p", []promptInput{
		choiceInput("color", "Pick a color", "red", "blue"),
		intInput("age", "Your age", 0),
	}, nil, DefaultPalette())
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

func TestPromptCellSubmitsValidAnswers(t *testing.T) {
	reply := make(chan map[string]any, 1)
	inputs := []promptInput{
		choiceInput("color", "Color", "red", "blue"),
	}
	c := NewPromptCell("p", inputs, reply, DefaultPalette())
	// Type "red" into the textinput.
	for _, r := range "red" {
		c = sendKeyToCell(t, c, string(r)).(*PromptCell)
	}
	c = sendKeyToCell(t, c, "enter").(*PromptCell)
	if !c.done {
		t.Fatal("cell should be done after valid submit")
	}
	select {
	case got := <-reply:
		if got["color"] != "red" {
			t.Errorf("answers[color] = %v, want %q", got["color"], "red")
		}
	default:
		t.Fatal("reply channel never received")
	}
	// Channel should be closed after the send.
	if _, ok := <-reply; ok {
		t.Errorf("reply channel should be closed after submit")
	}
}

func TestPromptCellInvalidJumpsToFirstError(t *testing.T) {
	reply := make(chan map[string]any, 1)
	inputs := []promptInput{
		intInput("a", "A", 0),
		intInput("b", "B", 0),
	}
	c := NewPromptCell("p", inputs, reply, DefaultPalette())
	// Move to second field, type garbage there.
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
	select {
	case <-reply:
		t.Fatal("reply channel should not have received on invalid submit")
	default:
	}
}

func TestPromptCellUsesDefaultWhenEmpty(t *testing.T) {
	reply := make(chan map[string]any, 1)
	inputs := []promptInput{
		intInput("count", "Count", 42),
	}
	c := NewPromptCell("p", inputs, reply, DefaultPalette())
	c = sendKeyToCell(t, c, "enter").(*PromptCell)
	if !c.done {
		t.Fatal("cell should submit with default-filled value")
	}
	got := <-reply
	if got["count"] != 42 {
		t.Errorf("answers[count] = %v, want 42 (the default)", got["count"])
	}
}
