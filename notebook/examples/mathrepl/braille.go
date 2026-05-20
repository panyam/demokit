package main

import "strings"

// brailleBase is U+2800 — the empty braille pattern. Each set dot
// adds the dot's bit value (0x01..0x80) to the codepoint.
const brailleBase = 0x2800

// brailleBit maps a (dotCol, dotRow) inside one braille character
// (2 cols × 4 rows) to its bit position in the codepoint.
//
//	col 0       col 1
//	bit 0       bit 3
//	bit 1       bit 4
//	bit 2       bit 5
//	bit 6       bit 7
var brailleBit = [4][2]int{
	{0, 3},
	{1, 4},
	{2, 5},
	{6, 7},
}

// Canvas is a dot grid that renders as braille characters. Each
// braille character covers 2 columns × 4 rows of dots.
type Canvas struct {
	dotsW, dotsH int
	pixels       []bool
}

// NewCanvas returns a Canvas sized for charsW × charsH braille
// characters — internally that's (charsW*2) × (charsH*4) dots.
func NewCanvas(charsW, charsH int) *Canvas {
	if charsW < 1 {
		charsW = 1
	}
	if charsH < 1 {
		charsH = 1
	}
	w, h := charsW*2, charsH*4
	return &Canvas{dotsW: w, dotsH: h, pixels: make([]bool, w*h)}
}

// Set lights the dot at (x, y). Out-of-range coords are a silent
// no-op so plot loops don't need bounds checks.
func (c *Canvas) Set(x, y int) {
	if x < 0 || x >= c.dotsW || y < 0 || y >= c.dotsH {
		return
	}
	c.pixels[y*c.dotsW+x] = true
}

// Render returns one string per braille row (dotsH/4 rows total).
// Each row is dotsW/2 braille chars wide.
func (c *Canvas) Render() []string {
	charsW := c.dotsW / 2
	charsH := c.dotsH / 4
	out := make([]string, charsH)
	for cy := 0; cy < charsH; cy++ {
		var sb strings.Builder
		sb.Grow(charsW * 3) // braille chars are 3 UTF-8 bytes each
		for cx := 0; cx < charsW; cx++ {
			ch := rune(brailleBase)
			for dy := 0; dy < 4; dy++ {
				for dx := 0; dx < 2; dx++ {
					px := cx*2 + dx
					py := cy*4 + dy
					if c.pixels[py*c.dotsW+px] {
						ch |= rune(1 << brailleBit[dy][dx])
					}
				}
			}
			sb.WriteRune(ch)
		}
		out[cy] = sb.String()
	}
	return out
}
