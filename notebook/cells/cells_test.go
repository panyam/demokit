package cells

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/panyam/demokit/notebook"
)

// allRows asks a cell for its full row list at the given width.
func allRows(c notebook.Cell, width int, focused bool, mode notebook.Mode) []string {
	return c.RenderRows(width, 0, c.HeightHint(width), focused, mode)
}

// captureClipboard returns a Clipboard that records the last
// payload into *got. strategy is fixed to "test".
func captureClipboard(got *string) notebook.Clipboard {
	return func(s string) (string, bool) {
		*got = s
		return "test", true
	}
}

// --- HeaderCell ---

func TestHeaderCellRendersTitleAndBody(t *testing.T) {
	c := NewHeader("h", "Authentication starts", "Set up the client.\nVerify creds load.")
	joined := strings.Join(allRows(c, 60, false, notebook.NavigationMode), "\n")
	for _, want := range []string{"Authentication starts", "Set up the client."} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected %q in render, got:\n%s", want, joined)
		}
	}
}

func TestHeaderCellFocusedChangesBorder(t *testing.T) {
	a := strings.Join(allRows(NewHeader("h", "Hello", ""), 60, false, notebook.NavigationMode), "\n")
	b := strings.Join(allRows(NewHeader("h", "Hello", ""), 60, true, notebook.NavigationMode), "\n")
	if a == b {
		t.Errorf("focused / unfocused renders identical:\n%s", a)
	}
}

func TestHeaderCellRenderRowsClampsRange(t *testing.T) {
	c := NewHeader("h", "T", "Body line one")
	full := c.HeightHint(60)
	if out := c.RenderRows(60, full-1, full+5, false, notebook.NavigationMode); len(out) != 1 {
		t.Errorf("over-end clamp: got %d rows, want 1", len(out))
	}
	if out := c.RenderRows(60, full+1, full+5, false, notebook.NavigationMode); out != nil {
		t.Errorf("past-end render should return nil, got %v", out)
	}
}

func TestHeaderCellWidthInvalidatesCache(t *testing.T) {
	c := NewHeader("h", "T", "this is a longer body that should wrap differently at different widths")
	h1 := c.HeightHint(40)
	h2 := c.HeightHint(120)
	if h1 == h2 {
		t.Errorf("HeightHint did not change with width: 40→%d, 120→%d", h1, h2)
	}
	if got := c.HeightHint(40); got != h1 {
		t.Errorf("cache invalidation on width re-shrink: got %d, want %d", got, h1)
	}
}

func TestHeaderCellStyleChangeInvalidatesCache(t *testing.T) {
	c := NewHeader("h", "Hello", "")
	a := strings.Join(allRows(c, 60, false, notebook.NavigationMode), "\n")
	c.Style = LightHeaderStyle()
	b := strings.Join(allRows(c, 60, false, notebook.NavigationMode), "\n")
	if a == b {
		t.Errorf("style change did not invalidate cache; render unchanged:\n%s", a)
	}
}

func TestHeaderCellTitleMutationReflects(t *testing.T) {
	c := NewHeader("h", "Original", "")
	_ = strings.Join(allRows(c, 60, false, notebook.NavigationMode), "\n")
	c.Title = "Replaced"
	joined := strings.Join(allRows(c, 60, false, notebook.NavigationMode), "\n")
	if !strings.Contains(joined, "Replaced") {
		t.Errorf("title mutation not reflected; got:\n%s", joined)
	}
}

// --- NoteCell ---

func TestNoteCellRendersTitleAndBody(t *testing.T) {
	c := NewNote("n", "Heads up", "Important context here.")
	joined := strings.Join(allRows(c, 60, false, notebook.NavigationMode), "\n")
	for _, want := range []string{"Heads up", "Important context here."} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected %q in render, got:\n%s", want, joined)
		}
	}
}

