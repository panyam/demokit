package notebook

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/panyam/demokit"
)

// VerbatimCell renders one verbatim block — possibly with multiple
// variants shown as a tab strip + a single active variant's body.
// In focused / view mode:
//
//   - Tab / Shift+Tab cycle the active variant (wrap).
//   - '1'..'9' jump directly (out-of-range ignored).
//   - 'c' copies the active variant's Content via demokit.Copy.
//
// Single-variant blocks omit the tab strip but still respect 'c'.
type VerbatimCell struct {
	id        string
	label     string
	variants  []demokit.Variant
	active    int

	cachedWidth  int
	cachedLines  []string
	cachedHeight int

	copyMsg string
}

// NewVerbatimCell builds a cell from a flat slice of demokit.Variants.
// The active variant is whichever carries IsDefault (first wins), or
// 0 if none does.
func NewVerbatimCell(id, label string, variants []demokit.Variant) *VerbatimCell {
	active := 0
	for i, v := range variants {
		if v.IsDefault {
			active = i
			break
		}
	}
	return &VerbatimCell{id: id, label: label, variants: variants, active: active}
}

// ID implements Cell.
func (c *VerbatimCell) ID() string { return c.id }

// HeightHint implements Cell.
func (c *VerbatimCell) HeightHint(width int) int {
	c.materialize(width)
	h := c.cachedHeight
	if c.copyMsg != "" {
		h++
	}
	return h
}

// RenderRows implements Cell.
func (c *VerbatimCell) RenderRows(width, startRow, endRow int, focused bool, mode Mode) []string {
	c.materialize(width)
	total := c.cachedHeight
	if c.copyMsg != "" {
		total++
	}
	if startRow < 0 {
		startRow = 0
	}
	if endRow > total {
		endRow = total
	}
	if startRow >= endRow {
		return nil
	}
	rows := make([]string, endRow-startRow)
	for i := startRow; i < endRow; i++ {
		var line string
		if i < c.cachedHeight {
			line = c.cachedLines[i]
		} else {
			line = "  " + c.copyMsg
		}
		rows[i-startRow] = applyFocusMarker(line, focused)
	}
	return rows
}

// Update implements Cell.
func (c *VerbatimCell) Update(msg tea.Msg, mode Mode) (Cell, tea.Cmd) {
	if cm, ok := msg.(clearCopyMsg); ok && cm.cellID == c.id {
		c.copyMsg = ""
		return c, nil
	}
	if mode != ViewMode {
		return c, nil
	}
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return c, nil
	}
	switch keyMsg.String() {
	case "tab":
		c.cycle(+1)
	case "shift+tab":
		c.cycle(-1)
	case "c":
		v := c.variants[c.active]
		strategy, ok := demokit.Copy(v.Content)
		if ok {
			label := v.Label
			if label == "" {
				c.copyMsg = "(copied via " + strategy + ")"
			} else {
				c.copyMsg = "(copied " + label + " via " + strategy + ")"
			}
		} else {
			c.copyMsg = "(copy failed — no clipboard provider)"
		}
		return c, clearCopyMsgAfter(c.id)
	default:
		// 1-9 jumps. Only single-digit keys.
		s := keyMsg.String()
		if len(s) == 1 && s[0] >= '1' && s[0] <= '9' {
			idx := int(s[0]-'1')
			if idx < len(c.variants) {
				c.active = idx
				c.cachedLines = nil // body changed → rerender
			}
		}
	}
	return c, nil
}

// StatusHint implements Cell. Wording adapts to the number of
// variants so the user isn't told about keys that do nothing.
func (c *VerbatimCell) StatusHint(_ Mode) string {
	if len(c.variants) <= 1 {
		return "c copy"
	}
	return fmt.Sprintf("Tab cycle · 1-%d jump · c copy", len(c.variants))
}

func (c *VerbatimCell) cycle(delta int) {
	n := len(c.variants)
	if n == 0 {
		return
	}
	c.active = (c.active + delta + n) % n
	c.cachedLines = nil // body changed → rerender
}

func (c *VerbatimCell) materialize(width int) {
	if width <= 0 {
		width = 80
	}
	if c.cachedWidth == width && c.cachedLines != nil {
		return
	}
	var rows []string
	rows = append(rows, "")
	// Label row.
	header := "❑ " + c.label
	if c.label == "" {
		header = "❑ verbatim"
	}
	rows = append(rows, header)
	// Tab strip — single-variant blocks skip it.
	if len(c.variants) > 1 {
		var tabs []string
		for i, v := range c.variants {
			name := v.Label
			if name == "" {
				name = fmt.Sprintf("#%d", i+1)
			}
			if i == c.active {
				tabs = append(tabs, "["+name+"]")
			} else {
				tabs = append(tabs, " "+name+" ")
			}
		}
		rows = append(rows, "  "+strings.Join(tabs, " "))
		rows = append(rows, "")
	} else {
		rows = append(rows, "")
	}
	// Active body — split into lines, truncated/wrapped to width.
	body := c.variants[c.active].Content
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimRight(line, "\r")
		// Hard-wrap rather than soft-wrap — code is layout-sensitive,
		// wrapping mid-token would lie about the snippet.
		if len(line) > width-4 {
			line = line[:width-4]
		}
		rows = append(rows, "    "+line)
	}
	rows = append(rows, "")
	c.cachedWidth = width
	c.cachedLines = rows
	c.cachedHeight = len(rows)
}
