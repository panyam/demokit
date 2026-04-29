package demokit

import (
	"fmt"
	"html"
	"sort"
	"strings"
)

// MarkdownFromTrace renders a recorded trace as markdown documentation.
// Unlike Demo.Markdown (which describes the demo's static declaration),
// this captures the actual path that was visited — useful for branching
// graph-mode demos where one document = one playthrough.
//
// Pass d for title/description/refs context. Pass nil to render the
// trace alone with no header.
func MarkdownFromTrace(d *Demo, entries []TraceEntry) string {
	var b strings.Builder

	if d != nil {
		fmt.Fprintf(&b, "# %s\n\n", d.title)
		if d.description != "" {
			fmt.Fprintf(&b, "%s\n\n", d.description)
		}
	}

	if len(entries) == 0 {
		b.WriteString("_(empty trace)_\n")
		return b.String()
	}

	stepIdx := 0
	allRefs := map[string]Ref{} // url → ref, dedup
	stepByID := map[string]*StepDef{}
	if d != nil {
		for _, it := range d.items {
			if s, ok := it.(*StepDef); ok {
				stepByID[s.id] = s
			}
		}
	}

	b.WriteString("## Walkthrough\n\n")
	for _, e := range entries {
		switch e.Kind {
		case KindStep:
			stepIdx++
			title := e.Title
			if title == "" {
				title = e.StepID
			}
			fmt.Fprintf(&b, "### %d. %s", stepIdx, title)
			if e.Visit > 1 {
				fmt.Fprintf(&b, " _(visit %d)_", e.Visit)
			}
			b.WriteString("\n\n")

			if s, ok := stepByID[e.StepID]; ok && s.note != "" {
				fmt.Fprintf(&b, "%s\n\n", s.note)
			}

			if s, ok := stepByID[e.StepID]; ok && len(s.refs) > 0 {
				b.WriteString("> **References:** ")
				for i, ref := range s.refs {
					if i > 0 {
						b.WriteString(", ")
					}
					fmt.Fprintf(&b, "[%s](%s)", ref.Name, ref.URL)
					allRefs[ref.URL] = ref
				}
				b.WriteString("\n\n")
			}

			if len(e.Inputs) > 0 {
				b.WriteString("**Inputs:**\n\n")
				for _, k := range sortedKeys(e.Inputs) {
					fmt.Fprintf(&b, "- `%s` = `%v`\n", k, e.Inputs[k])
				}
				b.WriteString("\n")
			}

			if e.Output != "" {
				b.WriteString("```\n")
				b.WriteString(strings.TrimRight(e.Output, "\n"))
				b.WriteString("\n```\n\n")
			}

			if e.Status != StatusSuccess && (e.Message != "" || e.Label != "") {
				label := e.Label
				if label == "" {
					label = e.Status.DefaultLabel()
				}
				fmt.Fprintf(&b, "> **%s:** %s\n\n", label, e.Message)
			}

			if e.Next != "" {
				fmt.Fprintf(&b, "→ jumped to `%s`\n\n", e.Next)
			}

		case KindSection:
			fmt.Fprintf(&b, "### %s\n\n", e.Title)
			if e.Body != "" {
				fmt.Fprintf(&b, "%s\n\n", e.Body)
			}
		}
	}

	if len(allRefs) > 0 {
		b.WriteString("## References\n\n")
		urls := make([]string, 0, len(allRefs))
		for u := range allRefs {
			urls = append(urls, u)
		}
		sort.Strings(urls)
		for _, u := range urls {
			r := allRefs[u]
			fmt.Fprintf(&b, "- [%s](%s)\n", r.Name, r.URL)
		}
		b.WriteString("\n")
	}

	return b.String()
}

