package demokit

import (
	"fmt"
	"strings"
)

// Markdown generates static linear documentation from the demo
// definition. This is the canonical output for non-branching demos
// (run with --doc md to regenerate). For branching demos, prefer
// RenderDocumentMD which captures the actual visited path from a
// recorded trace (--doc md --from <trace>).
func (d *Demo) Markdown() string {
	var b strings.Builder

	// Title and description
	fmt.Fprintf(&b, "# %s\n\n", d.title)
	if d.description != "" {
		fmt.Fprintf(&b, "%s\n\n", d.description)
	}

	// Collect steps for the summary
	var steps []*StepDef
	for _, it := range d.items {
		if s, ok := it.(*StepDef); ok {
			steps = append(steps, s)
		}
	}

	// What you'll learn (from step notes)
	hasNotes := false
	for _, s := range steps {
		if s.note != "" {
			hasNotes = true
			break
		}
	}
	if hasNotes {
		b.WriteString("## What you'll learn\n\n")
		for _, s := range steps {
			if s.note != "" {
				// Summary is a one-line teaser; multi-line notes
				// would otherwise spill into sibling top-level
				// bullets. Full note still renders in the per-step
				// detail below.
				teaser := s.note
				if i := strings.IndexByte(teaser, '\n'); i >= 0 {
					teaser = teaser[:i]
				}
				fmt.Fprintf(&b, "- **%s** — %s\n", s.title, teaser)
			}
		}
		b.WriteString("\n")
	}

	// Sequence diagram — only meaningful when actors are declared. A
	// `sequenceDiagram` with no participants doesn't render usefully in
	// mermaid, so demos without actors skip the Flow section entirely
	// and rely on the per-step detail below.
	if len(d.actors) > 0 {
		b.WriteString("## Flow\n\n```mermaid\nsequenceDiagram\n")
		for _, a := range d.actors {
			if a.ID != a.Label {
				fmt.Fprintf(&b, "    participant %s as %s\n", a.ID, a.Label)
			} else {
				fmt.Fprintf(&b, "    participant %s\n", a.ID)
			}
		}
		stepNum := 0
		for _, it := range d.items {
			if v, ok := it.(*StepDef); ok {
				stepNum++
				fmt.Fprintf(&b, "\n    Note over %s,%s: Step %d: %s\n",
					d.actors[0].ID, d.actors[len(d.actors)-1].ID, stepNum, v.title)
				for _, a := range v.arrows {
					if a.dashed {
						fmt.Fprintf(&b, "    %s-->>%s: %s\n", a.from, a.to, a.label)
					} else {
						fmt.Fprintf(&b, "    %s->>%s: %s\n", a.from, a.to, a.label)
					}
				}
			}
		}
		b.WriteString("```\n\n")
	}

	// Steps detail
	b.WriteString("## Steps\n\n")
	stepNum := 0
	allRefs := make(map[string]Ref) // dedup by URL
	for _, it := range d.items {
		switch v := it.(type) {
		case *StepDef:
			stepNum++
			fmt.Fprintf(&b, "### Step %d: %s\n\n", stepNum, v.title)
			if len(v.refs) > 0 {
				b.WriteString("> **References:** ")
				for i, ref := range v.refs {
					if i > 0 {
						b.WriteString(", ")
					}
					fmt.Fprintf(&b, "[%s](%s)", ref.Name, ref.URL)
					allRefs[ref.URL] = ref
				}
				b.WriteString("\n\n")
			}
			if v.note != "" {
				fmt.Fprintf(&b, "%s\n\n", v.note)
			}
		case *SectionDef:
			fmt.Fprintf(&b, "### %s\n\n%s\n\n", v.title, v.body)
		}
	}

	// Collected references (deduped)
	if len(allRefs) > 0 {
		b.WriteString("## References\n\n")
		for _, ref := range allRefs {
			fmt.Fprintf(&b, "- [%s](%s)\n", ref.Name, ref.URL)
		}
		b.WriteString("\n")
	}

	// Run command
	dir := d.dir
	if dir == "" {
		dir = "<this-directory>"
	}
	runPath := dir
	if d.runPrefix != "" {
		runPath = d.runPrefix + "/" + dir
	}
	b.WriteString("## Run it\n\n")
	fmt.Fprintf(&b, "```bash\ngo run ./%s/\n```\n\n", runPath)
	b.WriteString("Pass `--non-interactive` to skip pauses:\n\n")
	fmt.Fprintf(&b, "```bash\ngo run ./%s/ --non-interactive\n```\n", runPath)

	return b.String()
}
