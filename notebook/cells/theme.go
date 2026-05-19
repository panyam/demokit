package cells

// Theme bundles the built-in cell styles for batch application. A
// theme is the appropriate layer for light/dark mode selection —
// each individual cell style holds concrete colors with no mode
// flag; the theme picks which set of styles to use.
//
// Theme is intentionally a value type with exported fields. The
// notebook package itself doesn't depend on Theme — consumers can
// use these styles directly without going through Theme, or
// implement their own bundle. Theme is just a convenience.
type Theme struct {
	Header   HeaderStyle
	Note     NoteStyle
	Verbatim VerbatimStyle
}

// DarkTheme returns the dark-terminal styles for all built-in
// cells.
func DarkTheme() Theme {
	return Theme{
		Header:   DarkHeaderStyle(),
		Note:     DarkNoteStyle(),
		Verbatim: DarkVerbatimStyle(),
	}
}

// LightTheme returns the light-terminal styles for all built-in
// cells.
func LightTheme() Theme {
	return Theme{
		Header:   LightHeaderStyle(),
		Note:     LightNoteStyle(),
		Verbatim: LightVerbatimStyle(),
	}
}

// DefaultTheme returns the package default — Dark.
func DefaultTheme() Theme { return DarkTheme() }