// HTMLFromTrace renders a recorded trace as a minimal standalone HTML
// document. Markup is intentionally plain — callers can apply their own
// stylesheet by post-processing or by wrapping the output.
func HTMLFromTrace(d *Demo, entries []TraceEntry) string {
	var b strings.Builder
	title := "Demo"
	if d != nil && d.title != "" {
		title = d.title
	}
	fmt.Fprintf(&b, "<!doctype html>\n<html>\n<head>\n<meta charset=\"utf-8\">\n<title>%s</title>\n",
		html.EscapeString(title))
	b.WriteString("<style>\n" +
		"body { font: 14px/1.5 -apple-system, system-ui, sans-serif; max-width: 760px; margin: 2em auto; padding: 0 1em; }\n" +
		"pre { background: #f5f5f5; padding: 0.6em 1em; overflow-x: auto; border-radius: 4px; }\n" +
		"blockquote { border-left: 4px solid #ddd; margin: 1em 0; padding: 0.2em 1em; color: #666; }\n" +
		".error { border-left-color: #c33; color: #c33; }\n" +
		".warning { border-left-color: #c80; color: #c80; }\n" +
		".info { border-left-color: #28c; color: #28c; }\n" +
		".jump { color: #888; font-style: italic; }\n" +
		"</style>\n</head>\n<body>\n")

	fmt.Fprintf(&b, "<h1>%s</h1>\n", html.EscapeString(title))
	if d != nil && d.description != "" {
		fmt.Fprintf(&b, "<p>%s</p>\n", html.EscapeString(d.description))
	}

	stepByID := map[string]*StepDef{}
	if d != nil {
		for _, it := range d.items {
			if s, ok := it.(*StepDef); ok {
				stepByID[s.id] = s
			}
		}
	}

	stepIdx := 0
	for _, e := range entries {
		switch e.Kind {
		case KindStep:
			stepIdx++
			title := e.Title
			if title == "" {
				title = e.StepID
			}
			fmt.Fprintf(&b, "<h2>%d. %s", stepIdx, html.EscapeString(title))
			if e.Visit > 1 {
				fmt.Fprintf(&b, " <small>(visit %d)</small>", e.Visit)
			}
			b.WriteString("</h2>\n")

			if s, ok := stepByID[e.StepID]; ok && s.note != "" {
				fmt.Fprintf(&b, "<p>%s</p>\n", html.EscapeString(s.note))
			}

			if len(e.Inputs) > 0 {
				b.WriteString("<p><strong>Inputs:</strong></p>\n<ul>\n")
				for _, k := range sortedKeys(e.Inputs) {
					fmt.Fprintf(&b, "  <li><code>%s</code> = <code>%v</code></li>\n",
						html.EscapeString(k), e.Inputs[k])
				}
				b.WriteString("</ul>\n")
			}

			if e.Output != "" {
				fmt.Fprintf(&b, "<pre>%s</pre>\n", html.EscapeString(strings.TrimRight(e.Output, "\n")))
			}

			if e.Status != StatusSuccess && e.Message != "" {
				class := statusCSSClass(e.Status)
				label := e.Label
				if label == "" {
					label = e.Status.DefaultLabel()
				}
				fmt.Fprintf(&b, "<blockquote class=\"%s\"><strong>%s:</strong> %s</blockquote>\n",
					class, html.EscapeString(label), html.EscapeString(e.Message))
			}

			if e.Next != "" {
				fmt.Fprintf(&b, "<p class=\"jump\">→ jumped to <code>%s</code></p>\n", html.EscapeString(e.Next))
			}

		case KindSection:
			fmt.Fprintf(&b, "<h2>%s</h2>\n", html.EscapeString(e.Title))
			if e.Body != "" {
				fmt.Fprintf(&b, "<p>%s</p>\n", html.EscapeString(e.Body))
			}
		}
	}

	b.WriteString("</body>\n</html>\n")
	return b.String()
}

func statusCSSClass(s ResultStatus) string {
	switch s {
	case StatusError:
		return "error"
	case StatusWarning:
		return "warning"
	case StatusInfo:
		return "info"
	}
	return ""
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
