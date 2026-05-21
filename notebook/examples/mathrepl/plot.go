package main

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/panyam/demokit/notebook"
)

// plotBodyRows is the braille plot's body height (in terminal
// rows). Total cell height = plotBodyRows + 4 (border + title +
// status + border).
const plotBodyRows = 12

// plotRe parses `plot <expr> from <a> to <b>`. <a> and <b> may
// themselves be expressions (e.g. `pi*2`); the REPL evaluates
// them through the same Env.
var plotRe = regexp.MustCompile(`^\s*plot\s+(.+?)\s+from\s+(.+?)\s+to\s+(.+?)\s*$`)

// PlotCell renders an expression as a braille plot over [a, b].
// Implements notebook.Cell. Read-only — the cell is built once
// from a `plot ...` command and never re-evaluates (unlike a
// "live" plot which would re-sample on a tick).
type PlotCell struct {
	id   string
	expr string
	a, b float64

	cachedWidth int
	cached      []string
	env         *Env
}

// NewPlotCell parses src and returns a PlotCell, evaluating the
// `from` / `to` bounds against env. Returns an error if src
// doesn't match the plot grammar or the bounds don't evaluate.
func NewPlotCell(id, src string, env *Env) (*PlotCell, error) {
	m := plotRe.FindStringSubmatch(src)
	if m == nil {
		return nil, fmt.Errorf("plot syntax: plot <expr> from <a> to <b>")
	}
	aVal, err := env.Eval(m[2])
	if err != nil {
		return nil, fmt.Errorf("plot 'from': %s", err)
	}
	bVal, err := env.Eval(m[3])
	if err != nil {
		return nil, fmt.Errorf("plot 'to': %s", err)
	}
	return &PlotCell{
		id:   id,
		expr: strings.TrimSpace(m[1]),
		a:    AsFloat(aVal),
		b:    AsFloat(bVal),
		env:  env,
	}, nil
}

// ID implements notebook.Cell.
func (c *PlotCell) ID() string { return c.id }

// HeightHint implements notebook.Cell.
func (c *PlotCell) HeightHint(width int) int {
	c.materialize(width)
	return len(c.cached)
}

// RenderRows implements notebook.Cell.
func (c *PlotCell) RenderRows(width, startRow, endRow int, _ bool, _ notebook.Mode) []string {
	c.materialize(width)
	if startRow < 0 {
		startRow = 0
	}
	if endRow > len(c.cached) {
		endRow = len(c.cached)
	}
	if startRow >= endRow {
		return nil
	}
	out := make([]string, endRow-startRow)
	copy(out, c.cached[startRow:endRow])
	return out
}

// Update implements notebook.Cell. Read-only; Esc in CellActiveMode
// releases focus by convention. Other keys passthrough.
func (c *PlotCell) Update(msg tea.Msg, mode notebook.Mode) (notebook.Cell, tea.Cmd, bool) {
	if k, ok := msg.(tea.KeyMsg); ok && k.String() == "esc" && mode == notebook.CellActiveMode {
		return c, notebook.ReleaseFocus, true
	}
	return c, nil, false
}

// StatusHint implements notebook.Cell.
func (c *PlotCell) StatusHint(notebook.Mode) string { return "" }

func (c *PlotCell) materialize(width int) {
	if c.cached != nil && c.cachedWidth == width {
		return
	}
	inner := boxInnerWidth(width)
	canvas := NewCanvas(inner, plotBodyRows)
	dotsW := inner * 2
	dotsH := plotBodyRows * 4

	samples := make([]float64, dotsW)
	minY, maxY := math.Inf(1), math.Inf(-1)
	for i := 0; i < dotsW; i++ {
		t := float64(i) / float64(dotsW-1)
		x := c.a + t*(c.b-c.a)
		c.env.SetVar("x", x)
		v, err := c.env.evalExpr(c.expr)
		if err != nil {
			samples[i] = math.NaN()
			continue
		}
		f := AsFloat(v)
		samples[i] = f
		if !math.IsNaN(f) && !math.IsInf(f, 0) {
			if f < minY {
				minY = f
			}
			if f > maxY {
				maxY = f
			}
		}
	}

	// Plot samples onto the canvas.
	yRange := maxY - minY
	if math.IsInf(minY, 0) || math.IsInf(maxY, 0) || yRange == 0 {
		yRange = 1
		if math.IsInf(maxY, 0) {
			maxY = 0
		}
	}
	for i, y := range samples {
		if math.IsNaN(y) || math.IsInf(y, 0) {
			continue
		}
		// maxY → top of canvas (dot row 0); minY → bottom.
		norm := (maxY - y) / yRange
		row := int(norm * float64(dotsH-1))
		canvas.Set(i, row)
	}

	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FAFAFA")).
		Render(fmt.Sprintf("plot %s", c.expr))
	status := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).
		Render(fmt.Sprintf("from %s to %s", formatBound(c.a), formatBound(c.b)))

	body := strings.Join(canvas.Render(), "\n")
	content := title + "\n" + body + "\n" + status

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#5BB1FF")).
		Padding(0, 1).
		Width(inner).
		Render(content)
	c.cached = strings.Split(box, "\n")
	c.cachedWidth = width
}

// formatBound trims trailing zeros from a float for compact
// display in the plot's status row.
func formatBound(f float64) string {
	s := strconv.FormatFloat(f, 'f', -1, 64)
	return s
}
