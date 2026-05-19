package notebook

import (
	"strings"
	"testing"
)

func TestStringInputParsePassesThrough(t *testing.T) {
	in := NewStringInput("name", "Name?", nil)
	got, err := in.Parse("  alice  ")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if got != "  alice  " {
		t.Errorf("StringInput.Parse trimmed/changed input: got %q", got)
	}
}

func TestIntInputParseValidAndInvalid(t *testing.T) {
	in := NewIntInput("count", "Count?", nil)
	got, err := in.Parse(" 42 ")
	if err != nil {
		t.Fatalf("Parse(' 42 ') error: %v", err)
	}
	if got != 42 {
		t.Errorf("IntInput.Parse = %v (%T), want 42 (int)", got, got)
	}
	if _, err := in.Parse("nope"); err == nil {
		t.Error("IntInput.Parse('nope') should error")
	}
}

func TestChoiceInputParseIsCaseInsensitiveAndCanonical(t *testing.T) {
	in := NewChoiceInput("c", "Pick", nil, []string{"black", "sugar", "wild"})
	got, err := in.Parse("SUGAR")
	if err != nil {
		t.Fatalf("Parse('SUGAR') error: %v", err)
	}
	if got != "sugar" {
		t.Errorf("ChoiceInput.Parse = %v, want canonical 'sugar'", got)
	}
	if _, err := in.Parse("teal"); err == nil {
		t.Error("ChoiceInput.Parse('teal') should error — not an option")
	}
}

func TestChoiceInputHintListsOptions(t *testing.T) {
	in := NewChoiceInput("c", "Pick", nil, []string{"a", "b", "c"})
	hint := in.Hint()
	for _, want := range []string{"a", "b", "c", "options"} {
		if !strings.Contains(hint, want) {
			t.Errorf("Hint() = %q, missing %q", hint, want)
		}
	}
}

func TestStringAndIntInputsHaveNoHint(t *testing.T) {
	if h := NewStringInput("s", "S", nil).Hint(); h != "" {
		t.Errorf("StringInput.Hint() = %q, want empty", h)
	}
	if h := NewIntInput("i", "I", nil).Hint(); h != "" {
		t.Errorf("IntInput.Hint() = %q, want empty", h)
	}
}

func TestInputDefaultRoundTrips(t *testing.T) {
	if d := NewStringInput("s", "S", "x").InputDefault(); d != "x" {
		t.Errorf("StringInput default = %v, want x", d)
	}
	if d := NewIntInput("i", "I", 7).InputDefault(); d != 7 {
		t.Errorf("IntInput default = %v, want 7", d)
	}
	if d := NewStringInput("s", "S", nil).InputDefault(); d != nil {
		t.Errorf("nil default should round-trip as nil, got %v", d)
	}
}

// compile-time assertion that all three concrete types satisfy Input.
var _ = []Input{
	StringInput{},
	IntInput{},
	ChoiceInput{},
}
