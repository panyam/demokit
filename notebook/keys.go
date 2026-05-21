package notebook

// Canonical key-string constants for use in KeyMap bindings.
// These match what tea.KeyMsg.String() returns, so they're safe
// to use as map keys — same lookup as a raw string literal but
// with autocomplete + spelling safety.
//
// Apps can still pass raw strings for combos not listed here
// (e.g. "ctrl+alt+f4", "alt+shift+page"). The constants cover the
// common cases; anything tea.KeyMsg can stringify works as a
// binding key.
const (
	KeyEnter     = "enter"
	KeyEsc       = "esc"
	KeySpace     = " "
	KeyTab       = "tab"
	KeyShiftTab  = "shift+tab"
	KeyUp        = "up"
	KeyDown      = "down"
	KeyLeft      = "left"
	KeyRight     = "right"
	KeyHome      = "home"
	KeyEnd       = "end"
	KeyPgUp      = "pgup"
	KeyPgDown    = "pgdown"
	KeyBackspace = "backspace"
	KeyDelete    = "delete"
	KeyInsert    = "insert"
	KeyCtrlC     = "ctrl+c"
	KeyCtrlD     = "ctrl+d"
	KeyCtrlZ     = "ctrl+z"
	KeyCtrlR     = "ctrl+r"
	KeyCtrlL     = "ctrl+l"
)
