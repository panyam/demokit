package notebook

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// LineSource is the width-parameterized row layout an OutputCell
// renders from. It maps a buffer of logical lines onto the visual
// rows that actually paint at a given column width — wrapping long
// lines, accounting for ANSI/wide characters — and answers the two
// queries a scrolling viewport needs:
//
//   - RowCount(width): how many visual rows at this width (height,
//     scrollbar, follow/clamp math).
//   - Rows(width, start, end): just the visible window [start, end).
//
// The cell programs against this interface, not against a concrete
// buffer, so the layout implementation is swappable without
// touching the cell. The default EagerLineSource re-wraps the whole
// buffer on every call — correct and simple, fine for the dozens-
// to-hundreds of rows a notebook cell holds. A large, continuously
// growing scrollback (a streaming log/CLI tail) wants an indexed
// implementation: keep per-line visual-row counts in a Fenwick tree
// (binary indexed tree) so RowCount is a prefix sum and "which
// logical line owns visual row R" is a prefix-sum lower-bound, both
// O(log N), with O(log N) updates on append. That impl plugs in
// here with no cell changes. See the design issue for the plan.
type LineSource interface {
	// RowCount returns the number of visual rows at width columns.
	RowCount(width int) int
	// Rows returns visual rows in the half-open range [start, end),
	// clamped to the available range. Out-of-range returns nil.
	Rows(width, start, end int) []string
}

// EagerLineSource is the no-cache LineSource: it wraps every logical
// line in its backing OutputBuffer on each query. Storage stays in
// the OutputBuffer (the io.Writer streaming sink); this type only
// owns layout, so swapping it for an indexed source leaves the
// write path untouched.
type EagerLineSource struct {
	buf *OutputBuffer
}

// NewEagerLineSource returns an EagerLineSource laying out buf.
func NewEagerLineSource(buf *OutputBuffer) *EagerLineSource {
	return &EagerLineSource{buf: buf}
}

// RowCount implements LineSource.
func (s *EagerLineSource) RowCount(width int) int {
	return len(s.rows(width))
}

// Rows implements LineSource.
func (s *EagerLineSource) Rows(width, start, end int) []string {
	rows := s.rows(width)
	if start < 0 {
		start = 0
	}
	if end > len(rows) {
		end = len(rows)
	}
	if start >= end {
		return nil
	}
	out := make([]string, end-start)
	copy(out, rows[start:end])
	return out
}

// rows wraps the buffer's logical lines to width visible columns.
func (s *EagerLineSource) rows(width int) []string {
	if width < 1 {
		width = 1
	}
	logical := s.buf.Lines(0, s.buf.LineCount())
	rows := make([]string, 0, len(logical))
	for _, line := range logical {
		rows = append(rows, wrapANSILine(strings.TrimRight(line, "\r"), width)...)
	}
	return rows
}

// wrapANSILine hard-wraps one logical line to width visible columns,
// carrying SGR color across row breaks. ansi.Hardwrap keeps escape
// bytes where they sit but does not reopen the active style on the
// next row, so a long colored run would lose its color after the
// first break; we reopen the active SGR at the start of each
// continuation row and close it at the end of the prior one.
func wrapANSILine(line string, width int) []string {
	rows := strings.Split(ansi.Hardwrap(line, width, false), "\n")
	if len(rows) <= 1 {
		return rows
	}
	active := ""
	for i := range rows {
		if i > 0 && active != "" {
			rows[i] = active + rows[i]
		}
		active = activeSGRAfter(active, rows[i])
		if i < len(rows)-1 && active != "" {
			rows[i] += "\x1b[0m"
		}
	}
	return rows
}

// activeSGRAfter returns the SGR escape sequences still in effect at
// the end of s, given those active before it. A reset ("\x1b[0m" /
// "\x1b[m") clears the set; any other SGR sequence is appended. It
// tracks the running prefix rather than a full attribute model —
// enough to reopen color/style on a wrapped continuation row.
func activeSGRAfter(before, s string) string {
	active := before
	for i := 0; i < len(s); {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
				j++
			}
			if j < len(s) {
				if seq := s[i : j+1]; s[j] == 'm' {
					if seq == "\x1b[0m" || seq == "\x1b[m" {
						active = ""
					} else {
						active += seq
					}
				}
				i = j + 1
				continue
			}
		}
		i++
	}
	return active
}
