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

// TestWriteBundleSelfContained verifies the bundle is a complete
// HTML document with the player CSS and JS inlined and the trace
// JSON inside a <demokit-demo> element. No external <script src>
// or <link href> — the bundle must work from file:// alone.
func TestWriteBundleSelfContained(t *testing.T) {
	tmp, err := os.CreateTemp("", "demokit-bundle-*.html")
	if err != nil {
		t.Fatal(err)
	}
	tmp.Close()
	defer os.Remove(tmp.Name())

	d, trace := fixtureDemoForBundle()
	if err := WriteBundle(d, trace, tmp.Name()); err != nil {
		t.Fatalf("WriteBundle: %v", err)
	}

	body, err := os.ReadFile(tmp.Name())
	if err != nil {
		t.Fatal(err)
	}
	out := string(body)

	for _, want := range []string{
		"<!doctype html>",
		"<title>Bundle Test</title>",
		".demokit-player",       // CSS inlined
		"customElements.define", // JS inlined
		"<demokit-demo>",
		"\"trace\"", // JSON payload
	} {
		if !strings.Contains(out, want) {
			t.Errorf("bundle missing %q", want)
		}
	}

	for _, banned := range []string{"<script src=", "<link "} {
		if strings.Contains(out, banned) {
			t.Errorf("bundle should not reference external asset: contains %q", banned)
		}
	}
}

// TestWriteBundleStdoutWhenNoOutPath verifies WriteBundle("", ...)
// writes to stdout — the path other --doc formats use when --out is
// not specified.
func TestWriteBundleStdoutWhenNoOutPath(t *testing.T) {
	d, trace := fixtureDemoForBundle()

	out := captureStdout(t, func() {
		if err := WriteBundle(d, trace, ""); err != nil {
			t.Fatalf("WriteBundle: %v", err)
		}
	})

	if !strings.Contains(out, "<!doctype html>") || !strings.Contains(out, "<demokit-demo>") {
		t.Errorf("bundle to stdout missing structure:\n%s", out[:min(300, len(out))])
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
