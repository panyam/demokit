package notebook

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
