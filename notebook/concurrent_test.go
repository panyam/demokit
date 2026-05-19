package notebook

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// --- streamCell: a Cell with an OutputBuffer for Stream tests ---

type streamCell struct {
	id  string
	buf *OutputBuffer
}

func newStreamCell(id string) *streamCell {
	return &streamCell{id: id, buf: NewOutputBuffer()}
}

func (c *streamCell) ID() string                { return c.id }
func (c *streamCell) HeightHint(int) int        { return c.buf.LineCount() + 2 }
func (c *streamCell) StatusHint(Mode) string    { return "" }
func (c *streamCell) Buffer() *OutputBuffer     { return c.buf }
func (c *streamCell) Update(tea.Msg, Mode) (Cell, tea.Cmd) {
	return c, nil
}

func (c *streamCell) RenderRows(width, startRow, endRow int, _ bool, _ Mode) []string {
	rows := []string{"--- " + c.id + " ---"}
	rows = append(rows, c.buf.AllLines()...)
	rows = append(rows, "--- end ---")
	if startRow < 0 {
		startRow = 0
	}
	if endRow > len(rows) {
		endRow = len(rows)
	}
	if startRow >= endRow {
		return nil
	}
	return rows[startRow:endRow]
}

// waitForInputWaiter spins until the rendezvous has at least one
// registered input waiter (for the given cell id), so the test
// can safely Send a resolution without racing the await goroutine.
func waitForInputWaiter(t *testing.T, nb *Notebook, id CellID) {
	t.Helper()
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		nb.rdv.mu.Lock()
		_, ok := nb.rdv.inputs[id]
		nb.rdv.mu.Unlock()
		if ok {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("no input waiter for %q after 200ms", id)
}

// waitForAdvanceWaiter spins until the rendezvous has at least
// one registered advance waiter.
func waitForAdvanceWaiter(t *testing.T, nb *Notebook) {
	t.Helper()
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		nb.rdv.mu.Lock()
		n := len(nb.rdv.advances)
		nb.rdv.mu.Unlock()
		if n > 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("no advance waiter registered after 200ms")
}

// --- Stream ---

func TestStreamFillsCellIncrementally(t *testing.T) {
	nb := New(WithHeadless(), WithSize(80, 20))
	go nb.Run()
	t.Cleanup(nb.Stop)

	c := newStreamCell("out")
	nb.Append(c)
	w := nb.Stream("out")
	for i := 0; i < 5; i++ {
		fmt.Fprintf(w, "line %d\n", i)
	}
	if got := c.buf.LineCount(); got != 5 {
		t.Errorf("LineCount = %d, want 5", got)
	}
	// Snapshot should reflect the streamed content.
	snap := nb.Snapshot()
	if !strings.Contains(snap, "line 4") {
		t.Errorf("Snapshot missing latest line:\n%s", snap)
	}
}

func TestStreamAfterRemoveDiscards(t *testing.T) {
	nb := New(WithHeadless(), WithSize(80, 20))
	go nb.Run()
	t.Cleanup(nb.Stop)

	c := newStreamCell("out")
	nb.Append(c)
	w := nb.Stream("out")
	nb.Remove("out")
	// After removal, the next Stream("out") returns Discard.
	if w2 := nb.Stream("out"); w2 != io.Discard {
		t.Errorf("Stream after Remove = %T, want io.Discard", w2)
	}
	// The previously-returned writer still works on the buffer
	// (the buffer outlives the cell's presence in the store);
	// drops are silent in that the buffer is no longer rendered.
	fmt.Fprintln(w, "post-remove")
	if c.buf.LineCount() != 1 {
		t.Errorf("buffer should have 1 line; got %d", c.buf.LineCount())
	}
}

func TestStreamForMissingCellReturnsDiscard(t *testing.T) {
	nb := New(WithHeadless(), WithSize(40, 10))
	go nb.Run()
	t.Cleanup(nb.Stop)
	if w := nb.Stream("missing"); w != io.Discard {
		t.Errorf("Stream(missing) = %T, want io.Discard", w)
	}
}

// --- AwaitAdvance / AwaitInput ---

func TestPromptResolvesWithSubmittedAnswer(t *testing.T) {
	nb := New(WithHeadless(), WithSize(40, 10))
	go nb.Run()
	t.Cleanup(nb.Stop)

	nb.Append(newStreamCell("p")) // any cell; AwaitInputBy doesn't care
	done := make(chan InputResponse, 1)
	go func() { done <- nb.AwaitInputBy("p") }()
	waitForInputWaiter(t, nb, "p")

	nb.program.Send(PromptSubmittedMsg{
		CellID:  "p",
		Answers: map[string]any{"name": "alice"},
	})
	select {
	case resp := <-done:
		if resp.Source != "user-submitted" {
			t.Errorf("Source = %q, want user-submitted", resp.Source)
		}
		if resp.Answers["name"] != "alice" {
			t.Errorf("answer = %v, want alice", resp.Answers["name"])
		}
	case <-time.After(time.Second):
		t.Fatal("AwaitInputBy did not return within 1s")
	}
}

func TestAwaitAdvanceResolvesOnEnter(t *testing.T) {
	nb := New(WithHeadless(), WithSize(40, 10))
	go nb.Run()
	t.Cleanup(nb.Stop)

	done := make(chan AdvanceResponse, 1)
	go func() { done <- nb.AwaitAdvance(time.Time{}) }()
	waitForAdvanceWaiter(t, nb)

	nb.program.Send(tea.KeyMsg{Type: tea.KeyEnter})
	select {
	case resp := <-done:
		if resp.Source != "user-enter" {
			t.Errorf("Source = %q, want user-enter", resp.Source)
		}
	case <-time.After(time.Second):
		t.Fatal("AwaitAdvance did not return within 1s")
	}
}

func TestAwaitAdvanceDeadlineFiresAsAutoAdvance(t *testing.T) {
	nb := New(WithHeadless(), WithSize(40, 10))
	go nb.Run()
	t.Cleanup(nb.Stop)

	resp := nb.AwaitAdvance(time.Now().Add(30 * time.Millisecond))
	if resp.Source != "auto-advance" {
		t.Errorf("Source = %q, want auto-advance", resp.Source)
	}
}

func TestAwaitInputCancelledOnRemove(t *testing.T) {
	nb := New(WithHeadless(), WithSize(40, 10))
	go nb.Run()
	t.Cleanup(nb.Stop)

	nb.Append(newStreamCell("p"))
	done := make(chan InputResponse, 1)
	go func() { done <- nb.AwaitInputBy("p") }()
	waitForInputWaiter(t, nb, "p")

	nb.Remove("p")
	select {
	case resp := <-done:
		if resp.Source != "cancelled" {
			t.Errorf("Source = %q, want cancelled", resp.Source)
		}
	case <-time.After(time.Second):
		t.Fatal("AwaitInputBy did not unblock after Remove")
	}
}

// --- Stop unblocks awaits ---

func TestStopUnblocksAwaitAdvance(t *testing.T) {
	nb := New(WithHeadless(), WithSize(40, 10))
	go nb.Run()

	done := make(chan AdvanceResponse, 1)
	go func() { done <- nb.AwaitAdvance(time.Time{}) }()
	waitForAdvanceWaiter(t, nb)

	nb.Stop()
	select {
	case resp := <-done:
		if resp.Source != "cancelled" {
			t.Errorf("Source = %q, want cancelled", resp.Source)
		}
	case <-time.After(time.Second):
		t.Fatal("AwaitAdvance did not unblock after Stop")
	}
}

func TestStopUnblocksAwaitInput(t *testing.T) {
	nb := New(WithHeadless(), WithSize(40, 10))
	go nb.Run()

	nb.Append(newStreamCell("p"))
	done := make(chan InputResponse, 1)
	go func() { done <- nb.AwaitInputBy("p") }()
	waitForInputWaiter(t, nb, "p")

	nb.Stop()
	select {
	case resp := <-done:
		if resp.Source != "cancelled" {
			t.Errorf("Source = %q, want cancelled", resp.Source)
		}
	case <-time.After(time.Second):
		t.Fatal("AwaitInputBy did not unblock after Stop")
	}
}

// --- Concurrent writers ---

func TestConcurrentStreams(t *testing.T) {
	nb := New(WithHeadless(), WithSize(80, 20))
	go nb.Run()
	t.Cleanup(nb.Stop)

	const cells = 4
	const linesPerCell = 200
	for i := 0; i < cells; i++ {
		nb.Append(newStreamCell(fmt.Sprintf("c%d", i)))
	}
	var wg sync.WaitGroup
	for i := 0; i < cells; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			w := nb.Stream(fmt.Sprintf("c%d", idx))
			for j := 0; j < linesPerCell; j++ {
				fmt.Fprintf(w, "c%d line %d\n", idx, j)
			}
		}(i)
	}
	wg.Wait()
	for i := 0; i < cells; i++ {
		c, _ := nb.Get(fmt.Sprintf("c%d", i))
		if got := c.(*streamCell).buf.LineCount(); got != linesPerCell {
			t.Errorf("c%d LineCount = %d, want %d", i, got, linesPerCell)
		}
	}
}

