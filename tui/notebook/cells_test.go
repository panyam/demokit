package notebook

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/panyam/demokit"
)

// clipboardPayload extracts the plaintext payload from an OSC 52
// escape sequence `\x1b]52;c;<base64>\a` for assertion convenience.
// Returns "" if the buffer doesn't contain a single well-formed OSC 52.
func clipboardPayload(t *testing.T, buf *bytes.Buffer) string {
	t.Helper()
	raw := buf.String()
	const prefix = "\x1b]52;c;"
	const suffix = "\a"
	i := strings.Index(raw, prefix)
	if i < 0 {
		return ""
	}
	j := strings.Index(raw[i+len(prefix):], suffix)
	if j < 0 {
		return ""
	}
	enc := raw[i+len(prefix) : i+len(prefix)+j]
	dec, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		t.Fatalf("OSC52 payload was not base64: %q", enc)
	}
	return string(dec)
}

// allRows is a small helper: ask the cell for its full row list at
// the given width so tests can assert on the whole render at once.
func allRows(c Cell, width int, focused bool, mode Mode) []string {
	return c.RenderRows(width, 0, c.HeightHint(width), focused, mode)
}

func TestMetaCellRendersTitleAndBody(t *testing.T) {
	c := NewMetaCell("step.intro#0.meta", "Authentication starts", "Set up the client.\nVerify creds load.")
	rows := allRows(c, 60, false, SelectMode)
	if len(rows) == 0 {
		t.Fatal("no rows")
	}
	joined := strings.Join(rows, "\n")
	if !strings.Contains(joined, "Authentication starts") {
		t.Errorf("expected title text in render, got:\n%s", joined)
	}
	if !strings.Contains(joined, "Set up the client.") {
		t.Errorf("expected body in render, got:\n%s", joined)
	}
}

func TestMetaCellFocusedChangesBorderColor(t *testing.T) {
	// Focus is signaled by lipgloss border color now (FocusBorder vs
	// MetaBorder); the cell never emits the old glyph swap. We assert
	// the rendered ANSI changes when focused vs not — exact escape
	// sequences depend on the runtime palette so we just compare
	// inequality.
	c := NewMetaCell("m", "Hello", "")
	unfocused := strings.Join(allRows(c, 60, false, SelectMode), "\n")
	c2 := NewMetaCell("m", "Hello", "")
	focused := strings.Join(allRows(c2, 60, true, SelectMode), "\n")
	if unfocused == focused {
		t.Errorf("focused and unfocused renders are identical; expected border-color change\n%s", unfocused)
	}
}

func TestMetaCellRenderRowsClampsRange(t *testing.T) {
	c := NewMetaCell("m", "T", "Body line one")
	full := c.HeightHint(60)
	out := c.RenderRows(60, full-1, full+5, false, SelectMode)
	if len(out) != 1 {
		t.Errorf("over-end clamp: got %d rows, want 1", len(out))
	}
	none := c.RenderRows(60, full+1, full+5, false, SelectMode)
	if none != nil {
		t.Errorf("past-end render should return nil, got %v", none)
	}
}

func TestMetaCellWidthInvalidatesCache(t *testing.T) {
	c := NewMetaCell("m", "T", "this is a longer body that should wrap differently at different widths")
	h1 := c.HeightHint(40)
	h2 := c.HeightHint(120)
	if h1 == h2 {
		t.Errorf("HeightHint did not change with width: 40→%d, 120→%d", h1, h2)
	}
	// Going back to 40 should match h1 — cache must rebuild, not return h2.
	if got := c.HeightHint(40); got != h1 {
		t.Errorf("cache invalidation on width re-shrink: got %d, want %d", got, h1)
	}
}

func TestVerbatimCellTabCyclesActive(t *testing.T) {
	vs := []demokit.Variant{
		{Label: "curl", Content: "curl -s ...", IsDefault: true},
		{Label: "python", Content: "import requests"},
		{Label: "go", Content: "http.Get(...)"},
	}
	c := NewVerbatimCell("v", "Refresh token", vs)
	if c.active != 0 {
		t.Fatalf("initial active = %d, want 0 (the IsDefault variant)", c.active)
	}
	c.Update(tea.KeyMsg{Type: tea.KeyTab}, ViewMode)
	if c.active != 1 {
		t.Errorf("Tab → active = %d, want 1", c.active)
	}
	c.Update(tea.KeyMsg{Type: tea.KeyTab}, ViewMode)
	c.Update(tea.KeyMsg{Type: tea.KeyTab}, ViewMode) // wrap
	if c.active != 0 {
		t.Errorf("Tab wrap → active = %d, want 0", c.active)
	}
	c.Update(tea.KeyMsg{Type: tea.KeyShiftTab}, ViewMode)
	if c.active != 2 {
		t.Errorf("Shift+Tab wrap-back → active = %d, want 2", c.active)
	}
}

