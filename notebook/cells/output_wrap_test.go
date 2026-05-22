package cells

import (
	"strings"
	"testing"

	"github.com/panyam/demokit/notebook"
)

// A logical line wider than the box wraps onto several visual rows
// rather than being truncated: all of its content is rendered, and
// no rendered row is wider than the inner width.
func TestOutputCellWrapsLongLineInsteadOfTruncating(t *testing.T) {
	c := NewOutput("o", 100)
	c.Buffer().Append([]byte(strings.Repeat("x", 200) + "\n"))

	joined := strings.Join(allRows(c, 40, false, notebook.CellActiveMode), "\n")
	if got := strings.Count(joined, "x"); got != 200 {
		t.Fatalf("wrapped render has %d x's, want all 200 (truncation drops content)", got)
	}
}

// Bytes written without a trailing newline render immediately —
// the in-flight partial line is visible, not buffered out of sight
// until a '\n' arrives.
func TestOutputCellShowsInFlightPartialLine(t *testing.T) {
	c := NewOutput("o", 100)
	c.Buffer().Append([]byte("streaming-token"))

	joined := strings.Join(allRows(c, 80, false, notebook.CellActiveMode), "\n")
	if !strings.Contains(joined, "streaming-token") {
		t.Fatalf("partial line not rendered before newline:\n%s", joined)
	}
}
