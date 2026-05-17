package notebook

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

func TestBuildCellsFromStepStartBuildsExpectedShape(t *testing.T) {
	e := events.StepStart{
		Visit:  0,
		StepID: "refresh",
		Title:  "Refresh the token",
		Note:   "The access_token expires after 3600 seconds.",
		Arrows: []events.Arrow{{From: "App", To: "AS", Label: "POST /token (refresh)"}},
		Refs:   []events.Ref{{Name: "RFC 6749 §6"}},
		Verbatims: []events.Verbatim{
			{Label: "Body", Variants: []events.Variant{
				{Label: "curl", Lang: "bash", Content: "curl -s ...", IsDefault: true},
				{Label: "python", Lang: "python", Content: "import requests"},
			}},
		},
	}
	cells := buildCellsFromStepStart(e, DefaultPalette())
	if len(cells) != 2 {
		t.Fatalf("cells = %d, want 2 (meta + verbatim)", len(cells))
	}
	if _, ok := cells[0].(*MetaCell); !ok {
		t.Errorf("cells[0] = %T, want *MetaCell", cells[0])
	}
	vc, ok := cells[1].(*VerbatimCell)
	if !ok {
		t.Fatalf("cells[1] = %T, want *VerbatimCell", cells[1])
	}
	if vc.ID() != "refresh#0.verbatim0" {
		t.Errorf("verbatim cell ID = %q, want %q", vc.ID(), "refresh#0.verbatim0")
	}
}

func TestBuildMetaBodyJoinsNoteArrowsRefs(t *testing.T) {
	e := events.StepStart{
		StepID: "s",
		Title:  "S",
		Note:   "hello world",
		Arrows: []events.Arrow{{From: "A", To: "B", Label: "label"}},
		Refs:   []events.Ref{{Name: "RFC 1", URL: "https://example/rfc1"}},
	}
	body := buildMetaBody(e)
	for _, want := range []string{"hello world", "A -> B: label", "ref: RFC 1 (https://example/rfc1)"} {
		if !strings.Contains(body, want) {
			t.Errorf("meta body missing %q, got:\n%s", want, body)
		}
	}
}
