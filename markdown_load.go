package demokit

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
	"gopkg.in/yaml.v3"
)

// FromMarkdown loads demo content (title, description, actors, items —
// notes, mermaid arrows, refs, declared inputs, sections) from a sidecar
// markdown file. Behavior (Run, Coalesce, custom Parse closures) stays
// in Go and attaches to loaded steps via Bind(id).
//
// File format conventions:
//
//   - Optional YAML frontmatter (between "---\n" and "---\n") for
//     title/description/actors. If a key is present, it overrides the
//     same field set via Demo.Description() / .Actors() before this
//     call. Fields set after FromMarkdown override the markdown.
//   - Each "## heading" starts a new item. The heading's optional
//     "{#id}" suffix is the join key; without it, the title is
//     slugified to a default id.
//   - A blockquote ("> ...") under a heading becomes the step's note.
//   - Three reserved fenced info-strings: `mermaid` (arrow lines),
//     `inputs` (YAML), `refs` (YAML). Other prose under a heading is
//     treated as the section body when the heading isn't bound.
//   - Step vs section is decided by *content shape*: any of
//     [blockquote, mermaid arrows, inputs, refs] makes it a step;
//     prose-only headings are sections.
//
// Errors (missing file, malformed frontmatter, unknown input type)
// are stored on the Demo and surface at Execute time with a
// descriptive stderr message — FromMarkdown never panics or returns
// an error directly so chaining stays clean.
func (d *Demo) FromMarkdown(path string) *Demo {
	src, err := os.ReadFile(path)
	if err != nil {
		d.loadError = fmt.Errorf("FromMarkdown(%s): %w", path, err)
		return d
	}
	return d.FromMarkdownBytes(src)
}

// FromMarkdownBytes loads sidecar content from an in-memory byte slice
// instead of a file path. The same parsing rules as FromMarkdown apply.
//
// Typical use: pair with go:embed so the demo binary carries its own
// content and runs identically regardless of the invoker's working
// directory:
//
//	//go:embed demo.md
//	var demoMD []byte
//
//	demo := demokit.New("placeholder").FromMarkdownBytes(demoMD)
//
// Errors are stored on the Demo (same as FromMarkdown) and surface at
// Execute time; this method never returns an error or panics.
func (d *Demo) FromMarkdownBytes(src []byte) *Demo {
	fm, body, err := splitFrontmatter(src)
	if err != nil {
		d.loadError = err
		return d
	}
	if fm != nil {
		if fm.Title != "" {
			d.title = fm.Title
		}
		if fm.Description != "" {
			d.description = fm.Description
		}
		if len(fm.Actors) > 0 {
			d.actors = fm.Actors
		}
	}

	loaded, warnings, err := parseItems(body)
	if err != nil {
		d.loadError = err
		return d
	}
	d.loadWarnings = append(d.loadWarnings, warnings...)

	for _, li := range loaded {
		if li.isStep() {
			d.items = append(d.items, li.toStep())
			d.stepCount++
		} else {
			d.items = append(d.items, li.toSection())
		}
	}
	return d
}

// Bind attaches Go-side behavior (Run, Coalesce, etc.) to a step that
// was loaded by FromMarkdown. Returns the existing *StepDef so the
// caller can chain setters; setters on the returned value override
// whatever the markdown declared.
//
// If id doesn't match any loaded step, the bind error is recorded on
// the Demo and a discard *StepDef is returned (so chained setters
// don't panic). Errors surface at Execute time with a clear message.
func (d *Demo) Bind(id string) *StepDef {
	for _, it := range d.items {
		if s, ok := it.(*StepDef); ok && s.id == id {
			return s
		}
	}
	d.bindErrors = append(d.bindErrors, id)
	return &StepDef{} // discard — keeps chained setters compiling
}

// --- frontmatter ---

type frontmatter struct {
	Title       string     `yaml:"title"`
	Description string     `yaml:"description"`
	Actors      []ActorDef `yaml:"actors"`
}

// splitFrontmatter peels off a leading "---\n...\n---\n" YAML block
// (if present) and returns the parsed frontmatter plus the remaining
// markdown body.
func splitFrontmatter(src []byte) (*frontmatter, []byte, error) {
	const delim = "---\n"
	if !bytes.HasPrefix(src, []byte(delim)) {
		return nil, src, nil
	}
	rest := src[len(delim):]
	end := bytes.Index(rest, []byte("\n---\n"))
	if end < 0 {
		// Allow trailing "---" without a final newline (common when
		// editors strip trailing whitespace).
		end = bytes.Index(rest, []byte("\n---"))
		if end < 0 || end != len(rest)-4 {
			return nil, nil, fmt.Errorf("frontmatter missing closing ---")
		}
	}
	fmBytes := rest[:end]
	var body []byte
	if end+5 <= len(rest) {
		body = rest[end+5:] // skip "\n---\n"
	}
	var fm frontmatter
	if err := yaml.Unmarshal(fmBytes, &fm); err != nil {
		return nil, nil, fmt.Errorf("frontmatter YAML: %w", err)
	}
	return &fm, body, nil
}