func TestVerbatimCellNumericJump(t *testing.T) {
	vs := []demokit.Variant{
		{Label: "a", Content: "A"},
		{Label: "b", Content: "B"},
		{Label: "c", Content: "C"},
	}
	c := NewVerbatimCell("v", "L", vs)
	c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")}, ViewMode)
	if c.active != 1 {
		t.Errorf("'2' → active = %d, want 1", c.active)
	}
	c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("9")}, ViewMode)
	if c.active != 1 {
		t.Errorf("out-of-range '9' should be no-op; got active = %d", c.active)
	}
}

func TestVerbatimCellCopiesActiveContent(t *testing.T) {
	var buf bytes.Buffer
	demokit.SetClipboardWriter(&buf)
	defer demokit.SetClipboardWriter(nil)
	demokit.EnableShellClipboardFallback(false)
	defer demokit.EnableShellClipboardFallback(true)

	vs := []demokit.Variant{
		{Label: "curl", Content: "curl-payload"},
		{Label: "python", Content: "python-payload"},
	}
	c := NewVerbatimCell("v", "L", vs)
	c.Update(tea.KeyMsg{Type: tea.KeyTab}, ViewMode) // switch to python
	c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")}, ViewMode)
	if got := clipboardPayload(t, &buf); got != "python-payload" {
		t.Errorf("clipboard payload = %q, want %q", got, "python-payload")
	}
	if c.copyMsg == "" {
		t.Errorf("expected copyMsg to be set after copy")
	}
}

func TestVerbatimCellStatusHintAdaptsToCount(t *testing.T) {
	single := NewVerbatimCell("v", "L", []demokit.Variant{{Content: "x"}})
	if got := single.StatusHint(ViewMode); got != "c copy" {
		t.Errorf("single-variant hint = %q, want %q", got, "c copy")
	}
	multi := NewVerbatimCell("v", "L", []demokit.Variant{{Content: "a"}, {Content: "b"}, {Content: "c"}})
	if got := multi.StatusHint(ViewMode); !strings.Contains(got, "1-3 jump") {
		t.Errorf("multi-variant hint missing 1-3 jump, got %q", got)
	}
}

func TestOutputCellScrollsWithJK(t *testing.T) {
	buf := NewOutputBuffer()
	for i := 0; i < 20; i++ {
		buf.Append([]byte("line\n"))
	}
	c := NewOutputCell("o", buf, 5)
	for i := 0; i < 3; i++ {
		c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}, ViewMode)
	}
	if c.scrollOffset != 3 {
		t.Errorf("after 3×j, scrollOffset = %d, want 3", c.scrollOffset)
	}
	for i := 0; i < 50; i++ {
		c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}, ViewMode)
	}
	if max := 20 - 5; c.scrollOffset != max {
		t.Errorf("scrollOffset = %d, want clamped to %d", c.scrollOffset, max)
	}
	// k pulls back; should stop at 0.
	for i := 0; i < 100; i++ {
		c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")}, ViewMode)
	}
	if c.scrollOffset != 0 {
		t.Errorf("scrollOffset after k-spam = %d, want 0", c.scrollOffset)
	}
}

func TestOutputCellGGJumps(t *testing.T) {
	buf := NewOutputBuffer()
	for i := 0; i < 30; i++ {
		buf.Append([]byte("x\n"))
	}
	c := NewOutputCell("o", buf, 4)
	c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")}, ViewMode)
	if c.scrollOffset != 30-4 {
		t.Errorf("G → scrollOffset = %d, want %d", c.scrollOffset, 30-4)
	}
	c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")}, ViewMode)
	if c.scrollOffset != 0 {
		t.Errorf("g → scrollOffset = %d, want 0", c.scrollOffset)
	}
}

func TestOutputCellHeightCappedByMaxBody(t *testing.T) {
	buf := NewOutputBuffer()
	for i := 0; i < 100; i++ {
		buf.Append([]byte("y\n"))
	}
	c := NewOutputCell("o", buf, 5)
	// 3 header + 5 body + 1 status = 9
	if got, want := c.HeightHint(80), 9; got != want {
		t.Errorf("HeightHint = %d, want %d (capped by maxBody=5)", got, want)
	}
}

func TestOutputCellAutoFollowsOnAppend(t *testing.T) {
	buf := NewOutputBuffer()
	c := NewOutputCell("o", buf, 3)
	// Fewer than maxBody lines: scrollOffset stays at 0 (whole buf fits).
	buf.Append([]byte("a\n"))
	c.OnAppend()
	if c.scrollOffset != 0 {
		t.Errorf("with 1 line < maxBody, scrollOffset = %d, want 0", c.scrollOffset)
	}
	// Grow past maxBody: follow advances scrollOffset so the last
	// maxBody lines remain visible.
	for i := 0; i < 10; i++ {
		buf.Append([]byte("x\n"))
	}
	c.OnAppend()
	if want := 11 - 3; c.scrollOffset != want {
		t.Errorf("auto-follow scrollOffset = %d, want %d", c.scrollOffset, want)
	}
}

