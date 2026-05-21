// Package web is the embed surface for demokit's <demokit-demo>
// player. It provides Go entry points hosts use to ship traces into
// browsers — TraceFragment for inline embeds, WriteBundle for an
// HTML shell that links the player from sibling files (when --out
// is set) or from a CDN (when written to stdout).
//
// Call web.RegisterWith(demo) before Execute() to enable the
// "bundle" --doc format and the --serve live mode on a Demo:
//
//	demo := demokit.New("...")
//	web.RegisterWith(demo)
//
// The player files are committed under web/player/ and the HTML
// shell is rendered from a templar template under web/templates/.
// We never inline the player bytes into the bundle — local --out
// writes them as siblings; stdout-mode references the CDN copy
// pinned to a release tag.
package web

import (
	"bytes"
	"embed"
	_ "embed"
	"fmt"
	"html/template"
	"os"
	"path/filepath"

	"github.com/panyam/demokit"
	"github.com/panyam/templar"
)

//go:embed player/demokit-player.js
var playerJS string

//go:embed player/demokit-player.css
var playerCSS string

// ansi_up is vendored under web/player/ at a pinned upstream commit
// (see ansi_up.VENDOR.md). Embedding it into the binary means
// installed `go install` binaries — which have no access to our
// source tree — can still write the file alongside bundles and
// serve it from `--serve` without a runtime CDN fetch.
//
//go:embed player/ansi_up.js
var ansiUpJS string

//go:embed templates/*.html
var tmplFS embed.FS

// CDN URLs that the stdout-mode bundle references when the user
// pipes --doc bundle without --out. We pin by version (not by
// branch or `latest`) for reproducibility — bundles built with an
// older demokit binary keep pointing at their pinned tag's player.
//
//   - PlayerCDNVersion is the demokit release tag this code targets.
//     Bump on release; `replace github.com/panyam/demokit => ...` in
//     downstream go.mod files keeps in step.
//   - The ansi_up library handles the SGR → HTML conversion. Pinned
//     to its v6.0.6 release commit. Imported by the player module.
const (
	PlayerCDNVersion = "v0.0.11"
	playerCDNBase    = "https://cdn.jsdelivr.net/gh/panyam/demokit@" + PlayerCDNVersion + "/web/player"
)

// RegisterWith wires this package's capabilities into a demo:
//
//   - --doc bundle (writes a self-contained HTML + sibling assets)
//   - --serve <addr> (live HTTP+WS server; from serve.go)
//
// Call after constructing your demo and before Execute:
//
//	demo := demokit.New("...")
//	web.RegisterWith(demo)
//	demo.Execute()
//
// Per-instance registration means multiple demos in one process
// don't share state and tests are fully isolated.
func RegisterWith(d *demokit.Demo) {
	d.RegisterDocFormat("bundle", func(d *demokit.Demo, entries []demokit.TraceEntry, out string) error {
		return WriteBundle(d, entries, out)
	})
	d.RegisterServeHandler(func(d *demokit.Demo, addr string) error {
		return ServeHTTP(d, addr)
	})
}

// PlayerJS returns the source of the bundled <demokit-demo> Custom
// Element. Hosts that want to serve the player from their own
// origin can write this to a file or stream it from an HTTP handler.
func PlayerJS() string { return playerJS }

// PlayerCSS returns the player's stylesheet.
func PlayerCSS() string { return playerCSS }

// AnsiUpJS returns the vendored ansi_up.js source. Hosts that
// self-serve the player should drop this file alongside
// demokit-player.js so the player's `import './ansi_up.js'` resolves
// at runtime.
func AnsiUpJS() string { return ansiUpJS }

// TraceFragment returns an HTML element string with the trace JSON
// inlined inside a <demokit-demo> tag. Hosts include the player
// script separately (typically via the CDN URL or by self-hosting
// the file PlayerJS() returns).
//
// Without the player loaded, the element renders as inert text —
// graceful degradation.
func TraceFragment(d *demokit.Demo, entries []demokit.TraceEntry) string {
	jsonBody := demokit.RenderDocumentJSON(demokit.RenderContext{Demo: d, Trace: entries})
	var b bytes.Buffer
	b.WriteString("<demokit-demo>")
	b.WriteString(jsonBody)
	b.WriteString("</demokit-demo>")
	return b.String()
}

