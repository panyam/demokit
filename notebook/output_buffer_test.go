package notebook

import (
	"reflect"
	"testing"
)

// The in-flight trailing line (bytes since the last '\n') is a
// visible logical line, not hidden until a newline commits it —
// so sub-line streaming surfaces immediately.
func TestOutputBufferSurfacesInFlightPartialLine(t *testing.T) {
	b := NewOutputBuffer()

	b.Append([]byte("Thinking"))
	if got := b.LineCount(); got != 1 {
		t.Fatalf("partial line: LineCount = %d, want 1", got)
	}
	if got := b.Lines(0, 1); !reflect.DeepEqual(got, []string{"Thinking"}) {
		t.Fatalf("partial line: Lines = %q, want [Thinking]", got)
	}

	b.Append([]byte("... done\n"))
	if got := b.LineCount(); got != 1 {
		t.Fatalf("after newline: LineCount = %d, want 1 (line finalized in place)", got)
	}
	if got := b.Lines(0, 1); !reflect.DeepEqual(got, []string{"Thinking... done"}) {
		t.Fatalf("after newline: Lines = %q, want [Thinking... done]", got)
	}

	b.Append([]byte("next"))
	if got := b.LineCount(); got != 2 {
		t.Fatalf("second partial: LineCount = %d, want 2", got)
	}
	if got := b.AllLines(); !reflect.DeepEqual(got, []string{"Thinking... done", "next"}) {
		t.Fatalf("AllLines = %q, want [Thinking... done next]", got)
	}
}
