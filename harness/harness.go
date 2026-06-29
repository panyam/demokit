// Package harness wires the renderer, mode, and doc/serve plumbing that
// every demokit walkthrough otherwise rediscovers, so a demo's entry
// point is a single call instead of a copy-pasted renderer switch.
//
// It is batteries-included on purpose: importing harness pulls the tui,
// notebookbridge, and web subpackages (and their charm / websocket
// dependencies), because honoring --mode=tui / --mode=notebook / --serve
// requires that code to be linked. A consumer who wants a leaner binary,
// or a custom renderer set, should skip harness and wire renderers
// directly against demokit — the lean path stays available, the dependency
// cost lives at the "do I import harness?" boundary.
package harness

import (
	"github.com/panyam/demokit"
	"github.com/panyam/demokit/notebookbridge"
	"github.com/panyam/demokit/tui"
	"github.com/panyam/demokit/web"
)

// SetupRenderer selects the renderer matching demokit.Mode() and enables
// the web-backed doc/serve handlers, without running the demo. Use it
// when the caller wants to do its own work before Execute (extra flags,
// a custom plain renderer); otherwise prefer Run.
//
//   - --mode=tui      (or --tui)  → tui.Renderer
//   - --mode=notebook (or --note) → notebookbridge.Bridge
//   - --mode=plain / absent       → left unset; Execute defaults to PlainRenderer
//
// Both styled renderers are configured with BorderHorizontalOnly so a
// triple-click / drag-select over a verbatim block grabs only the
// content, no side box characters — the copy-friendly convention shared
// across demokit consumers. SetupRenderer also calls web.RegisterWith,
// so --doc bundle and --serve work without extra wiring.
//
// Do not also call web.RegisterWith yourself when using harness: the
// "bundle" doc format is registered here, and registering it twice
// panics.
func SetupRenderer(demo *demokit.Demo) {
	switch demokit.Mode() {
	case "tui":
		demo.WithRenderer(tui.New().WithBorderStyle(demokit.BorderHorizontalOnly))
	case "notebook":
		demo.WithRenderer(notebookbridge.New().WithBorderStyle(demokit.BorderHorizontalOnly))
	}
	web.RegisterWith(demo)
}

// Run is SetupRenderer followed by demo.Execute — the one-call entry
// point for a walkthrough's main(). Call it after the demo's steps are
// defined, in place of a hand-written renderer switch plus Execute.
func Run(demo *demokit.Demo) {
	SetupRenderer(demo)
	demo.Execute()
}