// --- AST walking ---

// loadedItem is the intermediate parsed-md form. Classified into
// *StepDef or *SectionDef based on content shape (isStep).
type loadedItem struct {
	id     string
	title  string
	note   string
	body   string
	arrows []arrowDef
	refs   []Ref
	inputs []InputDef
}

func (li loadedItem) isStep() bool {
	return li.note != "" ||
		len(li.arrows) > 0 ||
		len(li.refs) > 0 ||
		len(li.inputs) > 0
}

func (li loadedItem) toStep() *StepDef {
	return &StepDef{
		id:     li.id,
		title:  li.title,
		note:   li.note,
		arrows: li.arrows,
		refs:   li.refs,
		inputs: li.inputs,
	}
}

func (li loadedItem) toSection() *SectionDef {
	body := li.body
	if body == "" {
		// Prose-empty section is unusual but legal — preserve at
		// least the title in renders.
		body = ""
	}
	return &SectionDef{title: li.title, body: body}
}

// parseItems walks the markdown AST, splitting top-level content into
// items at every "## heading" boundary. Returns the items in source
// order plus any non-fatal warnings (e.g. unrecognized mermaid lines).
func parseItems(body []byte) ([]loadedItem, []string, error) {
	md := goldmark.New()
	root := md.Parser().Parse(text.NewReader(body))

	var items []loadedItem
	var warnings []string
	var current *loadedItem

	finalize := func() {
		if current != nil {
			items = append(items, *current)
			current = nil
		}
	}

	for n := root.FirstChild(); n != nil; n = n.NextSibling() {
		if h, ok := n.(*ast.Heading); ok && h.Level == 2 {
			finalize()
			title, id := parseHeading(h, body)
			current = &loadedItem{title: title, id: id}
			continue
		}
		if current == nil {
			// Content before the first ## heading — outside our
			// model. Warn so the author isn't surprised.
			warnings = append(warnings,
				fmt.Sprintf("content before first ## heading is ignored: %T", n))
			continue
		}

		switch x := n.(type) {
		case *ast.Blockquote:
			text := blockquoteText(x, body)
			if text == "" {
				continue
			}
			if current.note == "" {
				current.note = text
			} else {
				current.note = current.note + "\n\n" + text
			}

		case *ast.FencedCodeBlock:
			info := strings.TrimSpace(string(x.Language(body)))
			content := codeBlockText(x, body)
			switch info {
			case "mermaid":
				arrows, mw := parseMermaidArrows(content)
				current.arrows = append(current.arrows, arrows...)
				warnings = append(warnings, mw...)
			case "inputs":
				ins, err := parseInputsBlock(content)
				if err != nil {
					return nil, nil, fmt.Errorf("step %q inputs: %w", current.id, err)
				}
				current.inputs = append(current.inputs, ins...)
			case "refs":
				rs, err := parseRefsBlock(content)
				if err != nil {
					return nil, nil, fmt.Errorf("step %q refs: %w", current.id, err)
				}
				current.refs = append(current.refs, rs...)
			default:
				// Unknown info-string: preserve as part of the body
				// prose so future renderers can handle it.
				if current.body != "" {
					current.body += "\n\n"
				}
				current.body += "```" + info + "\n" + content + "```"
			}

		case *ast.Paragraph:
			text := nodeRawLines(x, body)
			if text == "" {
				continue
			}
			if current.body != "" {
				current.body += "\n\n"
			}
			current.body += text

		default:
			// Lists, tables, raw HTML, etc. — preserve via line
			// extraction. Goldmark's container nodes don't always
			// expose Lines() meaningfully; this is best-effort.
			if text := nodeRawLines(x, body); text != "" {
				if current.body != "" {
					current.body += "\n\n"
				}
				current.body += text
			}
		}
	}
	finalize()
	return items, warnings, nil
}

var anchorRegex = regexp.MustCompile(`\s*\{#([\w\-.]+)\}\s*$`)

