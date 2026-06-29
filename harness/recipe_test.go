package harness_test

import (
	"reflect"
	"testing"

	"github.com/panyam/demokit"
	"github.com/panyam/demokit/harness"
)

// TestWireRecipe verifies the helper builds one "Reproduce on the wire"
// block with curl (default, bash) and go variants in that order — the
// contract callers and renderers rely on.
func TestWireRecipe(t *testing.T) {
	s := &demokit.StepDef{}
	harness.WireRecipe(s, "curl x", "http.Get()")

	blocks := s.VerbatimBlocks()
	if len(blocks) != 1 {
		t.Fatalf("blocks = %d, want 1", len(blocks))
	}
	if blocks[0].Label != harness.WireLabel {
		t.Errorf("label = %q, want %q", blocks[0].Label, harness.WireLabel)
	}
	vs := blocks[0].Variants
	if len(vs) != 2 {
		t.Fatalf("variants = %d, want 2", len(vs))
	}
	if vs[0].Label != "curl" || vs[0].Lang != "bash" || vs[0].Content != "curl x" || !vs[0].IsDefault {
		t.Errorf("curl variant = %+v", vs[0])
	}
	if vs[1].Label != "go" || vs[1].Lang != "go" || vs[1].Content != "http.Get()" || vs[1].IsDefault {
		t.Errorf("go variant = %+v", vs[1])
	}
}

func TestSplitLines(t *testing.T) {
	if got := harness.SplitLines("a\nb\nc"); !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Errorf("SplitLines = %v", got)
	}
}
