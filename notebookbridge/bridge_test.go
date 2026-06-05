package notebookbridge

import (
	"strings"
	"testing"
	"time"

	"github.com/panyam/demokit"
	"github.com/panyam/demokit/events"
	"github.com/panyam/demokit/notebook"
	"github.com/panyam/demokit/notebook/cells"
)

func TestSlugifyNormalizesCommonShapes(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"Hello World", "hello-world"},
		{"alreadykebab", "alreadykebab"},
		{"  spaces  ", "spaces"},
		{"camelCaseInput", "camelcaseinput"},
		{"path/to/something", "path-to-something"},
		{"!!!leading-special!!!", "leading-special"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := slugify(tt.in); got != tt.want {
			t.Errorf("slugify(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestOutputIDIncludesVisitAndSlug(t *testing.T) {
	if got := outputID(3, "Refresh Token"); got != "refresh-token#3.output" {
		t.Errorf("outputID(3, Refresh Token) = %q", got)
	}
	if got := outputID(7, ""); got != "step7#7.output" {
		t.Errorf("outputID(7, empty) = %q, want fallback step7#7.output", got)
	}
}

func TestBuildHeaderBodyJoinsNoteArrowsRefs(t *testing.T) {
	e := events.StepStart{
		StepID: "s",
		Title:  "S",
		Note:   "hello world",
		Arrows: []events.Arrow{{From: "A", To: "B", Label: "label"}},
		Refs:   []events.Ref{{Name: "RFC 1", URL: "https://example/rfc1"}},
	}
	body := buildHeaderBody(e)
	for _, want := range []string{"hello world", "A -> B: label", "ref: RFC 1 (https://example/rfc1)"} {
		if !strings.Contains(body, want) {
			t.Errorf("header body missing %q, got:\n%s", want, body)
		}
	}
}

func TestBuildCellsFromStepStartProducesHeaderAndVerbatims(t *testing.T) {
	b := New()
	e := events.StepStart{
		Visit:  0,
		StepID: "refresh",
		Title:  "Refresh the token",
		Note:   "The access_token expires.",
		Verbatims: []events.Verbatim{
			{Label: "Body", Variants: []events.Variant{
				{Label: "curl", Lang: "bash", Content: "curl -s ...", IsDefault: true},
				{Label: "python", Lang: "python", Content: "import requests"},
			}},
		},
	}
	out := b.buildCellsFromStepStart(e)
	if len(out) != 2 {
		t.Fatalf("cells = %d, want 2 (header + verbatim)", len(out))
	}
	if out[0].ID() != "refresh#0.meta" {
		t.Errorf("header id = %q, want refresh#0.meta", out[0].ID())
	}
	if out[1].ID() != "refresh#0.verbatim0" {
		t.Errorf("verbatim id = %q, want refresh#0.verbatim0", out[1].ID())
	}
}

// TestBorderStyleHorizontalOnlyPlumbedToVerbatim verifies the
// bridge's WithBorderStyle reaches the constructed VerbatimCell's
// Edges field. Acceptance for the sides axis of issue 55 on the
// notebook side.
func TestBorderStyleHorizontalOnlyPlumbedToVerbatim(t *testing.T) {
	b := New().WithBorderStyle(demokit.BorderHorizontalOnly)
	e := events.StepStart{
		StepID: "s",
		Title:  "S",
		Verbatims: []events.Verbatim{
			{Label: "Body", Variants: []events.Variant{
				{Label: "curl", Content: "curl ..."},
			}},
		},
	}
	out := b.buildCellsFromStepStart(e)
	if len(out) != 2 {
		t.Fatalf("cells = %d, want 2", len(out))
	}
	vc, ok := out[1].(*cells.VerbatimCell)
	if !ok {
		t.Fatalf("expected *cells.VerbatimCell, got %T", out[1])
	}
	want := cells.HorizontalEdges()
	if vc.Style.Edges != want {
		t.Errorf("VerbatimCell.Style.Edges = %+v, want %+v", vc.Style.Edges, want)
	}
}

// TestBorderStyleNoneZeroesEdges verifies BorderNone drops all
// edges from the VerbatimCell.
func TestBorderStyleNoneZeroesEdges(t *testing.T) {
	b := New().WithBorderStyle(demokit.BorderNone)
	e := events.StepStart{
		StepID: "s",
		Verbatims: []events.Verbatim{
			{Label: "Body", Variants: []events.Variant{{Content: "x"}}},
		},
	}
	out := b.buildCellsFromStepStart(e)
	vc := out[1].(*cells.VerbatimCell)
	want := cells.BorderEdges{}
	if vc.Style.Edges != want {
		t.Errorf("BorderNone should zero all edges, got %+v", vc.Style.Edges)
	}
}

// TestBorderCharsCustomPlumbedToVerbatim verifies a custom
// BorderChars value reaches the VerbatimCell's Border field as a
// matching lipgloss.Border. Acceptance for the chars axis of
// issue 55 on the notebook side.
func TestBorderCharsCustomPlumbedToVerbatim(t *testing.T) {
	custom := demokit.BorderChars{
		Top: "#", Bottom: "#", Left: "*", Right: "*",
		TopLeft: "+", TopRight: "+", BottomLeft: "+", BottomRight: "+",
	}
	b := New().WithBorderChars(custom)
	e := events.StepStart{
		StepID: "s",
		Verbatims: []events.Verbatim{
			{Label: "Body", Variants: []events.Variant{{Content: "x"}}},
		},
	}
	out := b.buildCellsFromStepStart(e)
	vc := out[1].(*cells.VerbatimCell)
	if vc.Style.Border.Top != "#" || vc.Style.Border.Left != "*" || vc.Style.Border.TopLeft != "+" {
		t.Errorf("VerbatimCell.Style.Border = %+v, want chars from custom %+v",
			vc.Style.Border, custom)
	}
}

// TestBorderCharsHInfersHorizontalEdges verifies that the H-suffix
// presets (Left/Right empty) one-call-set the sides to horizontal-
// only without a companion WithBorderStyle.
func TestBorderCharsHInfersHorizontalEdges(t *testing.T) {
	b := New().WithBorderChars(demokit.BorderCharsRoundedH)
	e := events.StepStart{
		StepID: "s",
		Verbatims: []events.Verbatim{
			{Label: "Body", Variants: []events.Variant{{Content: "x"}}},
		},
	}
	out := b.buildCellsFromStepStart(e)
	vc := out[1].(*cells.VerbatimCell)
	if vc.Style.Edges.Left || vc.Style.Edges.Right {
		t.Errorf("Left/Right edges should be off (inferred from empty chars), got %+v",
			vc.Style.Edges)
	}
	if !vc.Style.Edges.Top || !vc.Style.Edges.Bottom {
		t.Errorf("Top/Bottom edges should be on, got %+v", vc.Style.Edges)
	}
}

// TestBorderStyleDefaultPreservesCellDefaults verifies that with
// no WithBorderStyle / WithBorderChars call, VerbatimCell uses its
// all-sides default and OutputCell uses its horizontal-edges
// default — backwards compatibility for callers who don't opt in.
func TestBorderStyleDefaultPreservesCellDefaults(t *testing.T) {
	b := New()
	e := events.StepStart{
		StepID: "s",
		Verbatims: []events.Verbatim{
			{Label: "Body", Variants: []events.Variant{{Content: "x"}}},
		},
	}
	out := b.buildCellsFromStepStart(e)
	vc := out[1].(*cells.VerbatimCell)
	if vc.Style.Edges != cells.AllEdges() {
		t.Errorf("default VerbatimCell edges should be AllEdges, got %+v", vc.Style.Edges)
	}

	// OutputCell's default-from-bridge differs: we want horizontal-
	// edges-only (matching the cell's built-in default).
	if got, want := b.edgesForOutput(), cells.HorizontalEdges(); got != want {
		t.Errorf("default OutputCell edges should be HorizontalEdges, got %+v", got)
	}
}

// Regression for #40. demokit.Execute appends the Header (and
// usually the first StepStart) before RenderHeader spawns the
// bridge's drain goroutine, so those events predate Subscribe().
// gocurrent's Notify only fires for post-subscribe activity (see
// gocurrent's TestLateSubscriberSeesAllPastEvents) — the backlog
// is reached via ReadFrom, not Notify. With the producer parked
// in AwaitResolution, no further append ever signals, so a drainer
// that waits on Notify without an initial catch-up read never sees
// the header and the notebook renders empty. Here every event is
// appended before the drainer starts, so only a catch-up drain can
// surface them.
func TestDrainDeliversEventsAppendedBeforeSubscribe(t *testing.T) {
	q := events.NewQueue()
	q.Append(events.Header{Title: "Cave of Cards", Description: "an adventure"})
	q.Append(events.StepStart{Visit: 1, StepID: "entrance", Title: "The Entrance"})

	b := New()
	b.queue = q
	b.outCellByVisit = map[int]notebook.CellID{}
	b.nbDone = make(chan struct{})
	nb := notebook.New(notebook.WithHeadless(), notebook.WithSize(60, 20))
	b.nb = nb
	go nb.Run()
	defer nb.Stop()
	defer close(b.nbDone)

	go b.drainEvents()

	deadline := time.After(2 * time.Second)
	for {
		snap := nb.Snapshot()
		if strings.Contains(snap, "Cave of Cards") && strings.Contains(snap, "The Entrance") {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("header/step never rendered — events appended before Subscribe were dropped:\n%s", snap)
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func TestConvertInputsKeepsTypeAndDefault(t *testing.T) {
	in := []events.Input{
		events.NewStringInput("name", "Name?", "alice"),
		events.NewIntInput("count", "Count?", 42),
		events.NewChoiceInput("flavor", "Flavor?", "vanilla", []string{"vanilla", "chocolate"}),
	}
	out := convertInputs(in)
	if len(out) != 3 {
		t.Fatalf("len = %d, want 3", len(out))
	}
	if out[0].InputName() != "name" || out[0].InputDefault() != "alice" {
		t.Errorf("string conversion lost data: %+v", out[0])
	}
	if out[1].InputName() != "count" || out[1].InputDefault() != 42 {
		t.Errorf("int conversion lost data: %+v", out[1])
	}
	if out[2].InputName() != "flavor" || out[2].InputDefault() != "vanilla" {
		t.Errorf("choice conversion lost data: %+v", out[2])
	}
	// Verify a Choice round-tripped its options through Parse.
	if _, err := out[2].Parse("vanilla"); err != nil {
		t.Errorf("choice Parse should accept declared option: %v", err)
	}
	if _, err := out[2].Parse("teal"); err == nil {
		t.Errorf("choice Parse should reject unknown option")
	}
}
