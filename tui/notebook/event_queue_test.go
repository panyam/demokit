package notebook

import (
	"sync"
	"testing"
	"time"
)

func TestEventQueueAppendThenRead(t *testing.T) {
	q := newEventQueue()
	q.Append(eventHeader{Title: "A"})
	q.Append(eventDone{})

	events, off := q.Read(0)
	if got, want := len(events), 2; got != want {
		t.Fatalf("Read(0) returned %d events, want %d", got, want)
	}
	if off != 2 {
		t.Errorf("newOffset = %d, want 2", off)
	}
	if _, ok := events[0].(eventHeader); !ok {
		t.Errorf("events[0] = %T, want eventHeader", events[0])
	}
	if _, ok := events[1].(eventDone); !ok {
		t.Errorf("events[1] = %T, want eventDone", events[1])
	}
}

func TestEventQueueIncrementalDrain(t *testing.T) {
	q := newEventQueue()
	q.Append(eventHeader{Title: "A"})

	first, off1 := q.Read(0)
	if len(first) != 1 || off1 != 1 {
		t.Fatalf("first drain: len=%d off=%d, want 1, 1", len(first), off1)
	}

	q.Append(eventDone{})
	q.Append(eventDone{})

	second, off2 := q.Read(off1)
	if len(second) != 2 || off2 != 3 {
		t.Errorf("second drain: len=%d off=%d, want 2, 3", len(second), off2)
	}

	third, off3 := q.Read(off2)
	if len(third) != 0 || off3 != 3 {
		t.Errorf("empty drain: len=%d off=%d, want 0, 3", len(third), off3)
	}
}

func TestEventQueueNotifyOnAppend(t *testing.T) {
	q := newEventQueue()
	notify := q.Notify()
	q.Append(eventHeader{Title: "A"})
	select {
	case <-notify:
		// expected
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Notify channel did not signal after Append")
	}
}

func TestEventQueueNotifyCoalesces(t *testing.T) {
	q := newEventQueue()
	notify := q.Notify()
	for i := 0; i < 5; i++ {
		q.Append(eventHeader{Title: "A"})
	}
	// Drain at most one wake-up; channel buffer is 1.
	<-notify
	select {
	case <-notify:
		t.Error("5 appends should coalesce to one notify; got a second wake-up")
	case <-time.After(20 * time.Millisecond):
		// expected
	}
}

func TestEventQueueReadFromOutOfRangeReturnsEmpty(t *testing.T) {
	q := newEventQueue()
	q.Append(eventDone{})
	events, off := q.Read(99)
	if len(events) != 0 {
		t.Errorf("out-of-range Read returned %d events, want 0", len(events))
	}
	if off != 1 {
		t.Errorf("out-of-range Read offset = %d, want 1 (current Len)", off)
	}
}

func TestEventQueueConcurrentAppends(t *testing.T) {
	q := newEventQueue()
	const n = 100
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			q.Append(eventDone{})
		}()
	}
	wg.Wait()
	if got := q.Len(); got != n {
		t.Errorf("Len = %d after %d concurrent appends, want %d", got, n, n)
	}
}
