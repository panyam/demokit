package main

import "testing"

func TestCanvasEmptyRendersBlankBraille(t *testing.T) {
	c := NewCanvas(2, 1)
	rows := c.Render()
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	want := string([]rune{brailleBase, brailleBase})
	if rows[0] != want {
		t.Errorf("empty render = %q, want %q", rows[0], want)
	}
}

func TestCanvasSingleDotEncodesCorrectBit(t *testing.T) {
	// (x, y) → expected codepoint = brailleBase + 1<<bit
	cases := []struct {
		x, y int
		want rune
	}{
		{0, 0, brailleBase | 1<<0},
		{0, 1, brailleBase | 1<<1},
		{0, 2, brailleBase | 1<<2},
		{1, 0, brailleBase | 1<<3},
		{1, 1, brailleBase | 1<<4},
		{1, 2, brailleBase | 1<<5},
		{0, 3, brailleBase | 1<<6},
		{1, 3, brailleBase | 1<<7},
	}
	for _, tc := range cases {
		c := NewCanvas(1, 1)
		c.Set(tc.x, tc.y)
		got := []rune(c.Render()[0])[0]
		if got != tc.want {
			t.Errorf("Set(%d,%d) = %U, want %U", tc.x, tc.y, got, tc.want)
		}
	}
}

func TestCanvasAllEightDotsRendersFullBraille(t *testing.T) {
	c := NewCanvas(1, 1)
	for dy := 0; dy < 4; dy++ {
		for dx := 0; dx < 2; dx++ {
			c.Set(dx, dy)
		}
	}
	got := []rune(c.Render()[0])[0]
	if got != 0x28FF {
		t.Errorf("all 8 dots = %U, want U+28FF", got)
	}
}

func TestCanvasOutOfBoundsSetIsNoop(t *testing.T) {
	c := NewCanvas(1, 1)
	c.Set(-1, 0)
	c.Set(0, 100)
	c.Set(100, 0)
	if got := []rune(c.Render()[0])[0]; got != brailleBase {
		t.Errorf("out-of-bounds Set affected render: %U", got)
	}
}

func TestCanvasRendersMultipleRows(t *testing.T) {
	c := NewCanvas(3, 2) // 6 dot-cols × 8 dot-rows
	rows := c.Render()
	if len(rows) != 2 {
		t.Errorf("rows = %d, want 2", len(rows))
	}
	for i, r := range rows {
		if got := []rune(r); len(got) != 3 {
			t.Errorf("row %d width = %d chars, want 3", i, len(got))
		}
	}
}
