package main

import (
	"math"
	"strings"
	"testing"

	"github.com/panyam/demokit/notebook"
)

func TestParsePlotAcceptsBareNumbers(t *testing.T) {
	c, err := NewPlotCell("p", "plot sin(x) from 0 to 6.28", NewEnv())
	if err != nil {
		t.Fatalf("NewPlotCell error: %v", err)
	}
	if c.expr != "sin(x)" {
		t.Errorf("expr = %q, want sin(x)", c.expr)
	}
	if math.Abs(c.b-6.28) > 1e-9 {
		t.Errorf("b = %v, want 6.28", c.b)
	}
}

func TestParsePlotAcceptsExpressionBounds(t *testing.T) {
	c, err := NewPlotCell("p", "plot cos(x) from 0 to pi*2", NewEnv())
	if err != nil {
		t.Fatalf("NewPlotCell error: %v", err)
	}
	if math.Abs(c.b-(2*math.Pi)) > 1e-9 {
		t.Errorf("b = %v, want 2π", c.b)
	}
}

func TestParsePlotRejectsBadSyntax(t *testing.T) {
	if _, err := NewPlotCell("p", "plot sin(x) over 0 6.28", NewEnv()); err == nil {
		t.Error("malformed plot should error")
	}
}

func TestPlotCellRendersBrailleContent(t *testing.T) {
	c, err := NewPlotCell("p", "plot sin(x) from 0 to 6.28", NewEnv())
	if err != nil {
		t.Fatalf("NewPlotCell error: %v", err)
	}
	rows := c.RenderRows(60, 0, c.HeightHint(60), false, notebook.CellActiveMode)
	joined := strings.Join(rows, "\n")
	if !strings.Contains(joined, "plot sin(x)") {
		t.Errorf("plot title missing:\n%s", joined)
	}
	// A braille char is in U+2800..U+28FF — at least one must
	// appear in the body for a sine wave.
	hasBraille := false
	for _, r := range joined {
		if r >= 0x2800 && r <= 0x28FF && r != 0x2800 {
			hasBraille = true
			break
		}
	}
	if !hasBraille {
		t.Errorf("no non-empty braille char in plot output:\n%s", joined)
	}
}
