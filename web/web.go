// Package web is the embed surface for demokit's <demokit-demo>
// player. It provides Go entry points hosts use to ship traces into
// browsers — TraceFragment for inline embeds, WriteBundle for
// self-contained HTML, PlayerJS/PlayerCSS for hosts that prefer to
// serve the assets from their own origin.
//
// Importing this package (even via blank import) registers the
// "bundle" doc format with demokit core, so `--doc bundle` becomes
// available on demos that opt in:
//
//	import _ "github.com/panyam/demokit/web"
//
// The player is committed under web/player/ as hand-written vanilla
// JS + CSS, embedded into the binary via go:embed.
package web

import (
	"bytes"
	_ "embed"
	"fmt"
	"html"
	"os"

	"github.com/panyam/demokit"
)

//go:embed player/demokit-player.js
var playerJS string

//go:embed player/demokit-player.css
var playerCSS string

func init() {
	demokit.RegisterDocFormat("bundle", func(d *demokit.Demo, entries []demokit.TraceEntry, out string) error {
		return WriteBundle(d, entries, out)
	})
}

// PlayerJS returns the source of the bundled <demokit-demo> Custom
// Element. Hosts that want to serve the player from their own
// origin can write this to a file or stream it from an HTTP handler.
func PlayerJS() string { return playerJS }

// PlayerCSS returns the player's stylesheet.
func PlayerCSS() string { return playerCSS }

// TraceFragment returns an HTML element string with the trace JSON
// inlined inside a <demokit-demo> tag. Hosts include the player
// script separately (e.g. via <script src=".../demokit-player.js">)
// or through WriteBundle for a self-contained page.
//
// The fragment is safe to inline in markdown/blog posts as long as
// the host page also includes the player JS. Without the player,
// the element renders as inert text — graceful degradation.
func TraceFragment(d *demokit.Demo, entries []demokit.TraceEntry) string {
	jsonBody := demokit.RenderDocumentJSON(demokit.RenderContext{Demo: d, Trace: entries})
	var b bytes.Buffer
	b.WriteString("<demokit-demo>")
	b.WriteString(jsonBody)
	b.WriteString("</demokit-demo>")
	return b.String()
}

// WriteBundle writes a self-contained HTML document to outPath:
// player JS + CSS embedded inline, trace JSON inline, ready to open
// from file:// without a server. If outPath is empty, the bundle
// is written to stdout.
//
// The bundle has no external <script src> or <link href> references
// and works from file:// — useful for shipping single-file demo
// archives, attaching to bug reports, or copying into slide decks.
func WriteBundle(d *demokit.Demo, entries []demokit.TraceEntry, outPath string) error {
	bundle := bundleHTML(d, entries)
	if outPath == "" {
		_, err := os.Stdout.WriteString(bundle)
		return err
	}
	return os.WriteFile(outPath, []byte(bundle), 0o644)
}

// bundleHTML assembles the self-contained page.
func bundleHTML(d *demokit.Demo, entries []demokit.TraceEntry) string {
	title := "Demo"
	if d != nil && d.Title() != "" {
		title = d.Title()
	}
	traceJSON := demokit.RenderDocumentJSON(demokit.RenderContext{Demo: d, Trace: entries})

	var b bytes.Buffer
	b.WriteString("<!doctype html>\n<html lang=\"en\">\n<head>\n")
	b.WriteString("<meta charset=\"utf-8\">\n")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	fmt.Fprintf(&b, "<title>%s</title>\n", html.EscapeString(title))
	b.WriteString("<style>\n")
	b.WriteString("body { margin: 2em auto; max-width: 900px; padding: 0 1em; font-family: -apple-system, BlinkMacSystemFont, sans-serif; }\n")
	b.WriteString(playerCSS)
	b.WriteString("\n</style>\n")
	b.WriteString("</head>\n<body>\n")
	b.WriteString("<demokit-demo>")
	b.WriteString(traceJSON)
	b.WriteString("</demokit-demo>\n")
	b.WriteString("<script>\n")
	b.WriteString(playerJS)
	b.WriteString("\n</script>\n")
	b.WriteString("</body>\n</html>\n")
	return b.String()
}
