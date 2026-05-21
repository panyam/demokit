package notebookbridge

import (
	"strings"
	"testing"

	"github.com/panyam/demokit/events"
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
