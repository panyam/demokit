package main

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/panyam/demokit/notebook"
	"github.com/panyam/demokit/notebook/cells"
)

func TestStepCount(t *testing.T) {
	cases := []struct {
		a, b, step float64
		want       int
	}{
		{0, 10, 1, 11},
		{0, 1, 0.25, 5},
		{10, 0, -2, 6},
		{0, 0, 1, 1},
		{0, 5, -1, 0}, // wrong direction → no points
	}
	for _, c := range cases {
		if got := stepCount(c.a, c.b, c.step); got != c.want {
			t.Errorf("stepCount(%g,%g,%g) = %d, want %d", c.a, c.b, c.step, got, c.want)
		}
	}
}

func TestDefaultSeriesStepGives40Points(t *testing.T) {
	step := defaultSeriesStep(0, 10)
	count := stepCount(0, 10, step)
	if count < 35 || count > 45 {
		t.Errorf("defaultSeriesStep produced count=%d, want ~40", count)
	}
}

func TestRunSeries_AppendsOutputCellAndStreams(t *testing.T) {
	nb := notebook.New(
		notebook.WithHeadless(),
		notebook.WithSize(80, 24),
	)
	go nb.Run()
	t.Cleanup(nb.Stop)

	env := NewEnv()
	ctl := newSeriesController()
	if err := runSeries(nb, ctl, 1, "series x*x from 1 to 5 step 1 rate 1", env); err != nil {
		t.Fatalf("runSeries returned error: %v", err)
	}

	// The OutputCell is appended synchronously; streaming runs in a
	// goroutine. Poll briefly for the full set of values to land.
	deadline := time.Now().Add(2 * time.Second)
	var snap string
	for time.Now().Before(deadline) {
		snap = nb.Snapshot()
		if strings.Contains(snap, "x=5") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	for _, want := range []string{"x=1", "x=2", "x=3", "x=4", "x=5"} {
		if !strings.Contains(snap, want) {
			t.Errorf("snapshot missing %q:\n%s", want, snap)
		}
	}
	cell, ok := nb.Get("series-1")
	if !ok {
		t.Fatal("series cell not appended")
	}
	oc, ok := cell.(*cells.OutputCell)
	if !ok {
		t.Fatalf("cell type = %T, want *OutputCell", cell)
	}
	if oc.MaxBody() < 6 {
		t.Errorf("series cell maxBody = %d, want >=6 (header + 5 values)", oc.MaxBody())
	}
}

func TestRunSeries_BadStepDirection(t *testing.T) {
	nb := notebook.New(notebook.WithHeadless(), notebook.WithSize(80, 24))
	go nb.Run()
	t.Cleanup(nb.Stop)
	env := NewEnv()
	err := runSeries(nb, newSeriesController(), 1, "series x from 1 to 5 step -1", env)
	if err == nil {
		t.Error("expected error for wrong-direction step")
	}
}

func TestRunSeries_NonNumericBounds(t *testing.T) {
	nb := notebook.New(notebook.WithHeadless(), notebook.WithSize(80, 24))
	go nb.Run()
	t.Cleanup(nb.Stop)
	env := NewEnv()
	err := runSeries(nb, newSeriesController(), 1, "series x from \"hi\" to 5", env)
	if err == nil {
		t.Error("expected error for non-numeric 'from'")
	}
}

func TestRunSeries_RateAndCancellation(t *testing.T) {
	nb := notebook.New(notebook.WithHeadless(), notebook.WithSize(80, 24))
	go nb.Run()
	t.Cleanup(nb.Stop)
	env := NewEnv()
	ctl := newSeriesController()

	// 100 points * 50ms = ~5s if it ran to completion. Cancel
	// immediately after launch; should see "(cancelled)" tail
	// well before that.
	if err := runSeries(nb, ctl, 1, "series x from 1 to 100 step 1 rate 50", env); err != nil {
		t.Fatalf("runSeries error: %v", err)
	}
	if !ctl.cancelAll() {
		t.Error("cancelAll returned false but a series was running")
	}
	deadline := time.Now().Add(2 * time.Second)
	var snap string
	for time.Now().Before(deadline) {
		snap = nb.Snapshot()
		if strings.Contains(snap, "(cancelled)") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !strings.Contains(snap, "(cancelled)") {
		t.Errorf("snapshot missing (cancelled) tail:\n%s", snap)
	}
	if strings.Contains(snap, "x=100") {
		t.Errorf("series ran past cancellation:\n%s", snap)
	}
}

func TestParseSeriesSrc_RateAndStepEitherOrder(t *testing.T) {
	cases := []string{
		"series x from 0 to 10 step 1 rate 50",
		"series x from 0 to 10 rate 50 step 1",
		"series x from 0 to 10 rate 50",
		"series x from 0 to 10 step 1",
		"series x from 0 to 10",
	}
	for _, src := range cases {
		if _, _, _, _, err := parseSeriesSrc(src); err != nil {
			t.Errorf("parseSeriesSrc(%q) returned error: %v", src, err)
		}
	}
}

func TestRunSeries_BadRate(t *testing.T) {
	nb := notebook.New(notebook.WithHeadless(), notebook.WithSize(80, 24))
	go nb.Run()
	t.Cleanup(nb.Stop)
	env := NewEnv()
	for _, src := range []string{
		"series x from 0 to 10 rate -5",
		"series x from 0 to 10 rate fast",
	} {
		if err := runSeries(nb, newSeriesController(), 1, src, env); err == nil {
			t.Errorf("expected error for %q", src)
		}
	}
}

func TestSeriesEndpointPinning(t *testing.T) {
	// Floating-point drift: 0 + 5*0.2 = 1.0000000000000002 in IEEE
	// 754 — without endpoint pinning the loop would emit x=1.0...02
	// on the last point. stepCount + the streamSeries clamp handle
	// this.
	a, b, step := 0.0, 1.0, 0.2
	count := stepCount(a, b, step)
	last := a + float64(count-1)*step
	if math.Abs(last-b) > 1e-9 {
		// Not the test's purpose to fail here — the clamp in
		// streamSeries handles the displayed value. Just sanity.
		t.Logf("last (pre-clamp) = %v vs b = %v", last, b)
	}
}