func TestNoteCellCopyInCellActiveModeUsesClipboard(t *testing.T) {
	var got string
	c := NewNote("n", "T", "the-payload")
	c.SetClipboard(captureClipboard(&got))
	c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")}, notebook.CellActiveMode)
	if got != "the-payload" {
		t.Errorf("clipboard payload = %q, want %q", got, "the-payload")
	}
	if c.copyMsg == "" {
		t.Error("expected copyMsg to be set after copy")
	}
}

func TestNoteCellCopyInNavigationModeAlsoWorks(t *testing.T) {
	var got string
	c := NewNote("n", "T", "another-payload")
	c.SetClipboard(captureClipboard(&got))
	c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")}, notebook.NavigationMode)
	if got != "another-payload" {
		t.Errorf("clipboard in NavigationMode = %q, want %q", got, "another-payload")
	}
}

func TestNoteCellNoClipboardFallback(t *testing.T) {
	c := NewNote("n", "T", "x")
	c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")}, notebook.CellActiveMode)
	if !strings.Contains(c.copyMsg, "copy failed") {
		t.Errorf("expected fallback copyMsg, got %q", c.copyMsg)
	}
}

func TestNoteCellSetClipboardNilUsesNoClipboard(t *testing.T) {
	c := NewNote("n", "T", "x")
	c.SetClipboard(nil)
	c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")}, notebook.CellActiveMode)
	if !strings.Contains(c.copyMsg, "copy failed") {
		t.Errorf("nil clipboard should fall back to NoClipboard; got %q", c.copyMsg)
	}
}

func TestNoteCellFallbackClipboardReplaysOnT(t *testing.T) {
	var primary, fallback string
	c := NewNote("n", "T", "the body")
	c.SetClipboard(captureClipboard(&primary))
	c.SetFallbackClipboard(func(s string) (string, bool) { fallback = s; return "/tmp/x", true })
	c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")}, notebook.CellActiveMode)
	if primary != "the body" {
		t.Errorf("primary captured %q, want %q", primary, "the body")
	}
	if !strings.Contains(c.copyMsg, "press t") {
		t.Errorf("banner should hint at 't' when fallback configured: %q", c.copyMsg)
	}
	c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")}, notebook.CellActiveMode)
	if fallback != "the body" {
		t.Errorf("fallback captured %q, want %q", fallback, "the body")
	}
}

// --- VerbatimCell ---

func TestVerbatimCellDefaultActiveHonorsIsDefault(t *testing.T) {
	c := NewVerbatim("v", "L", []Variant{
		{Label: "a", Content: "A"},
		{Label: "b", Content: "B", IsDefault: true},
		{Label: "c", Content: "C"},
	})
	if c.Active != 1 {
		t.Errorf("Active = %d, want 1 (the IsDefault variant)", c.Active)
	}
}

func TestVerbatimCellTabCycles(t *testing.T) {
	c := NewVerbatim("v", "L", []Variant{
		{Label: "a", Content: "A"},
		{Label: "b", Content: "B"},
		{Label: "c", Content: "C"},
	})
	c.Update(tea.KeyMsg{Type: tea.KeyTab}, notebook.CellActiveMode)
	if c.Active != 1 {
		t.Errorf("after Tab: Active = %d, want 1", c.Active)
	}
	c.Update(tea.KeyMsg{Type: tea.KeyTab}, notebook.CellActiveMode)
	c.Update(tea.KeyMsg{Type: tea.KeyTab}, notebook.CellActiveMode) // wrap
	if c.Active != 0 {
		t.Errorf("Tab wrap: Active = %d, want 0", c.Active)
	}
	c.Update(tea.KeyMsg{Type: tea.KeyShiftTab}, notebook.CellActiveMode)
	if c.Active != 2 {
		t.Errorf("Shift+Tab from 0: Active = %d, want 2", c.Active)
	}
}

