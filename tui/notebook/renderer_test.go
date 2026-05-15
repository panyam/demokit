package notebook

import (
	"strings"
	"testing"

	"github.com/panyam/demokit"
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

func TestVariantsFromViewPreservesFields(t *testing.T) {
	in := []demokit.VariantView{
		{Label: "curl", Lang: "bash", Content: "curl -s ...", IsDefault: true},
		{Label: "python", Lang: "python", Content: "import requests", IsDefault: false},
	}
	out := variantsFromView(in)
	if len(out) != len(in) {
		t.Fatalf("len = %d, want %d", len(out), len(in))
	}
	for i, v := range out {
		if v.Label != in[i].Label || v.Lang != in[i].Lang || v.Content != in[i].Content || v.IsDefault != in[i].IsDefault {
			t.Errorf("variant %d: got %+v, want %+v", i, v, in[i])
		}
	}
}

func TestCellsForStepBuildsExpectedShape(t *testing.T) {
	d := demokit.New("test")
	d.Step("Refresh the token").ID("refresh").
		Note("The access_token expires after 3600 seconds.").
		Arrow("App", "AS", "POST /token (refresh)").
		Ref(demokit.Ref{Name: "RFC 6749 §6"}).
		VerbatimVariants("Body",
			demokit.MakeVariant("curl", "bash", "curl -s ...").Default(),
			demokit.MakeVariant("python", "python", "import requests"),
		)
	step := d.StepByID("refresh")
	if step == nil {
		t.Fatal("expected step with ID refresh")
	}
	bodyCells, outputCell, buf, oid := cellsForStep(0, step, DefaultPalette())
	// Body is meta + verbatim only — OutputCell is deferred until
	// the user has signalled "run" (handled separately by the
	// renderer's appendOutputCell flush).
	if len(bodyCells) != 2 {
		t.Fatalf("body cells: got %d, want 2 (meta + verbatim)", len(bodyCells))
	}
	if _, ok := bodyCells[0].(*MetaCell); !ok {
		t.Errorf("bodyCells[0] = %T, want *MetaCell", bodyCells[0])
	}
	if _, ok := bodyCells[1].(*VerbatimCell); !ok {
		t.Errorf("bodyCells[1] = %T, want *VerbatimCell", bodyCells[1])
	}
	oc, ok := outputCell.(*OutputCell)
	if !ok {
		t.Fatalf("outputCell = %T, want *OutputCell", outputCell)
	}
	if oc.ID() != oid {
		t.Errorf("output cell ID mismatch: oc.ID()=%q, returned oid=%q", oc.ID(), oid)
	}
	if buf == nil {
		t.Fatal("returned OutputBuffer is nil")
	}
}

func TestBuildMetaBodyJoinsNoteArrowsRefs(t *testing.T) {
	d := demokit.New("t")
	d.Step("S").ID("s").
		Note("hello world").
		Arrow("A", "B", "label").
		Ref(demokit.Ref{Name: "RFC 1", URL: "https://example/rfc1"})
	body := buildMetaBody(d.StepByID("s"))
	for _, want := range []string{"hello world", "A -> B: label", "ref: RFC 1 (https://example/rfc1)"} {
		if !strings.Contains(body, want) {
			t.Errorf("meta body missing %q, got:\n%s", want, body)
		}
	}
}
