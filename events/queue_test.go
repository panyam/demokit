package events

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestQueueAppendReturnsOffset(t *testing.T) {
	q := NewQueue()
	off0 := q.Append(Header{Title: "A"})
	off1 := q.Append(Done{})
	if off0 != 0 || off1 != 1 {
		t.Errorf("offsets = %d, %d; want 0, 1", off0, off1)
	}
}

func TestQueueReadDrainsIncrementally(t *testing.T) {
	q := NewQueue()
	q.Append(Header{Title: "A"})

	first, off1 := q.Read(0)
	if len(first) != 1 || off1 != 1 {
		t.Fatalf("first drain: len=%d off=%d, want 1, 1", len(first), off1)
	}

	q.Append(Done{})
	q.Append(Done{})

	second, off2 := q.Read(off1)
	if len(second) != 2 || off2 != 3 {
		t.Errorf("second drain: len=%d off=%d, want 2, 3", len(second), off2)
	}
}

func TestQueueNotifyOnAppend(t *testing.T) {
	q := NewQueue()
	notify := q.Notify()
	q.Append(Header{Title: "A"})
	select {
	case <-notify:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Notify did not signal after Append")
	}
}

func TestQueueReadAtBounds(t *testing.T) {
	q := NewQueue()
	q.Append(Header{Title: "A"})
	e, ok := q.ReadAt(0)
	if !ok {
		t.Fatal("ReadAt(0) returned ok=false")
	}
	if _, isHeader := e.(Header); !isHeader {
		t.Errorf("ReadAt(0) = %T, want Header", e)
	}
	if _, ok := q.ReadAt(99); ok {
		t.Error("ReadAt(99) should return ok=false for out-of-range")
	}
}

func TestResolveFillsAdvanceResolution(t *testing.T) {
	q := NewQueue()
	offset := q.Append(WaitForAdvance{Visit: 1})

	if err := q.Resolve(offset, &AdvanceResolution{Source: "user-enter"}); err != nil {
		t.Fatalf("Resolve returned %v", err)
	}
	e, _ := q.ReadAt(offset)
	w := e.(WaitForAdvance)
	if w.Resolution == nil {
		t.Fatal("Resolution should be non-nil after Resolve")
	}
	if w.Resolution.Source != "user-enter" {
		t.Errorf("Source = %q, want %q", w.Resolution.Source, "user-enter")
	}
}

func TestResolveFirstWriterWins(t *testing.T) {
	q := NewQueue()
	offset := q.Append(WaitForAdvance{Visit: 1})

	if err := q.Resolve(offset, &AdvanceResolution{Source: "consumer-A"}); err != nil {
		t.Fatalf("first Resolve: %v", err)
	}
	err := q.Resolve(offset, &AdvanceResolution{Source: "consumer-B"})
	if !errors.Is(err, ErrAlreadyResolved) {
		t.Errorf("second Resolve should return ErrAlreadyResolved; got %v", err)
	}
	e, _ := q.ReadAt(offset)
	w := e.(WaitForAdvance)
	if w.Resolution.Source != "consumer-A" {
		t.Errorf("first-writer-wins violated: Source = %q, want %q", w.Resolution.Source, "consumer-A")
	}
}

func TestResolvePromptOpen(t *testing.T) {
	q := NewQueue()
	offset := q.Append(PromptOpen{Visit: 1, Inputs: []Input{NewStringInput("x", "X?", nil)}})

	answers := map[string]any{"x": "hello"}
	if err := q.Resolve(offset, &PromptResolution{Answers: answers, Source: "user-submitted"}); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	e, _ := q.ReadAt(offset)
	p := e.(PromptOpen)
	if p.Resolution == nil || p.Resolution.Answers["x"] != "hello" {
		t.Errorf("PromptOpen resolution wrong: %+v", p.Resolution)
	}
}

func TestResolveRejectsNonSyncEvent(t *testing.T) {
	q := NewQueue()
	offset := q.Append(Header{Title: "A"})
	err := q.Resolve(offset, &AdvanceResolution{})
	if !errors.Is(err, ErrNotResolvable) {
		t.Errorf("Resolve on non-sync event should return ErrNotResolvable; got %v", err)
	}
}

func TestResolveRejectsWrongResolutionType(t *testing.T) {
	q := NewQueue()
	offset := q.Append(WaitForAdvance{Visit: 1})
	err := q.Resolve(offset, &PromptResolution{})
	if !errors.Is(err, ErrNotResolvable) {
		t.Errorf("WaitForAdvance + PromptResolution should fail; got %v", err)
	}
}

func TestAwaitResolutionBlocksUntilResolved(t *testing.T) {
	q := NewQueue()
	offset := q.Append(WaitForAdvance{Visit: 1})

	got := make(chan any, 1)
	go func() {
		got <- q.AwaitResolution(offset)
	}()

	// Briefly confirm it's not returning yet.
	select {
	case <-got:
		t.Fatal("AwaitResolution returned before Resolve")
	case <-time.After(30 * time.Millisecond):
	}

	_ = q.Resolve(offset, &AdvanceResolution{Source: "test"})

	select {
	case res := <-got:
		ar, ok := res.(*AdvanceResolution)
		if !ok || ar.Source != "test" {
			t.Errorf("AwaitResolution returned %T %+v, want *AdvanceResolution{Source:test}", res, res)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("AwaitResolution did not return after Resolve")
	}
}

func TestAwaitResolutionReturnsImmediatelyIfAlreadyResolved(t *testing.T) {
	q := NewQueue()
	offset := q.Append(WaitForAdvance{Visit: 1})
	_ = q.Resolve(offset, &AdvanceResolution{Source: "test"})

	// AwaitResolution must return without blocking — replay path
	// supplies events with pre-filled Resolution and producers
	// must not deadlock.
	done := make(chan any, 1)
	go func() {
		done <- q.AwaitResolution(offset)
	}()
	select {
	case res := <-done:
		if res == nil {
			t.Error("AwaitResolution returned nil for already-resolved event")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("AwaitResolution blocked on already-resolved event")
	}
}

func TestAwaitResolutionNilForNonSyncEvent(t *testing.T) {
	q := NewQueue()
	offset := q.Append(Header{Title: "A"})
	if got := q.AwaitResolution(offset); got != nil {
		t.Errorf("AwaitResolution on non-sync event = %v, want nil", got)
	}
}

func TestConcurrentResolveFirstWriterWins(t *testing.T) {
	// Stress: many goroutines race to resolve the same event.
	// Exactly one should succeed; all others should see
	// ErrAlreadyResolved. No panics, no double-resolve.
	q := NewQueue()
	offset := q.Append(WaitForAdvance{Visit: 1})

	const n = 50
	var (
		wg      sync.WaitGroup
		successes int32
		mu      sync.Mutex
	)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			err := q.Resolve(offset, &AdvanceResolution{Source: "id"})
			if err == nil {
				mu.Lock()
				successes++
				mu.Unlock()
			} else if !errors.Is(err, ErrAlreadyResolved) {
				t.Errorf("unexpected error: %v", err)
			}
		}(i)
	}
	wg.Wait()
	if successes != 1 {
		t.Errorf("successes = %d, want exactly 1", successes)
	}
}