func TestAppendDuringStream(t *testing.T) {
	nb := New(WithHeadless(), WithSize(80, 20))
	go nb.Run()
	t.Cleanup(nb.Stop)

	c := newStreamCell("stream")
	nb.Append(c)
	w := nb.Stream("stream")

	streamDone := make(chan struct{})
	go func() {
		defer close(streamDone)
		for i := 0; i < 500; i++ {
			fmt.Fprintf(w, "x%d\n", i)
		}
	}()

	// Append many cells while the stream runs.
	for i := 0; i < 100; i++ {
		if _, err := nb.Append(newStreamCell(fmt.Sprintf("a%d", i))); err != nil {
			t.Fatalf("Append a%d error: %v", i, err)
		}
	}
	<-streamDone

	if got := c.buf.LineCount(); got != 500 {
		t.Errorf("stream LineCount = %d, want 500", got)
	}
	if got := nb.Len(); got != 101 {
		t.Errorf("Len = %d, want 101 (1 stream + 100 appended)", got)
	}
}

func TestStreamLineCountUnderLoad(t *testing.T) {
	nb := New(WithHeadless(), WithSize(80, 20))
	go nb.Run()
	t.Cleanup(nb.Stop)

	c := newStreamCell("hi")
	nb.Append(c)
	w := nb.Stream("hi")

	const N = 5000
	var n atomic.Int64
	var wg sync.WaitGroup
	for g := 0; g < 5; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < N/5; i++ {
				fmt.Fprintln(w, "x")
				n.Add(1)
			}
		}()
	}
	wg.Wait()

	if got, want := c.buf.LineCount(), int(n.Load()); got != want {
		t.Errorf("LineCount = %d, want %d (no lost lines)", got, want)
	}
}

// --- Buffer io.Writer concurrency ---

func TestOutputBufferConcurrentWrites(t *testing.T) {
	buf := NewOutputBuffer()
	var wg sync.WaitGroup
	const writers = 8
	const linesPer = 250
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < linesPer; i++ {
				var b bytes.Buffer
				fmt.Fprintf(&b, "w%d-%d\n", id, i)
				buf.Append(b.Bytes())
			}
		}(w)
	}
	wg.Wait()
	if got, want := buf.LineCount(), writers*linesPer; got != want {
		t.Errorf("LineCount = %d, want %d", got, want)
	}
}
