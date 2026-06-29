package harness

import (
	"strings"

	"github.com/panyam/demokit"
)

// WireLabel is the block title WireRecipe attaches. Exported so callers
// can match it when post-processing or filtering verbatim blocks.
const WireLabel = "Reproduce on the wire"

// WireRecipe attaches a two-variant "Reproduce on the wire" verbatim
// block to a step: the wire-level curl form (the default, for
// non-interactive and markdown output) and the equivalent Go form. It is
// the common shape behind protocol walkthroughs that show both the raw
// request and the client-library call; for any other variant set, call
// StepDef.VerbatimVariants directly.
//
// Returns the step so calls keep chaining.
func WireRecipe(s *demokit.StepDef, curl, goSource string) *demokit.StepDef {
	return s.VerbatimVariants(WireLabel,
		demokit.MakeVariant("curl", "bash", curl).Default(),
		demokit.MakeVariant("go", "go", goSource),
	)
}

// SplitLines splits a multi-line string on "\n". It keeps step Run
// bodies focused on the narrative ("for _, line := range
// harness.SplitLines(s)") rather than line-iteration mechanics; it is a
// thin wrapper over strings.Split with no trimming.
func SplitLines(s string) []string {
	return strings.Split(s, "\n")
}
