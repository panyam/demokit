package notebook

import (
	"testing"
	"time"
)

func TestOutputBufferAppendSplitsOnNewline(t *testing.T) {
	b := NewOutputBuffer()
	b.Append([]byte("hello\nworld\n"))
	if got, want := b.LineCount(), 2; got != want {
		t.Fatalf("LineCount = %d, want %d", got, want)
	}
	lines := b.Lines(0, 2)
	if len(lines) != 2 || lines[0] != "hello" || lines[1] != "world" {
		t.Errorf("Lines = %v, want [hello world]", lines)
	}
}

func TestOutputBufferHoldsPendingLine(t *testing.T) {
	b := NewOutputBuffer()
	b.Append([]byte("partial"))
	if got, want := b.LineCount(), 0; got != want {
		t.Errorf("partial line should not be counted; LineCount = %d, want %d", got, want)
	}
	b.Append([]byte(" more\n"))
	if got, want := b.LineCount(), 1; got != want {
		t.Fatalf("LineCount after completion = %d, want %d", got, want)
	}
	if got, want := b.Lines(0, 1)[0], "partial more"; got != want {
		t.Errorf("joined line = %q, want %q", got, want)
	}
}

func TestOutputBufferLinesClampsRange(t *testing.T) {
	b := NewOutputBuffer()
	b.Append([]byte("a\nb\nc\n"))

	tests := []struct {
		start, end int
		want       []string
	}{
		{0, 3, []string{"a", "b", "c"}},
		{1, 2, []string{"b"}},
		{-5, 1, []string{"a"}},     // negative start clamps to 0
		{2, 100, []string{"c"}},    // over-end clamps
		{3, 5, nil},                // entirely past end
		{2, 1, nil},                // inverted range
	}
	for _, tt := range tests {
		got := b.Lines(tt.start, tt.end)
		if !equalStringSlice(got, tt.want) {
			t.Errorf("Lines(%d, %d) = %v, want %v", tt.start, tt.end, got, tt.want)
		}
	}
}

func TestOutputBufferWakesSubscriberOnNewline(t *testing.T) {
	b := NewOutputBuffer()
	ch := b.Subscribe()
	b.Append([]byte("line one\n"))
	select {
	case <-ch:
		// expected
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Subscribe channel did not wake after newline")
	}
}

func TestOutputBufferDoesNotWakeOnPartialLine(t *testing.T) {
	b := NewOutputBuffer()
	ch := b.Subscribe()
	b.Append([]byte("no newline yet"))
	select {
	case <-ch:
		t.Fatal("Subscribe channel woke on partial-line append; should only wake on committed line")
	case <-time.After(20 * time.Millisecond):
		// expected — no wakeup
	}
}

func TestOutputBufferCoalescesBurstWakeups(t *testing.T) {
	b := NewOutputBuffer()
	ch := b.Subscribe()
	// Three line-completing appends in quick succession. Subscriber
	// drained once should see at most one pending wakeup (channel
	// capacity is 1).
	b.Append([]byte("one\n"))
	b.Append([]byte("two\n"))
	b.Append([]byte("three\n"))
	<-ch
	select {
	case <-ch:
		t.Fatal("burst of Appends should coalesce into one wakeup, got two")
	case <-time.After(20 * time.Millisecond):
		// expected
	}
}

func TestOutputBufferAllLinesReturnsCopy(t *testing.T) {
	b := NewOutputBuffer()
	b.Append([]byte("a\nb\n"))
	got := b.AllLines()
	want := []string{"a", "b"}
	if !equalStringSlice(got, want) {
		t.Errorf("AllLines = %v, want %v", got, want)
	}
	// Mutate the returned slice; original buffer should be untouched.
	got[0] = "mutated"
	if again := b.AllLines(); again[0] != "a" {
		t.Errorf("AllLines returned a live slice; buffer mutated to %q", again[0])
	}
}

func TestOutputBufferAppendIgnoresEmpty(t *testing.T) {
	b := NewOutputBuffer()
	b.Append(nil)
	b.Append([]byte{})
	if b.LineCount() != 0 {
		t.Errorf("empty appends should be no-ops")
	}
}

func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

