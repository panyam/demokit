package tui

import (
	"fmt"
	"strconv"
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
// variant cases, and the full switch/copy syntax when variants are
// present.
func copyPromptHint(copyables []copyableBlock) string {
	if len(copyables) == 0 {
		return "  Press Enter to run this step..."
	}
	// Look for any multi-variant block to decide whether to expose the
	// switch/<label> forms in the hint.
	hasVariants := false
	for _, c := range copyables {
		if len(c.view.Variants) > 1 {
			hasVariants = true
			break
		}
	}
	if hasVariants {
		return "  Press Enter to run · `<label>` or `<n>` to switch · `c` copy active · `c <label>` copy named"
	}
	return "  Press Enter to run · type `c` to copy"
}

// handleCopyCommand parses one line typed at the pause and executes
// the requested action against the available copyable blocks. Returns
// a status line to display after the action ("(copied via osc52)",
// "(switched to python)", error, etc.); the caller re-prompts after
// printing it. The bool return is true when the caller should
// re-render the active variant inline (switch happened) — the prompt
// loop handles that.
//
// Recognized commands (case-insensitive):
//
//	c             → copy the first copyable block's currently-active
//	                variant. Active starts at the Default-marked
//	                variant (or the first variant if none is marked).
//	c <label>     → copy a variant by label without switching active.
//	                Matches across all copyable blocks, earliest wins.
//	<label>       → switch the first multi-variant block's active to
//	                this label.
//	<n>           → switch by 1-based index within the first
//	                multi-variant block.
//
// Anything else returns "(unknown command)" so the user sees the
// prompt is interactive rather than waiting on raw input.
func (r *Renderer) handleCopyCommand(cmd string, copyables []copyableBlock) (msg string, switched bool) {
	if len(copyables) == 0 {
		return "(no copyable blocks on this step)", false
	}
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return "", false
	}
	first := strings.ToLower(parts[0])

	switch first {
	case "c":
		// Copy commands.
		if len(parts) == 1 {
			block := copyables[0]
			va := block.view.Variants[r.activeIndex(block.index)]
			return performCopy(block.view.Label, va), false
		}
		wanted := strings.Join(parts[1:], " ")
		for _, c := range copyables {
			for _, va := range c.view.Variants {
				if strings.EqualFold(va.Label, wanted) {
					return performCopy(c.view.Label, va), false
				}
			}
		}
		return fmt.Sprintf("(no variant labeled %q on this step)", wanted), false
	}

	// Switch commands — only meaningful for multi-variant blocks.
	target := r.firstMultiVariantBlock(copyables)
	if target == nil {
		return fmt.Sprintf("(unknown command %q — type `c` to copy or press Enter to continue)", cmd), false
	}
	if idx, ok := matchVariant(cmd, target.view.Variants); ok {
		if r.activeVariant == nil {
			r.activeVariant = map[int]int{}
		}
		r.activeVariant[target.index] = idx
		label := target.view.Variants[idx].Label
		if label == "" {
			label = fmt.Sprintf("variant %d", idx+1)
		}
		return fmt.Sprintf("(switched to %s)", label), true
	}
	return fmt.Sprintf("(unknown command %q — type `c` to copy, a variant label/number to switch, or press Enter to continue)", cmd), false
}

// firstMultiVariantBlock returns the first copyable block that
// actually has tabs (more than one variant); single-variant blocks
// have no switch target.
func (r *Renderer) firstMultiVariantBlock(copyables []copyableBlock) *copyableBlock {
	for i := range copyables {
		if len(copyables[i].view.Variants) > 1 {
			return &copyables[i]
		}
	}
	return nil
}

// matchVariant resolves a switch token (`<label>` or 1-based number)
// to a variant index. Returns the index and true on match.
func matchVariant(token string, variants []demokit.VariantView) (int, bool) {
	token = strings.TrimSpace(token)
	if n, err := strconv.Atoi(token); err == nil {
		if n >= 1 && n <= len(variants) {
			return n - 1, true
		}
		return 0, false
	}
	for i, v := range variants {
		if strings.EqualFold(v.Label, token) {
			return i, true
		}
	}
	return 0, false
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
