package demokit

import (
	"fmt"
	"html"
	"sort"
	"strings"
)

// RenderEntryMD renders a single TraceEntry as a self-contained markdown
// fragment. No preamble, no walkthrough header, no global references
// list — those are document-level concerns. Inline references for the
// step are emitted in place; deduplicated aggregation happens only in
// RenderDocumentMD.
func RenderEntryMD(ctx RenderContext, entry TraceEntry, opts EntryOpts) string {
	var b strings.Builder
	switch entry.Kind {
	case KindStep:
		title := entry.Title
		if title == "" {
			title = entry.StepID
		}
		if opts.StepNumber > 0 {
			fmt.Fprintf(&b, "### %d. %s", opts.StepNumber, title)
		} else {
			fmt.Fprintf(&b, "### %s", title)
		}
		if entry.Visit > 1 {
			fmt.Fprintf(&b, " _(visit %d)_", entry.Visit)
		}
		b.WriteString("\n\n")

		s := lookupStep(ctx.Demo, entry.StepID)
		if s != nil && s.note != "" {
			fmt.Fprintf(&b, "%s\n\n", s.note)
		}
		if s != nil && len(s.refs) > 0 {
			b.WriteString("> **References:** ")
			for i, ref := range s.refs {
				if i > 0 {
					b.WriteString(", ")
				}
				fmt.Fprintf(&b, "[%s](%s)", ref.Name, ref.URL)
			}
			b.WriteString("\n\n")
		}

		if len(entry.Inputs) > 0 {
			b.WriteString("**Inputs:**\n\n")
			for _, k := range sortedKeys(entry.Inputs) {
				fmt.Fprintf(&b, "- `%s` = `%v`\n", k, entry.Inputs[k])
			}
			b.WriteString("\n")
		}

		if entry.Output != "" {
			b.WriteString("```\n")
			b.WriteString(strings.TrimRight(entry.Output, "\n"))
			b.WriteString("\n```\n\n")
		}

		if entry.Status != StatusSuccess && (entry.Message != "" || entry.Label != "") {
			label := entry.Label
			if label == "" {
				label = entry.Status.DefaultLabel()
			}
			fmt.Fprintf(&b, "> **%s:** %s\n\n", label, entry.Message)
		}

		if entry.Next != "" {
			fmt.Fprintf(&b, "→ jumped to `%s`\n\n", entry.Next)
		}

	case KindSection:
		fmt.Fprintf(&b, "### %s\n\n", entry.Title)
		if entry.Body != "" {
			fmt.Fprintf(&b, "%s\n\n", entry.Body)
		}
	}
	return b.String()
}

// RenderDocumentMD renders the full markdown document: title +
// description preamble, "Walkthrough" header, per-entry fragments in
// order, and a deduplicated references footer.
func RenderDocumentMD(ctx RenderContext) string {
	var b strings.Builder

	if ctx.Demo != nil {
		fmt.Fprintf(&b, "# %s\n\n", ctx.Demo.title)
		if ctx.Demo.description != "" {
			fmt.Fprintf(&b, "%s\n\n", ctx.Demo.description)
		}
	}

	if len(ctx.Trace) == 0 {
		b.WriteString("_(empty trace)_\n")
		return b.String()
	}

	b.WriteString("## Walkthrough\n\n")
	stepIdx := 0
	for _, e := range ctx.Trace {
		opts := EntryOpts{}
		if e.Kind == KindStep {
			stepIdx++
			opts.StepNumber = stepIdx
		}
		b.WriteString(RenderEntryMD(ctx, e, opts))
	}

	if refs := collectRefsFromTrace(ctx); len(refs) > 0 {
		b.WriteString("## References\n\n")
		urls := make([]string, 0, len(refs))
		for u := range refs {
			urls = append(urls, u)
		}
		sort.Strings(urls)
		for _, u := range urls {
			r := refs[u]
			fmt.Fprintf(&b, "- [%s](%s)\n", r.Name, r.URL)
		}
		b.WriteString("\n")
	}

	return b.String()
}

// MarkdownFromTrace is a thin compatibility wrapper around
// RenderDocumentMD. Prefer RenderDocumentMD in new code.
func MarkdownFromTrace(d *Demo, entries []TraceEntry) string {
	return RenderDocumentMD(RenderContext{Demo: d, Trace: entries})
}

