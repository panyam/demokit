package demokit

// RenderContext is the input to every doc renderer. Renderers are pure
// functions of these three values and never invoke Run; computed values
// produced during execution live in Trace, the demo's static definition
// lives in Demo, and live-state snapshots (future) flow through State.
//
// Three render modes share this contract:
//
//   - Static: Trace == nil. Walk Demo definition only. Suitable for
//     declarative demos and sidecar markdown sources.
//   - Trace:  Trace == loaded entries. Post-hoc record from --record.
//   - Live:   Trace == partial entries; State == current snapshot.
//     Reserved for future WebSocket-driven embeds.
type RenderContext struct {
	// Demo is the definition: title, description, items, actors, declared
	// notes/arrows/refs. May be nil for trace-only renders without a
	// document header.
	Demo *Demo
	// Trace is the recorded path of one execution. Nil means a static
	// render; an empty non-nil slice produces an explicit empty-trace
	// marker in document renderers.
	Trace []TraceEntry
	// State is reserved for future live-state interpolation in WS
	// embeds. Renderers do not consume it today.
	State any
}

// EntryOpts controls per-entry rendering. Whole-document renderers
// compute these internally; incremental consumers (e.g. a WS embed
// pushing one fragment at a time) supply them by hand.
type EntryOpts struct {
	// StepNumber is the 1-based absolute index of this entry across the
	// current run. Used in step headings. Zero means "no number" — the
	// renderer falls back to the entry's title alone.
	StepNumber int
}
