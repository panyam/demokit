package notebook

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// MetaCell is the always-present step-header cell — title plus the
// step's body of arrows / refs / note rendered as plain wrapped
// text. Read-only in Phase A; future EditMode would let the user
// rename or rewrite the note.
//
// Held immutable after construction (the demo's *Step content
// doesn't mutate during the trace). Width caching keeps repeated
// HeightHint calls cheap.
type MetaCell struct {
	id    string
	title string
	// body is the rendered, wrap-ready body text (arrows + refs +
	// note, joined with blank lines). NotebookRenderer pre-formats
	// it from the StepDef so MetaCell doesn't need a back-reference
	// to demokit types in Phase A.
	body string

	// width cache: lines/heightForWidth describe the body rendered
	// at the given width. Invalidated on width change.
	cachedWidth  int
	cachedLines  []string
	cachedHeight int
}

// NewMetaCell builds a MetaCell. title is the step's display title;
// body is the rendered prose (already joined). id should be unique
// across the trace (e.g. "step.name#visit0.meta").
func NewMetaCell(id, title, body string) *MetaCell {
	return &MetaCell{id: id, title: title, body: body}
}

// ID implements Cell.
func (c *MetaCell) ID() string { return c.id }

// HeightHint implements Cell. The cell renders as: blank line +
// "▸ TITLE" + blank line + wrapped body lines + trailing blank.
func (c *MetaCell) HeightHint(width int) int {
	c.materialize(width)
	return c.cachedHeight
}

// RenderRows implements Cell — returns the half-open row range.
// Clamped to availability so a viewport asking past the end just
// gets fewer rows back.
func (c *MetaCell) RenderRows(width, startRow, endRow int, focused bool, mode Mode) []string {
	c.materialize(width)
	if startRow < 0 {
		startRow = 0
	}
	if endRow > c.cachedHeight {
		endRow = c.cachedHeight
	}
	if startRow >= endRow {
		return nil
	}
	// Apply focus prefix on the title row only — '▸' becomes '▶' so
	// the user can see which cell holds the cursor.
	rows := make([]string, endRow-startRow)
	for i := startRow; i < endRow; i++ {
		line := c.cachedLines[i]
		if focused && strings.HasPrefix(line, "▸ ") {
			line = "▶ " + line[len("▸ "):]
		}
		rows[i-startRow] = line
	}
	return rows
}

// Update implements Cell. MetaCell is read-only in Phase A — no key
// handling at all, including Esc (the model handles Esc at the
// outer level by popping mode).
func (c *MetaCell) Update(_ tea.Msg, _ Mode) (Cell, tea.Cmd) {
	return c, nil
}

// StatusHint implements Cell — MetaCell exposes only the
// advance-step gesture, since it has no per-cell action.
func (c *MetaCell) StatusHint(_ Mode) string {
	return "Space/Shift+Enter advance"
}

// materialize lazily rebuilds cachedLines/cachedHeight when width
// changes. Cheap on cache hit; the body wrap is the only work on
// miss and even that's bounded by the cell's text length.
func (c *MetaCell) materialize(width int) {
	if width <= 0 {
		width = 80
	}
	if c.cachedWidth == width && c.cachedLines != nil {
		return
	}
	var rows []string
	rows = append(rows, "")
	rows = append(rows, "▸ "+c.title)
	rows = append(rows, "")
	if strings.TrimSpace(c.body) != "" {
		for _, line := range wrapPlain(c.body, width-2) {
			rows = append(rows, "  "+line)
		}
		rows = append(rows, "")
	}
	c.cachedWidth = width
	c.cachedLines = rows
	c.cachedHeight = len(rows)
}

// wrapPlain is the package-local wrap helper used by Meta/Section
// cells. Word-wraps each input paragraph (separated by '\n') to
// max width. Blank input lines become single blank output lines.
// Keep this tiny — cells do their own indentation outside of it.
func wrapPlain(s string, width int) []string {
	if width <= 4 {
		width = 4
	}
	var out []string
	for _, para := range strings.Split(s, "\n") {
		if strings.TrimSpace(para) == "" {
			out = append(out, "")
			continue
		}
		words := strings.Fields(para)
		if len(words) == 0 {
			out = append(out, "")
			continue
		}
		line := words[0]
		for _, w := range words[1:] {
			if len(line)+1+len(w) > width {
				out = append(out, line)
				line = w
				continue
			}
			line = line + " " + w
		}
		out = append(out, line)
	}
	return out
}
