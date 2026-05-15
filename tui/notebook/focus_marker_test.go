package notebook

import "testing"

func TestApplyFocusMarkerSwapsMultiByteGlyph(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"meta glyph", "▸ Title", "▶ Title"},
		{"section glyph", "§ Heads up", "▶ Heads up"},
		{"verbatim glyph", "❑ Refresh", "▶ Refresh"},
		{"already-focused glyph (no-op semantic)", "▶ Already", "▶ Already"},
		{"emoji glyph", "🎯 Goal", "▶ Goal"},
	}
	for _, tt := range tests {
		if got := applyFocusMarker(tt.in, true); got != tt.want {
			t.Errorf("%s: applyFocusMarker(%q, true) = %q, want %q", tt.name, tt.in, got, tt.want)
		}
	}
}

func TestApplyFocusMarkerLeavesUnfocusedAlone(t *testing.T) {
	in := "▸ Title"
	if got := applyFocusMarker(in, false); got != in {
		t.Errorf("focused=false should be a no-op: got %q, want %q", got, in)
	}
}

func TestApplyFocusMarkerPassesThroughASCIILed(t *testing.T) {
	// Body content shapes that look like "rune + space + rest" but
	// must NOT be munged — the multi-byte restriction guards them.
	cases := []string{
		"A -> B: label",      // arrow line in MetaCell body
		"    curl -s ...",    // indented code in VerbatimCell body
		"  - bullet",         // bullet in section body
		"x ",                 // single char + trailing space
		"",                   // empty
		"  ",                 // whitespace only
	}
	for _, in := range cases {
		if got := applyFocusMarker(in, true); got != in {
			t.Errorf("ASCII-led %q should pass through; got %q", in, got)
		}
	}
}

func TestApplyFocusMarkerLeavesNonShapedLinesAlone(t *testing.T) {
	// Multi-byte first rune but no following space → not a title row.
	cases := []string{
		"▸Title",   // missing space
		"▶",        // single glyph, no rest
		"§\tTitle", // tab instead of space
	}
	for _, in := range cases {
		if got := applyFocusMarker(in, true); got != in {
			t.Errorf("non-shaped %q should pass through; got %q", in, got)
		}
	}
}
