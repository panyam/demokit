package main

import (
	"strings"
	"testing"

	"github.com/panyam/demokit/notebook"
)

func TestResultCellSuccessShowsExprAndValue(t *testing.T) {
	c := NewResult("r", "2 + 2", "4", "")
	out := strings.Join(c.RenderRows(40, 0, c.HeightHint(40), false, notebook.ViewMode), "\n")
	for _, want := range []string{"2 + 2", "4"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in render, got:\n%s", want, out)
		}
	}
}

func TestResultCellErrorShowsArrowAndMessage(t *testing.T) {
	c := NewResult("r", "1/0", "", "division by zero")
	out := strings.Join(c.RenderRows(40, 0, c.HeightHint(40), false, notebook.ViewMode), "\n")
	if !strings.Contains(out, "1/0") {
		t.Errorf("expr missing:\n%s", out)
	}
	if !strings.Contains(out, "division by zero") {
		t.Errorf("error message missing:\n%s", out)
	}
	if !strings.Contains(out, "→") {
		t.Errorf("error arrow missing:\n%s", out)
	}
}
