package notebook

import (
	"encoding/base64"
	"fmt"
	"os"
)

// Clipboard is the injection point cells call when the user presses
// 'c'. Returns (strategy, ok) — strategy is a human-readable label
// like "OSC52" or "pbcopy" that the cell displays in its toast;
// ok is whether the write succeeded.
//
// Decoupled from any specific clipboard implementation so the
// notebook package depends only on stdlib + charm. Callers wire
// their own implementation via WithClipboard or Cell.SetClipboard.
type Clipboard func(content string) (strategy string, ok bool)

// NoClipboard always fails. Used as the default when no clipboard
// is injected — cells render a "(copy failed — no clipboard
// provider)" toast.
var NoClipboard Clipboard = func(string) (string, bool) { return "", false }

// OSC52Clipboard returns a Clipboard that writes via the OSC 52
// terminal escape sequence. The escape is consumed by the
// terminal emulator (kitty, iTerm2, Alacritty, WezTerm, recent
// Apple Terminal, tmux with config, etc.) and copies into the
// system clipboard — no extra binaries or platform integration
// needed.
//
// The escape is written to os.Stderr to keep it out of the
// alt-screen buffer that bubbletea owns; terminals that don't
// understand OSC 52 silently ignore it (no visible garbage in
// well-behaved terminals).
//
// Use as a one-liner default:
//
//	nb := notebook.New(notebook.WithClipboard(notebook.OSC52Clipboard()))
func OSC52Clipboard() Clipboard {
	return func(content string) (string, bool) {
		encoded := base64.StdEncoding.EncodeToString([]byte(content))
		// \x1b]52;c;<base64>\x07 is the standard OSC 52 form
		// for the "c" (clipboard) selection.
		fmt.Fprintf(os.Stderr, "\x1b]52;c;%s\x07", encoded)
		return "OSC52", true
	}
}
