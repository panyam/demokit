// Package cells provides the built-in widget library for the
// notebook component — HeaderCell, NoteCell, VerbatimCell,
// OutputCell, PromptCell. Each cell is a self-contained, styleable
// widget that implements notebook.Cell. Consumers are free to
// implement their own Cells alongside these.
package cells

import "charm.land/lipgloss/v2"

// focusedBorder returns the lipgloss border shape to apply for a
// cell at the given focus state. Both shapes occupy identical
// character positions so HeightHint stays accurate when focus
// moves between cells.
//
// Built-in cells use this; custom cells outside this package can
// pick their own focus signaling. Not exported because it's a
// convention of the built-in set, not a contract.
func focusedBorder(focused bool) lipgloss.Border {
	if focused {
		return lipgloss.ThickBorder()
	}
	return lipgloss.RoundedBorder()
}

// BorderEdges configures which sides of a cell's box draw a
// border line. Default zero value is all-off, which is rarely
// desired; cells expose typed style defaults instead. The
// AllEdges / HorizontalEdges presets cover the common cases.
type BorderEdges struct {
	Top, Right, Bottom, Left bool
}

// AllEdges returns a BorderEdges with all four sides on — the
// classic boxed cell look.
func AllEdges() BorderEdges {
	return BorderEdges{Top: true, Right: true, Bottom: true, Left: true}
}

// HorizontalEdges returns a BorderEdges with only top and bottom
// on — the "rule above / rule below" look. Useful for cells
// whose content users will copy-paste: no vertical bars get
// caught in the selection.
func HorizontalEdges() BorderEdges {
	return BorderEdges{Top: true, Bottom: true}
}

// innerWidth returns the content width for a lipgloss box of the
// given outer width with the given edges and a fixed Padding(0,1).
// Each on-edge consumes 1 char; padding consumes 2.
func innerWidth(outer int, edges BorderEdges) int {
	w := outer - 2 // Padding(0,1)
	if edges.Left {
		w--
	}
	if edges.Right {
		w--
	}
	if w < 10 {
		w = 10
	}
	return w
}

// chromeRows returns the count of non-body rows the box consumes
// at the given edges (top border + bottom border). Cells add their
// own title / status / etc. on top.
func chromeRows(edges BorderEdges) int {
	n := 0
	if edges.Top {
		n++
	}
	if edges.Bottom {
		n++
	}
	return n
}
