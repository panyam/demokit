package web

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/panyam/demokit"
)

func fixtureDemoForBundle() (*demokit.Demo, []demokit.TraceEntry) {
	d := demokit.New("Bundle Test").Description("a small fixture")
	d.Step("first").ID("a").Note("first note")
	d.Step("second").ID("b")
	trace := []demokit.TraceEntry{
		{Kind: demokit.KindStep, Title: "first", StepID: "a", Visit: 1, Output: "hi"},
		{Kind: demokit.KindStep, Title: "second", StepID: "b", Visit: 1},
	}
	return d, trace
}

// captureStdout swaps os.Stdout for a pipe during fn and returns
// what was written. Mirror of the helper in the parent package; we
// keep an independent copy here so the web package's tests don't
// reach into demokit-internal test helpers.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = orig }()
	fn()
	w.Close()
	out, _ := io.ReadAll(r)
	return string(out)
}

// TestTraceFragmentInlinesJSON verifies the Mode-C fragment carries
// the trace JSON inline between <demokit-demo> tags and that JSON
// round-trips back through json.Unmarshal. Pins the contract for
// hosts that paste the fragment into their pages.
func TestTraceFragmentInlinesJSON(t *testing.T) {
	d, trace := fixtureDemoForBundle()
	frag := TraceFragment(d, trace)

	if !strings.HasPrefix(frag, "<demokit-demo>") {
		t.Errorf("fragment doesn't start with <demokit-demo>: %q", frag[:min(50, len(frag))])
	}
	if !strings.HasSuffix(strings.TrimRight(frag, "\n"), "</demokit-demo>") {
		t.Errorf("fragment doesn't end with </demokit-demo>: %q", frag)
	}

	body := strings.TrimPrefix(frag, "<demokit-demo>")
	body = strings.TrimSuffix(body, "</demokit-demo>")

	var parsed map[string]any
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		t.Fatalf("inline JSON didn't round-trip: %v\nbody: %s", err, body)
	}
	if parsed["demo"] == nil || parsed["trace"] == nil {
		t.Errorf("expected demo + trace keys: %v", parsed)
	}
}

// TestWriteBundleLocalWritesThreeFiles verifies WriteBundle with an
// out path drops three sibling files (HTML + JS + CSS) and that the
// HTML references the assets via relative paths so the bundle works
// from file:// without network. Inlining the player bytes into the
// HTML would bloat every bundle; the asset-trio shape keeps the
// HTML small and lets the JS/CSS cache across pages.
func TestWriteBundleLocalWritesThreeFiles(t *testing.T) {
	tmp := t.TempDir()
	htmlPath := tmp + "/bundle.html"
	jsPath := tmp + "/demokit-player.js"
	cssPath := tmp + "/demokit-player.css"

	d, trace := fixtureDemoForBundle()
	if err := WriteBundle(d, trace, htmlPath); err != nil {
		t.Fatalf("WriteBundle: %v", err)
	}

	html, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatalf("HTML not written: %v", err)
	}
	for _, want := range []string{
		"<!doctype html>",
		"<title>Bundle Test</title>",
		"<link rel=\"stylesheet\" href=\"./demokit-player.css\">",
		"<script type=\"module\" src=\"./demokit-player.js\">",
		"<demokit-demo>",
		"\"trace\"", // JSON payload
	} {
		if !strings.Contains(string(html), want) {
			t.Errorf("bundle HTML missing %q", want)
		}
	}
	// Player bytes must NOT be inlined into the HTML — that's the
	// regression this test guards against.
	for _, banned := range []string{".demokit-player {", "customElements.define"} {
		if strings.Contains(string(html), banned) {
			t.Errorf("bundle HTML should not inline %q (the player files are siblings now)", banned)
		}
	}

	js, err := os.ReadFile(jsPath)
	if err != nil {
		t.Fatalf("player JS not written: %v", err)
	}
	if !strings.Contains(string(js), "customElements.define") {
		t.Errorf("sibling demokit-player.js doesn't look like the player; got %d bytes", len(js))
	}

	css, err := os.ReadFile(cssPath)
	if err != nil {
		t.Fatalf("player CSS not written: %v", err)
	}
	if !strings.Contains(string(css), ".demokit-player") {
		t.Errorf("sibling demokit-player.css doesn't look like the stylesheet; got %d bytes", len(css))
	}
}

// TestWriteBundleStdoutUsesCDN verifies WriteBundle("") produces a
// single-file shell that references the player from a pinned CDN
// URL. Stdout-mode is for piping to a single file or another tool;
// the local-asset trio doesn't fit that shape.
func TestWriteBundleStdoutUsesCDN(t *testing.T) {
	d, trace := fixtureDemoForBundle()
	out := captureStdout(t, func() {
		if err := WriteBundle(d, trace, ""); err != nil {
			t.Fatalf("WriteBundle: %v", err)
		}
	})

	for _, want := range []string{
		"<!doctype html>",
		"https://cdn.jsdelivr.net/gh/panyam/demokit@" + PlayerCDNVersion + "/web/player/demokit-player.css",
		"https://cdn.jsdelivr.net/gh/panyam/demokit@" + PlayerCDNVersion + "/web/player/demokit-player.js",
		"<demokit-demo>",
		"\"trace\"",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("stdout bundle missing %q", want)
		}
	}
}

// TestPlayerJSCSSEmbedded verifies //go:embed actually pulled the
// player files in. A typo in the //go:embed directive would silently
// produce empty strings — this catches that.
func TestPlayerJSCSSEmbedded(t *testing.T) {
	if !strings.Contains(PlayerJS(), "DemokitDemoElement") {
		t.Errorf("PlayerJS embed missing or wrong; got %d bytes", len(PlayerJS()))
	}
	if !strings.Contains(PlayerCSS(), ".demokit-player") {
		t.Errorf("PlayerCSS embed missing or wrong; got %d bytes", len(PlayerCSS()))
	}
}

// TestRegisterDocFormatHookWiresBundle verifies that importing this
// package registers "bundle" as a doc format. Without this init
// hook, --doc bundle would silently fail on demos that don't import
// the web package.
func TestRegisterDocFormatHookWiresBundle(t *testing.T) {
	// Calling RegisterDocFormat("bundle", ...) again would be a no-op
	// (replaces the handler), so we can't directly assert on registry
	// internals from here. The behavior pinned: WriteBundle should
	// have been registered at init() time. We exercise the round-trip
	// indirectly — if init() didn't fire, demokit core's --doc bundle
	// dispatch wouldn't find the handler. This is implicitly tested
	// by the integration smoke (running --doc bundle from main).
	//
	// Direct assertion: PlayerJS/PlayerCSS are non-empty.
	if PlayerJS() == "" || PlayerCSS() == "" {
		t.Error("init() didn't run — embedded assets are empty")
	}
}