// parseHeading extracts the title text and the {#id} anchor from a
// level-2 heading. Without an explicit anchor, the id is slugified
// from the title.
func parseHeading(h *ast.Heading, src []byte) (title, id string) {
	var raw strings.Builder
	for c := h.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			raw.Write(t.Value(src))
			continue
		}
		// For inline emphasis/code etc. fall back to writing the
		// node's raw segment span — best effort.
		if seg := c.Lines(); seg != nil && seg.Len() > 0 {
			for i := range seg.Len() {
				s := seg.At(i)
				raw.Write(s.Value(src))
			}
		}
	}
	full := strings.TrimSpace(raw.String())
	if m := anchorRegex.FindStringSubmatch(full); m != nil {
		id = m[1]
		title = strings.TrimSpace(strings.TrimSuffix(full, m[0]))
		return
	}
	title = full
	id = slugify(title)
	return
}

// slugify converts "Pick a Symptom" → "pick-a-symptom". ASCII-only;
// non-alnum collapsed to single hyphens.
func slugify(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteRune('-')
				prevDash = true
			}
		}
	}
	return strings.TrimRight(b.String(), "-")
}

// nodeRawLines reads the source bytes covered by the node's line
// segments. Works for paragraphs, code blocks, and other leaf-line
// containers; container nodes (blockquotes, lists) need a recursive
// helper.
func nodeRawLines(n ast.Node, src []byte) string {
	lines := n.Lines()
	if lines == nil || lines.Len() == 0 {
		return ""
	}
	var buf strings.Builder
	for i := range lines.Len() {
		s := lines.At(i)
		buf.Write(s.Value(src))
	}
	return strings.TrimRight(buf.String(), "\n")
}

// blockquoteText flattens a blockquote into plain markdown by reading
// the source lines of each child paragraph and joining with blank
// lines (preserves multi-paragraph blockquotes).
func blockquoteText(bq ast.Node, src []byte) string {
	var parts []string
	for c := bq.FirstChild(); c != nil; c = c.NextSibling() {
		if text := nodeRawLines(c, src); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n\n")
}

// codeBlockText reads the body of a fenced code block (excluding the
// opening/closing fences).
func codeBlockText(b *ast.FencedCodeBlock, src []byte) string {
	var buf strings.Builder
	lines := b.Lines()
	for i := range lines.Len() {
		s := lines.At(i)
		buf.Write(s.Value(src))
	}
	return buf.String()
}

// --- mermaid arrows ---

var mermaidArrowRegex = regexp.MustCompile(`^\s*(\S+)\s*(-->>|->>)\s*(\S+)\s*:\s*(.+?)\s*$`)

// parseMermaidArrows extracts arrow lines from a mermaid block.
// Recognized: "A ->> B: label" (solid) and "A -->> B: label" (dashed).
// Other syntax (participant, Note over, alt/loop, autonumber, etc.)
// produces a warning so the author knows it's being dropped.
func parseMermaidArrows(content string) ([]arrowDef, []string) {
	var arrows []arrowDef
	var warnings []string
	for line := range strings.SplitSeq(content, "\n") {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "%%") || trim == "sequenceDiagram" {
			continue
		}
		if m := mermaidArrowRegex.FindStringSubmatch(trim); m != nil {
			arrows = append(arrows, arrowDef{
				from:   m[1],
				to:     m[3],
				label:  m[4],
				dashed: m[2] == "-->>",
			})
			continue
		}
		warnings = append(warnings,
			fmt.Sprintf("mermaid: dropping unsupported line %q (only ->>/--> arrows are modeled)", trim))
	}
	return arrows, warnings
}

// --- inputs / refs blocks ---

type inputDecl struct {
	Name    string   `yaml:"name"`
	Prompt  string   `yaml:"prompt"`
	Default any      `yaml:"default"`
	Type    string   `yaml:"type"`
	Options []string `yaml:"options"`
}

func parseInputsBlock(content string) ([]InputDef, error) {
	var decls []inputDecl
	if err := yaml.Unmarshal([]byte(content), &decls); err != nil {
		return nil, fmt.Errorf("parse inputs YAML: %w", err)
	}
	var out []InputDef
	for _, d := range decls {
		var in InputDef
		switch d.Type {
		case "string", "":
			in = String()
		case "int":
			in = Int()
		case "choice":
			in = Choice(d.Options...)
		default:
			return nil, fmt.Errorf("unknown input type %q for input %q (want string|int|choice)", d.Type, d.Name)
		}
		in.Name = d.Name
		in.Prompt = d.Prompt
		if d.Default != nil {
			in.Default = d.Default
		}
		out = append(out, in)
	}
	return out, nil
}

type refDecl struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

func parseRefsBlock(content string) ([]Ref, error) {
	var decls []refDecl
	if err := yaml.Unmarshal([]byte(content), &decls); err != nil {
		return nil, fmt.Errorf("parse refs YAML: %w", err)
	}
	var out []Ref
	for _, d := range decls {
		out = append(out, Ref{Name: d.Name, URL: d.URL})
	}
	return out, nil
}