// RenderEntryHTML renders a single TraceEntry as a self-contained HTML
// fragment. No doctype, no <head>, no surrounding document chrome —
// callers compose those in RenderDocumentHTML.
func RenderEntryHTML(ctx RenderContext, entry TraceEntry, opts EntryOpts) string {
	var b strings.Builder
	switch entry.Kind {
	case KindStep:
		title := entry.Title
		if title == "" {
			title = entry.StepID
		}
		if opts.StepNumber > 0 {
			fmt.Fprintf(&b, "<h2>%d. %s", opts.StepNumber, html.EscapeString(title))
		} else {
			fmt.Fprintf(&b, "<h2>%s", html.EscapeString(title))
		}
		if entry.Visit > 1 {
			fmt.Fprintf(&b, " <small>(visit %d)</small>", entry.Visit)
		}
		b.WriteString("</h2>\n")

		s := lookupStep(ctx.Demo, entry.StepID)
		if s != nil && s.note != "" {
			fmt.Fprintf(&b, "<p>%s</p>\n", html.EscapeString(s.note))
		}

		if len(entry.Inputs) > 0 {
			b.WriteString("<p><strong>Inputs:</strong></p>\n<ul>\n")
			for _, k := range sortedKeys(entry.Inputs) {
				fmt.Fprintf(&b, "  <li><code>%s</code> = <code>%v</code></li>\n",
					html.EscapeString(k), entry.Inputs[k])
			}
			b.WriteString("</ul>\n")
		}

		if entry.Output != "" {
			fmt.Fprintf(&b, "<pre>%s</pre>\n", html.EscapeString(strings.TrimRight(entry.Output, "\n")))
		}

		if entry.Status != StatusSuccess && entry.Message != "" {
			class := statusCSSClass(entry.Status)
			label := entry.Label
			if label == "" {
				label = entry.Status.DefaultLabel()
			}
			fmt.Fprintf(&b, "<blockquote class=\"%s\"><strong>%s:</strong> %s</blockquote>\n",
				class, html.EscapeString(label), html.EscapeString(entry.Message))
		}

		if entry.Next != "" {
			fmt.Fprintf(&b, "<p class=\"jump\">→ jumped to <code>%s</code></p>\n", html.EscapeString(entry.Next))
		}

	case KindSection:
		fmt.Fprintf(&b, "<h2>%s</h2>\n", html.EscapeString(entry.Title))
		if entry.Body != "" {
			fmt.Fprintf(&b, "<p>%s</p>\n", html.EscapeString(entry.Body))
		}
	}
	return b.String()
}

// RenderDocumentHTML renders a minimal standalone HTML document built
// from the same per-entry fragments.
func RenderDocumentHTML(ctx RenderContext) string {
	var b strings.Builder
	title := "Demo"
	if ctx.Demo != nil && ctx.Demo.title != "" {
		title = ctx.Demo.title
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
	if ctx.Demo != nil && ctx.Demo.description != "" {
		fmt.Fprintf(&b, "<p>%s</p>\n", html.EscapeString(ctx.Demo.description))
	}

	stepIdx := 0
	for _, e := range ctx.Trace {
		opts := EntryOpts{}
		if e.Kind == KindStep {
			stepIdx++
			opts.StepNumber = stepIdx
		}
		b.WriteString(RenderEntryHTML(ctx, e, opts))
	}

	b.WriteString("</body>\n</html>\n")
	return b.String()
}

// HTMLFromTrace is a thin compatibility wrapper around RenderDocumentHTML.
// Prefer RenderDocumentHTML in new code.
func HTMLFromTrace(d *Demo, entries []TraceEntry) string {
	return RenderDocumentHTML(RenderContext{Demo: d, Trace: entries})
}

// lookupStep finds a step by ID in the demo's items list, or nil if the
// demo is nil or the step isn't found.
func lookupStep(d *Demo, id string) *StepDef {
	if d == nil {
		return nil
	}
	for _, it := range d.items {
		if s, ok := it.(*StepDef); ok && s.id == id {
			return s
		}
	}
	return nil
}

// collectRefsFromTrace walks the trace and gathers every step's refs,
// keyed by URL for deduplication.
func collectRefsFromTrace(ctx RenderContext) map[string]Ref {
	out := map[string]Ref{}
	if ctx.Demo == nil {
		return out
	}
	for _, e := range ctx.Trace {
		if e.Kind != KindStep {
			continue
		}
		s := lookupStep(ctx.Demo, e.StepID)
		if s == nil {
			continue
		}
		for _, ref := range s.refs {
			out[ref.URL] = ref
		}
	}
	return out
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
