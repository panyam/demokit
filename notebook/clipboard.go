package notebook

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
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

// FileClipboard returns a Clipboard that writes the payload to a
// new file under dir and returns the path as the strategy string.
// Empty dir defaults to os.TempDir().
//
// Useful as a manual fallback when the terminal suppresses OSC 52
// (e.g. iTerm2 with "Applications in terminal may access clipboard"
// disabled). The OSC 52 write itself has no error channel back
// from the terminal, so FileClipboard exists for the user to escape
// to when the OSC52 toast lied.
//
// Composes with OutputCell's optional fallback hook (the 't' key
// in the OutputCell convention — see cells/output.go).
//
// The strategy string is the file path so the toast displays where
// the user can find it.
func FileClipboard(dir string) Clipboard {
	if dir == "" {
		dir = os.TempDir()
	}
	return func(content string) (string, bool) {
		f, err := os.CreateTemp(dir, "notebook-copy-*.txt")
		if err != nil {
			return "", false
		}
		path := f.Name()
		if _, err := f.WriteString(content); err != nil {
			_ = f.Close()
			_ = os.Remove(path)
			return "", false
		}
		if err := f.Close(); err != nil {
			return "", false
		}
		// Show just the leaf when dir is the platform tmp dir
		// (full path is long on macOS: /var/folders/...). Caller
		// can still reach it via /tmp on Unix because tmp is
		// usually symlinked. Return the absolute path so the
		// user can copy-paste into a shell.
		return filepath.Clean(path), true
	}
}

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