func TestOutputCellManualScrollDisablesFollow(t *testing.T) {
	buf := NewOutputBuffer()
	for i := 0; i < 10; i++ {
		buf.Append([]byte("x\n"))
	}
	c := NewOutputCell("o", buf, 3)
	c.OnAppend()
	// User scrolls up; subsequent chunks should NOT yank them.
	c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")}, ViewMode)
	if c.follow {
		t.Error("follow should be disabled after k")
	}
	frozen := c.scrollOffset
	for i := 0; i < 5; i++ {
		buf.Append([]byte("y\n"))
	}
	c.OnAppend()
	if c.scrollOffset != frozen {
		t.Errorf("with follow off, OnAppend moved scrollOffset from %d to %d", frozen, c.scrollOffset)
	}
	// G re-engages follow and jumps to bottom.
	c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")}, ViewMode)
	if !c.follow {
		t.Error("follow should be re-enabled after G")
	}
	buf.Append([]byte("z\n"))
	c.OnAppend()
	if want := buf.LineCount() - 3; c.scrollOffset != want {
		t.Errorf("after G + chunk: scrollOffset = %d, want %d", c.scrollOffset, want)
	}
}

func TestOutputCellRenderShowsLatestLinesWhenFollowing(t *testing.T) {
	buf := NewOutputBuffer()
	c := NewOutputCell("o", buf, 3)
	for i := 1; i <= 5; i++ {
		buf.Append([]byte(fmt.Sprintf("line%d\n", i)))
		c.OnAppend()
	}
	rows := allRows(c, 80, false, ViewMode)
	joined := strings.Join(rows, "\n")
	for _, want := range []string{"line3", "line4", "line5"} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected latest line %q in render, got:\n%s", want, joined)
		}
	}
	for _, off := range []string{"line1", "line2"} {
		if strings.Contains(joined, off) {
			t.Errorf("expected earliest line %q to be scrolled off, got:\n%s", off, joined)
		}
	}
}

func TestOutputCellCopiesInSelectMode(t *testing.T) {
	var buf bytes.Buffer
	demokit.SetClipboardWriter(&buf)
	defer demokit.SetClipboardWriter(nil)
	demokit.EnableShellClipboardFallback(false)
	defer demokit.EnableShellClipboardFallback(true)

	ob := NewOutputBuffer()
	ob.Append([]byte("hello\nworld\n"))
	c := NewOutputCell("o", ob, 10)
	c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")}, SelectMode)
	if got := clipboardPayload(t, &buf); got != "hello\nworld" {
		t.Errorf("clipboard payload = %q, want %q", got, "hello\nworld")
	}
	if c.copyMsg == "" {
		t.Error("expected copyMsg to be set after copy in SelectMode")
	}
}

func TestVerbatimCellCopiesInSelectMode(t *testing.T) {
	var buf bytes.Buffer
	demokit.SetClipboardWriter(&buf)
	defer demokit.SetClipboardWriter(nil)
	demokit.EnableShellClipboardFallback(false)
	defer demokit.EnableShellClipboardFallback(true)

	c := NewVerbatimCell("v", "L", []demokit.Variant{{Label: "x", Content: "payload"}})
	c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")}, SelectMode)
	if got := clipboardPayload(t, &buf); got != "payload" {
		t.Errorf("clipboard payload = %q, want %q", got, "payload")
	}
}

func TestSectionCellCopies(t *testing.T) {
	var buf bytes.Buffer
	demokit.SetClipboardWriter(&buf)
	defer demokit.SetClipboardWriter(nil)
	demokit.EnableShellClipboardFallback(false)
	defer demokit.EnableShellClipboardFallback(true)

	c := NewSectionCell("s", "Heads up", "Important context here.")
	c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")}, ViewMode)
	if got := clipboardPayload(t, &buf); got != "Important context here." {
		t.Errorf("clipboard payload = %q, want %q", got, "Important context here.")
	}
}

func TestCellsIgnoreUpdatesInSelectMode(t *testing.T) {
	vs := []demokit.Variant{{Content: "a"}, {Content: "b"}}
	c := NewVerbatimCell("v", "L", vs)
	c.Update(tea.KeyMsg{Type: tea.KeyTab}, SelectMode)
	if c.active != 0 {
		t.Errorf("Tab in SelectMode should be ignored; active = %d, want 0", c.active)
	}
}
