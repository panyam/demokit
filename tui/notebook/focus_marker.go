package notebook

import "unicode/utf8"

// applyFocusMarker swaps the first rune of line for "▶" when
// focused is true and the line shape matches "<multi-byte glyph> <text>".
//
// The multi-byte restriction is what keeps the helper from chewing
// on body content: title markers (▸ / § / ❑ / …) are all ≥ 2 bytes,
// while body lines start with ASCII letters / digits / spaces (each
// a single byte) and pass through unchanged. New cell types can pick
// any multi-byte unfocused glyph they want — no per-character special
// casing in cells.
//
// Lines that don't match (body rows, blank padding, ASCII-led
// content) are returned as-is.
func applyFocusMarker(line string, focused bool) string {
	if !focused {
		return line
	}
	r, size := utf8.DecodeRuneInString(line)
	if r == utf8.RuneError || size < 2 || len(line) < size+1 || line[size] != ' ' {
		return line
	}
	return "▶" + line[size:]
}
