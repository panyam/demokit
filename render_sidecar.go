package demokit

import (
	"fmt"
	"strings"
)

// Sidecar renders the demo definition as sidecar markdown — the inverse of
// FromMarkdown. It is a pure function of the definition (it never runs
// steps), so it is the robust way to lift an inline, Go-defined walkthrough's
// content into a demo.md: behavior (Run/Coalesce closures) stays in Go, while
// notes, arrows, refs, declared inputs, and verbatim blocks move to markdown.
//
// Round-trip: loading a demo.md and re-emitting via Sidecar reproduces the
// same content (modulo formatting). Emitted by `--doc sidecar`.
//
// Step ids are the explicit StepDef id when set, else the slugified title,
// deduplicated in declaration order so a heading's {#id} matches what Bind
// expects.
func (d *Demo) Sidecar() string {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "title: %s\n", d.title)
	if d.description != "" {
		fmt.Fprintf(&b, "description: %s\n", d.description)
	}
	if len(d.actors) > 0 {
		b.WriteString("actors:\n")
		for _, a := range d.actors {
			fmt.Fprintf(&b, "  - { id: %s, label: %s }\n", a.ID, a.Label)
		}
	}
	b.WriteString("---\n\n")

	used := map[string]int{}
	for _, it := range d.items {
		switch v := it.(type) {
		case *SectionDef:
			fmt.Fprintf(&b, "## %s {#%s}\n\n", v.title, sidecarID("", v.title, used))
			if v.body != "" {
				b.WriteString(v.body + "\n\n")
			}
		case *StepDef:
			fmt.Fprintf(&b, "## %s {#%s}\n\n", v.title, sidecarID(v.id, v.title, used))
			writeNoteSidecar(&b, v.note)
			writeArrowsSidecar(&b, v.arrows)
			writeInputsSidecar(&b, v.inputs)
			writeRefsSidecar(&b, v.refs)
			writeVerbatimSidecar(&b, v.verbatim)
		}
	}
	return b.String()
}

func writeNoteSidecar(b *strings.Builder, note string) {
	if note == "" {
		return
	}
	for line := range strings.SplitSeq(note, "\n") {
		if line == "" {
			b.WriteString(">\n")
		} else {
			b.WriteString("> " + line + "\n")
		}
	}
	b.WriteString("\n")
}

func writeArrowsSidecar(b *strings.Builder, arrows []arrowDef) {
	if len(arrows) == 0 {
		return
	}
	b.WriteString("```mermaid\n")
	for _, a := range arrows {
		op := "->>"
		if a.dashed {
			op = "-->>"
		}
		fmt.Fprintf(b, "%s %s %s: %s\n", a.from, op, a.to, a.label)
	}
	b.WriteString("```\n\n")
}

func writeInputsSidecar(b *strings.Builder, inputs []InputDef) {
	if len(inputs) == 0 {
		return
	}
	b.WriteString("```inputs\n")
	for _, in := range inputs {
		fmt.Fprintf(b, "- name: %s\n", in.Name)
		if in.Prompt != "" {
			fmt.Fprintf(b, "  prompt: %s\n", in.Prompt)
		}
		if in.Kind != "" {
			fmt.Fprintf(b, "  type: %s\n", in.Kind)
		}
		if len(in.Options) > 0 {
			fmt.Fprintf(b, "  options: [%s]\n", strings.Join(in.Options, ", "))
		}
		if in.Default != nil {
			fmt.Fprintf(b, "  default: %v\n", in.Default)
		}
	}
	b.WriteString("```\n\n")
}

func writeRefsSidecar(b *strings.Builder, refs []Ref) {
	if len(refs) == 0 {
		return
	}
	b.WriteString("```refs\n")
	for _, r := range refs {
		fmt.Fprintf(b, "- name: %s\n  url: %s\n", r.Name, r.URL)
	}
	b.WriteString("```\n\n")
}

// writeVerbatimSidecar emits verbatim blocks in the attribute-fence form the
// loader reads: a shared verbatim="<title>" per block, per-variant label and
// default=true, tilde fences so a ``` body can nest.
func writeVerbatimSidecar(b *strings.Builder, blocks []verbatimBlock) {
	for _, v := range blocks {
		for _, va := range v.Variants {
			attrs := fmt.Sprintf("verbatim=%q", v.Label)
			if va.Label != "" {
				attrs += " label=" + va.Label
			}
			if va.IsDefault {
				attrs += " default=true"
			}
			fmt.Fprintf(b, "~~~%s {%s}\n", va.Lang, attrs)
			b.WriteString(strings.TrimRight(va.Content, "\n"))
			b.WriteString("\n~~~\n")
		}
		b.WriteString("\n")
	}
}

// sidecarID resolves a stable heading id: explicit id, else slugified title,
// deduplicated in declaration order (foo, foo-2, foo-3). Shares slugify with
// the loader so a round-trip keeps the same ids.
func sidecarID(explicit, title string, used map[string]int) string {
	id := explicit
	if id == "" {
		id = slugify(title)
	}
	if id == "" {
		id = "step"
	}
	n := used[id]
	used[id]++
	if n == 0 {
		return id
	}
	return fmt.Sprintf("%s-%d", id, n+1)
}
