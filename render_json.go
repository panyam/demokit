package demokit

import (
	"bytes"
	"encoding/json"
)

// docOutput is the top-level JSON envelope. Both fields are omitempty so
// that static renders (definition only) and trace-only renders produce
// asymmetric documents — embed hosts use the presence of "trace" as the
// mode discriminator.
type docOutput struct {
	Demo  *demoView    `json:"demo,omitempty"`
	Trace []TraceEntry `json:"trace,omitempty"`
}

// demoView is the JSON-serializable projection of a *Demo. We project
// (rather than tag *Demo directly) because Demo carries unexported state
// — flag values, recorder hooks, items as an interface slice — that
// shouldn't appear in the wire format.
type demoView struct {
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	Actors      []ActorDef `json:"actors,omitempty"`
	Items       []itemView `json:"items,omitempty"`
}

// itemView discriminates step and section items via the "kind" field,
// matching the TraceKind values used in trace entries. Step-only fields
// are omitempty for sections, and vice versa.
type itemView struct {
	Kind   TraceKind   `json:"kind"`
	ID     string      `json:"id,omitempty"`
	Title  string      `json:"title"`
	Note   string      `json:"note,omitempty"`
	Body   string      `json:"body,omitempty"` // sections only
	Arrows []ArrowView `json:"arrows,omitempty"`
	Refs   []Ref       `json:"refs,omitempty"`
	Inputs []inputView `json:"inputs,omitempty"`
}

// inputView projects an InputDef without its Parse closure (which has
// no JSON representation). Carries the declarative metadata authors
// see — Name/Prompt/Default plus the typed shape (Kind/Options) — so
// embed hosts can render proper choice pickers, int validators, etc.
type inputView struct {
	Name    string   `json:"name"`
	Prompt  string   `json:"prompt,omitempty"`
	Default any      `json:"default,omitempty"`
	Kind    string   `json:"kind,omitempty"`
	Options []string `json:"options,omitempty"`
}

// RenderDocumentJSON renders the demo definition (and optionally a
// trace) as a single JSON object. Suitable for consumption by web embeds
// and other host renderers that prefer structured data over rendered
// markup.
//
// Output is pretty-printed with two-space indentation and a trailing
// newline so that piping to a file produces a readable artifact.
func RenderDocumentJSON(ctx RenderContext) string {
	out := docOutput{}
	if ctx.Demo != nil {
		out.Demo = projectDemo(ctx.Demo)
	}
	out.Trace = ctx.Trace

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	// Encoder.Encode only fails when the value contains an unsupported
	// type; our view structs deliberately exclude those, so an error
	// here would be a programming bug worth surfacing.
	if err := enc.Encode(out); err != nil {
		panic("demokit: RenderDocumentJSON encode failed: " + err.Error())
	}
	return buf.String()
}

// JSONFromTrace is the trace-mode wrapper, mirroring RenderDocumentJSON
// for callers that prefer the (demo, entries) shape.
func JSONFromTrace(d *Demo, entries []TraceEntry) string {
	return RenderDocumentJSON(RenderContext{Demo: d, Trace: entries})
}

// JSON is the static-mode entry point on Demo, mirroring Markdown().
// Suitable for embed hosts loading the definition once at page init.
func (d *Demo) JSON() string {
	return RenderDocumentJSON(RenderContext{Demo: d})
}

// projectDemo builds the JSON-serializable view of a Demo.
func projectDemo(d *Demo) *demoView {
	v := &demoView{
		Title:       d.title,
		Description: d.description,
		Actors:      d.actors,
	}
	for _, it := range d.items {
		switch x := it.(type) {
		case *StepDef:
			iv := itemView{
				Kind:   KindStep,
				ID:     x.id,
				Title:  x.title,
				Note:   x.note,
				Arrows: x.Arrows(),
				Refs:   x.refs,
			}
			for _, in := range x.inputs {
				iv.Inputs = append(iv.Inputs, inputView{
					Name:    in.Name,
					Prompt:  in.Prompt,
					Default: in.Default,
					Kind:    in.Kind,
					Options: in.Options,
				})
			}
			v.Items = append(v.Items, iv)
		case *SectionDef:
			v.Items = append(v.Items, itemView{
				Kind:  KindSection,
				Title: x.title,
				Body:  x.body,
			})
		}
	}
	return v
}
