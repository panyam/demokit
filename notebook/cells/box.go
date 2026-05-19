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

// maxBoxWidth returns the inner content width for a lipgloss
// bordered box at the given outer width: 2 chars for vertical
// borders + 2 chars horizontal padding = 4 reserved. Clamped to a
// minimum so narrow terminals don't blow up wrap logic.
func maxBoxWidth(outer int) int {
	w := outer - 4
	if w < 10 {
		w = 10
	}
	return w
}
