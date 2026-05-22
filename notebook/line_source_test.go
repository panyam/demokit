package notebook

import (
	"strings"
	"testing"
)

// A logical line wider than the layout width wraps onto several
// visual rows; all content is preserved (no truncation) and no row
// exceeds the width.
func TestEagerLineSourceWrapsLongLine(t *testing.T) {
	buf := NewOutputBuffer()
	buf.Append([]byte(strings.Repeat("x", 200) + "\n"))
	src := NewEagerLineSource(buf)

	rows := src.Rows(40, 0, src.RowCount(40))
	if len(rows) < 5 {
		t.Fatalf("long line should wrap to several rows, got %d", len(rows))
	}
	if got := strings.Count(strings.Join(rows, ""), "x"); got != 200 {
		t.Fatalf("wrapping dropped content: %d x's, want 200", got)
	}
}

// Wrapping is ANSI-aware: a colored logical line that spills across
// rows keeps its SGR color on every wrapped row (color isn't lost
// after the first break, and escapes aren't split mid-sequence).
func TestEagerLineSourceWrapPreservesAnsiColor(t *testing.T) {
	const red = "\x1b[31m"
	buf := NewOutputBuffer()
	buf.Append([]byte(red + strings.Repeat("x", 200) + "\x1b[0m\n"))
	src := NewEagerLineSource(buf)

	rows := src.Rows(38, 0, src.RowCount(38))
	if len(rows) < 2 {
		t.Fatalf("expected the long line to wrap, got %d rows", len(rows))
	}
	for i, r := range rows {
		if strings.Contains(r, "x") && !strings.Contains(r, "\x1b[3") {
			t.Fatalf("wrapped row %d lost its color: %q", i, r)
		}
	}
}

// The in-flight partial line (no trailing '\n') is laid out like any
// other line, so sub-line streaming is visible immediately.
func TestEagerLineSourceShowsInFlightPartial(t *testing.T) {
	buf := NewOutputBuffer()
	buf.Append([]byte("streaming-token"))
	src := NewEagerLineSource(buf)

	rows := src.Rows(80, 0, src.RowCount(80))
	if strings.Join(rows, "") != "streaming-token" {
		t.Fatalf("partial line not laid out: %q", rows)
	}
}