// WriteBundle writes the bundle to outPath as an HTML shell and
// drops the player JS + CSS as siblings in the same directory:
//
//	<outPath>
//	<dir>/demokit-player.js
//	<dir>/demokit-player.css
//
// The HTML's <link> and <script> tags reference the siblings via
// relative paths so the bundle works from file:// without network.
//
// If outPath is empty, the bundle is written to stdout with CDN URLs
// for the player files (single-file form, requires network on first
// open). For air-gapped distribution, use the file form.
func WriteBundle(d *demokit.Demo, entries []demokit.TraceEntry, outPath string) error {
	if outPath == "" {
		return writeBundleStdout(d, entries)
	}
	return writeBundleLocal(d, entries, outPath)
}

func writeBundleStdout(d *demokit.Demo, entries []demokit.TraceEntry) error {
	html, err := renderBundleHTML(d, entries,
		playerCDNBase+"/demokit-player.css",
		playerCDNBase+"/demokit-player.js",
	)
	if err != nil {
		return err
	}
	_, err = os.Stdout.WriteString(html)
	return err
}

func writeBundleLocal(d *demokit.Demo, entries []demokit.TraceEntry, outPath string) error {
	dir := filepath.Dir(outPath)
	cssPath := filepath.Join(dir, "demokit-player.css")
	jsPath := filepath.Join(dir, "demokit-player.js")
	ansiPath := filepath.Join(dir, "ansi_up.js")

	html, err := renderBundleHTML(d, entries,
		"./demokit-player.css",
		"./demokit-player.js",
	)
	if err != nil {
		return fmt.Errorf("render bundle template: %w", err)
	}
	if err := os.WriteFile(outPath, []byte(html), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", outPath, err)
	}
	if err := os.WriteFile(cssPath, []byte(playerCSS), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", cssPath, err)
	}
	if err := os.WriteFile(jsPath, []byte(playerJS), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", jsPath, err)
	}
	if err := os.WriteFile(ansiPath, []byte(ansiUpJS), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", ansiPath, err)
	}
	return nil
}

// renderBundleHTML loads the bundle template via templar and
// renders it with the given asset hrefs. Templar lets us compose
// includes/inheritance later if the templates grow; for now it's
// just a thin wrapper around html/template that loads from our
// embedded FS.
func renderBundleHTML(d *demokit.Demo, entries []demokit.TraceEntry, cssHref, jsSrc string) (string, error) {
	title := "Demo"
	if d != nil && d.Title() != "" {
		title = d.Title()
	}
	traceJSON := demokit.RenderDocumentJSON(demokit.RenderContext{Demo: d, Trace: entries})

	group := templar.NewTemplateGroup()
	group.Loader = templar.NewFileSystemLoader(templar.FSFolder{FS: tmplFS, Path: "templates"})
	tmpls, err := group.Loader.Load("bundle.html", "")
	if err != nil {
		return "", fmt.Errorf("load bundle.html: %w", err)
	}
	if len(tmpls) == 0 {
		return "", fmt.Errorf("bundle.html template not found")
	}

	var buf bytes.Buffer
	data := bundleData{
		Title:         title,
		PlayerCSSHref: cssHref,
		PlayerJSSrc:   jsSrc,
		// template.HTML opts the JSON out of html/template's auto
		// escaping. JSON itself is already HTML-injection-safe —
		// encoding/json's default SetEscapeHTML(true) turns "<", ">",
		// "&" into "<" / ">" / "&" before they reach
		// us. Letting html/template escape AGAIN would just bloat
		// the bundle (every `"` becomes `&#34;`).
		TraceJSON: template.HTML(traceJSON),
	}
	if err := group.RenderHtmlTemplate(&buf, tmpls[0], "", data, nil); err != nil {
		return "", fmt.Errorf("render bundle.html: %w", err)
	}
	return buf.String(), nil
}

// bundleData is the variable namespace exposed to bundle.html.
// Field names mirror what the template expects.
type bundleData struct {
	Title         string
	PlayerCSSHref string
	PlayerJSSrc   string
	TraceJSON     template.HTML
}