func TestVerbatimCellNumericJump(t *testing.T) {
	c := NewVerbatim("v", "L", []Variant{
		{Content: "A"}, {Content: "B"}, {Content: "C"},
	})
	c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")}, notebook.CellActiveMode)
	if c.Active != 1 {
		t.Errorf("'2' → Active = %d, want 1", c.Active)
	}
	c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("9")}, notebook.CellActiveMode)
	if c.Active != 1 {
		t.Errorf("out-of-range '9' should be no-op; Active = %d", c.Active)
	}
}

func TestVerbatimCellCopyInNavigationModeUsesActiveVariant(t *testing.T) {
	var got string
	c := NewVerbatim("v", "L", []Variant{
		{Label: "curl", Content: "curl-payload"},
		{Label: "python", Content: "python-payload"},
	})
	c.SetClipboard(captureClipboard(&got))
	c.Update(tea.KeyMsg{Type: tea.KeyTab}, notebook.CellActiveMode) // switch to python
	c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")}, notebook.NavigationMode)
	if got != "python-payload" {
		t.Errorf("clipboard = %q, want python-payload", got)
	}
}

func TestVerbatimCellTabIgnoredInNavigationMode(t *testing.T) {
	c := NewVerbatim("v", "L", []Variant{{Content: "A"}, {Content: "B"}})
	c.Update(tea.KeyMsg{Type: tea.KeyTab}, notebook.NavigationMode)
	if c.Active != 0 {
		t.Errorf("Tab in NavigationMode should be ignored; Active = %d", c.Active)
	}
}

func TestVerbatimCellFallbackClipboardReplaysOnT(t *testing.T) {
	var primary, fallback string
	c := NewVerbatim("v", "L", []Variant{{Label: "curl", Content: "curl-payload"}})
	c.SetClipboard(captureClipboard(&primary))
	c.SetFallbackClipboard(func(s string) (string, bool) { fallback = s; return "/tmp/x", true })
	c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")}, notebook.CellActiveMode)
	if primary != "curl-payload" {
		t.Errorf("primary captured %q, want %q", primary, "curl-payload")
	}
	if !strings.Contains(c.copyMsg, "press t") {
		t.Errorf("banner should hint at 't' when fallback configured: %q", c.copyMsg)
	}
	c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")}, notebook.CellActiveMode)
	if fallback != "curl-payload" {
		t.Errorf("fallback captured %q, want %q", fallback, "curl-payload")
	}
}

func TestVerbatimCellStatusHintAdaptsToCount(t *testing.T) {
	single := NewVerbatim("v", "L", []Variant{{Content: "x"}})
	if got := single.StatusHint(notebook.CellActiveMode); got != "c copy" {
		t.Errorf("single-variant hint = %q, want %q", got, "c copy")
	}
	multi := NewVerbatim("v", "L", []Variant{{Content: "a"}, {Content: "b"}, {Content: "c"}})
	if got := multi.StatusHint(notebook.CellActiveMode); !strings.Contains(got, "1-3 jump") {
		t.Errorf("multi-variant hint missing 1-3 jump, got %q", got)
	}
}

// --- Theme ---

func TestDarkAndLightThemesDifferAtTheStyleLevel(t *testing.T) {
	dark, light := DarkTheme(), LightTheme()
	if dark.Header == light.Header {
		t.Error("DarkTheme.Header == LightTheme.Header — theme is not mode-aware")
	}
	if dark.Note == light.Note {
		t.Error("DarkTheme.Note == LightTheme.Note")
	}
	if dark.Verbatim == light.Verbatim {
		t.Error("DarkTheme.Verbatim == LightTheme.Verbatim")
	}
}

func TestApplyingThemeToCellChangesRender(t *testing.T) {
	c := NewHeader("h", "Hello", "")
	a := strings.Join(allRows(c, 60, false, notebook.NavigationMode), "\n")
	c.Style = LightTheme().Header
	b := strings.Join(allRows(c, 60, false, notebook.NavigationMode), "\n")
	if a == b {
		t.Error("applying LightTheme produced identical render")
	}
}
