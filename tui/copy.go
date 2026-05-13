package tui

import (
	"fmt"
	"strings"

	"github.com/panyam/demokit"
)

// copyableBlock is a flat handle for the pause-loop copy parser. It
// pairs a verbatim block with its index inside the step (used for
// future addressability) so error messages can refer to the block
// unambiguously.
type copyableBlock struct {
	index int
	view  demokit.VerbatimView
}

// copyableBlocks returns the verbatim blocks on step that participate
// in the line-based copy command — i.e. blocks rendered inside the
// TUI's bordered box. Single-variant blocks are copyable only when the
// demo set Demo.BoxedVerbatim(). Multi-variant blocks are always
// copyable (they're auto-boxed regardless of the flag).
func (r *Renderer) copyableBlocks(step *demokit.StepDef) []copyableBlock {
	if step == nil {
		return nil
	}
	demo := step.Demo()
	boxedDefault := demo != nil && demo.IsBoxedVerbatim()
	var out []copyableBlock
	for i, v := range step.VerbatimBlocks() {
		if len(v.Variants) == 0 {
			continue
		}
		if !(boxedDefault || len(v.Variants) > 1) {
			continue
		}
		out = append(out, copyableBlock{index: i, view: v})
	}
	return out
}

// copyPromptHint builds the one-line prompt shown at the pause. The
// hint adapts to what's actually copyable on the step — no copy hint
// when there's nothing to copy, a simple "c to copy" for single-
// variant cases, and a labeled form when variants are present.
func copyPromptHint(copyables []copyableBlock) string {
	if len(copyables) == 0 {
		return "  Press Enter to run this step..."
	}
	// Look for any multi-variant block to decide whether to expose the
	// "c <label>" form in the hint.
	hasVariants := false
	for _, c := range copyables {
		if len(c.view.Variants) > 1 {
			hasVariants = true
			break
		}
	}
	if hasVariants {
		return "  Press Enter to run · type `c` to copy default · `c <label>` for a named variant"
	}
	return "  Press Enter to run · type `c` to copy"
}

// handleCopyCommand parses one line typed at the pause and executes
// the requested action against the available copyable blocks. Returns
// a status line to display after the action ("(copied via osc52)",
// error, etc.); the caller re-prompts after printing it.
//
// Recognized commands (case-insensitive):
//
//	c           → copy the first copyable block's default variant
//	              (or first variant if none marked default)
//	c <label>   → copy the variant whose Label matches <label> from the
//	              first copyable block carrying that label. Useful when
//	              the step has only one block; if multiple blocks share
//	              a label, the earliest in declaration order wins.
//
// Anything else returns "(unknown command)" so the user sees the
// prompt is interactive rather than waiting on raw input.
func (r *Renderer) handleCopyCommand(cmd string, copyables []copyableBlock) string {
	if len(copyables) == 0 {
		return "(no copyable blocks on this step)"
	}
	parts := strings.Fields(cmd)
	if len(parts) == 0 || !strings.EqualFold(parts[0], "c") {
		return fmt.Sprintf("(unknown command %q — type `c` to copy or press Enter to continue)", cmd)
	}

	// Bare "c" → first copyable block, default-or-first variant.
	if len(parts) == 1 {
		block := copyables[0]
		va := pickDefaultVariant(block.view.Variants)
		return performCopy(block.view.Label, va)
	}

	// "c <label>" → find first block with a matching variant label.
	wanted := strings.Join(parts[1:], " ")
	for _, c := range copyables {
		for _, va := range c.view.Variants {
			if strings.EqualFold(va.Label, wanted) {
				return performCopy(c.view.Label, va)
			}
		}
	}
	return fmt.Sprintf("(no variant labeled %q on this step)", wanted)
}

// pickDefaultVariant returns the variant marked IsDefault, or the
// first variant when none is marked. variants must be non-empty —
// callers filter empty blocks out of copyables.
func pickDefaultVariant(variants []demokit.VariantView) demokit.VariantView {
	for _, v := range variants {
		if v.IsDefault {
			return v
		}
	}
	return variants[0]
}

// performCopy invokes the clipboard primitive on va.Content and
// returns the status line to display. Includes the variant's label
// in the message when present so the user sees which form they
// grabbed.
func performCopy(blockLabel string, va demokit.VariantView) string {
	strategy, ok := demokit.Copy(va.Content)
	if !ok {
		return "(copy failed — no clipboard provider available)"
	}
	what := va.Label
	if what == "" {
		what = blockLabel
	}
	if what == "" {
		return fmt.Sprintf("(copied via %s)", strategy)
	}
	return fmt.Sprintf("(copied %q via %s)", what, strategy)
}
