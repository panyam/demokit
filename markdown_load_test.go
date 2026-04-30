package demokit

import (
	"os"
	"strings"
	"testing"
)

// fixtureFullMarkdown is a sidecar md that exercises every loader
// feature in a single document: frontmatter, an explicit-anchor
// section, a step with note + mermaid + refs + inputs, a step with
// an implicit (slugified) anchor, and a prose-only section.
const fixtureFullMarkdown = `---
title: Auth Failure Triage
description: Pick a symptom and walk the recovery path
actors:
  - { id: User, label: User }
  - { id: App,  label: App }
  - { id: AS,   label: Auth Server }
---

## How this demo works {#how-it-works}

You'll be asked to pick a failure symptom. Each branch shows the
recovery flow for that case.

## Pick a symptom {#triage}

> Most auth failures fall into a handful of buckets.

` + "```inputs\n" + `- name: symptom
  prompt: Symptom (expired/scope/ratelimit)
  type: choice
  options: [expired, scope, ratelimit]
  default: expired
- name: retries
  type: int
  default: 3
` + "```" + `

## Expired token

` + "```mermaid\n" + `App ->> AS: GET /api (Bearer expired)
AS -->> App: 401 token_expired
` + "```" + `

> The access token's TTL has elapsed; the API rejects it.

` + "```refs\n" + `- name: RFC 6749 §5.2
  url: https://www.rfc-editor.org/rfc/rfc6749#section-5.2
` + "```" + `
`

// TestFromMarkdownEndToEnd verifies a sidecar with frontmatter, sections,
// steps, blockquote notes, mermaid arrows, refs, and typed inputs all
// load into the expected Demo state. Catches a wholesale loader
// regression.
func TestFromMarkdownEndToEnd(t *testing.T) {
	d := New("placeholder").FromMarkdownBytes([]byte(fixtureFullMarkdown))
	if d.loadError != nil {
		t.Fatalf("unexpected loadError: %v", d.loadError)
	}

	if d.title != "Auth Failure Triage" {
		t.Errorf("frontmatter title not applied: %q", d.title)
	}
	if d.description != "Pick a symptom and walk the recovery path" {
		t.Errorf("frontmatter description not applied: %q", d.description)
	}
	if len(d.actors) != 3 ||
		d.actors[0].ID != "User" || d.actors[2].Label != "Auth Server" {
		t.Errorf("frontmatter actors not applied or out of order: %+v", d.actors)
	}

	if len(d.items) != 3 {
		t.Fatalf("expected 3 loaded items, got %d", len(d.items))
	}

	// Item 0: prose-only section (no blockquote, no fenced blocks).
	sec, ok := d.items[0].(*SectionDef)
	if !ok {
		t.Fatalf("items[0] = %T, want *SectionDef", d.items[0])
	}
	if sec.title != "How this demo works" {
		t.Errorf("section title = %q", sec.title)
	}
	if !strings.Contains(sec.body, "Each branch") {
		t.Errorf("section body lost prose: %q", sec.body)
	}

	// Item 1: step with explicit anchor + inputs + blockquote note.
	step1, ok := d.items[1].(*StepDef)
	if !ok {
		t.Fatalf("items[1] = %T, want *StepDef", d.items[1])
	}
	if step1.id != "triage" || step1.title != "Pick a symptom" {
		t.Errorf("step1 id/title = %q / %q", step1.id, step1.title)
	}
	if !strings.Contains(step1.note, "handful of buckets") {
		t.Errorf("step1 note missing blockquote text: %q", step1.note)
	}
	if len(step1.inputs) != 2 {
		t.Fatalf("step1 inputs len = %d, want 2", len(step1.inputs))
	}
	if step1.inputs[0].Name != "symptom" || step1.inputs[0].Kind != "choice" {
		t.Errorf("inputs[0] = %+v", step1.inputs[0])
	}
	if !equalStrings(step1.inputs[0].Options, []string{"expired", "scope", "ratelimit"}) {
		t.Errorf("inputs[0].Options = %v", step1.inputs[0].Options)
	}
	if step1.inputs[1].Kind != "int" {
		t.Errorf("inputs[1].Kind = %q, want \"int\"", step1.inputs[1].Kind)
	}

	// Item 2: step with implicit anchor (slug "expired-token") and
	// mermaid + refs blocks.
	step2, ok := d.items[2].(*StepDef)
	if !ok {
		t.Fatalf("items[2] = %T, want *StepDef", d.items[2])
	}
	if step2.id != "expired-token" {
		t.Errorf("implicit slug id = %q, want \"expired-token\"", step2.id)
	}
	if len(step2.arrows) != 2 ||
		step2.arrows[0].from != "App" || !step2.arrows[1].dashed {
		t.Errorf("step2 arrows = %+v", step2.arrows)
	}
	if len(step2.refs) != 1 || step2.refs[0].Name != "RFC 6749 §5.2" {
		t.Errorf("step2 refs = %+v", step2.refs)
	}
}

// TestSplitFrontmatterEdgeCases verifies the frontmatter peeler handles
// the absence of frontmatter, an unterminated frontmatter, and an empty
// body after frontmatter without panicking or losing content.
func TestSplitFrontmatterEdgeCases(t *testing.T) {
	t.Run("no frontmatter", func(t *testing.T) {
		fm, body, err := splitFrontmatter([]byte("# Doc\n\nbody\n"))
		if err != nil || fm != nil {
			t.Errorf("expected no frontmatter, got fm=%v err=%v", fm, err)
		}
		if !strings.HasPrefix(string(body), "# Doc") {
			t.Errorf("body lost: %q", body)
		}
	})

	t.Run("unterminated frontmatter is an error", func(t *testing.T) {
		_, _, err := splitFrontmatter([]byte("---\ntitle: X\n\nbody\n"))
		if err == nil {
			t.Errorf("expected error for unterminated frontmatter")
		}
	})

	t.Run("empty body after frontmatter", func(t *testing.T) {
		fm, body, err := splitFrontmatter([]byte("---\ntitle: X\n---\n"))
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if fm == nil || fm.Title != "X" {
			t.Errorf("frontmatter not parsed: %+v", fm)
		}
		if len(body) != 0 {
			t.Errorf("body should be empty, got %q", body)
		}
	})
}

// TestHeadingAnchorAndSlug verifies that headings with explicit {#id}
// anchors keep the id verbatim while headings without one fall back to
// a slugified title. Pinning the anchor regex and the slug rules.
func TestHeadingAnchorAndSlug(t *testing.T) {
	src := []byte(`## Explicit Anchor {#my-anchor}

` + "```inputs" + `
- name: x
  type: string
` + "```" + `

## Lots of UPPER & symbols!

` + "```inputs" + `
- name: y
  type: string
` + "```" + `
`)
	d := New("").FromMarkdownBytes(src)
	if d.loadError != nil {
		t.Fatalf("loadError: %v", d.loadError)
	}
	if len(d.items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(d.items))
	}
	if d.items[0].(*StepDef).id != "my-anchor" {
		t.Errorf("explicit anchor lost: %q", d.items[0].(*StepDef).id)
	}
	if got := d.items[1].(*StepDef).id; got != "lots-of-upper-symbols" {
		t.Errorf("slug = %q, want \"lots-of-upper-symbols\"", got)
	}
}

// TestMermaidArrowParsing verifies the mermaid block parser extracts
// solid (->>) and dashed (-->>) arrows, ignores empty/comment lines and
// the sequenceDiagram keyword, and warns about unsupported syntax
// (participants, Note over, alt blocks) without dropping recognized
// arrows in the same block.
func TestMermaidArrowParsing(t *testing.T) {
	content := `sequenceDiagram
    participant App
    %% a comment
    App ->> AS: hello
    AS -->> App: 200 ok
    Note over App,AS: ignored note`

	arrows, warnings := parseMermaidArrows(content)
	if len(arrows) != 2 {
		t.Fatalf("arrows = %d, want 2: %+v", len(arrows), arrows)
	}
	if arrows[0].dashed || !arrows[1].dashed {
		t.Errorf("dashed flags wrong: %+v", arrows)
	}
	if arrows[0].label != "hello" || arrows[1].label != "200 ok" {
		t.Errorf("labels wrong: %+v", arrows)
	}
	if len(warnings) == 0 {
		t.Errorf("expected warnings for participant/Note over lines")
	}
}

// TestInputsBlockUnknownTypeIsAnError pins the inputs registry — an
// unknown type halts the load with a helpful error rather than silently
// dropping the input.
func TestInputsBlockUnknownTypeIsAnError(t *testing.T) {
	src := []byte(`## X

` + "```inputs" + `
- name: x
  type: nonsense
` + "```" + `
`)
	d := New("").FromMarkdownBytes(src)
	if d.loadError == nil || !strings.Contains(d.loadError.Error(), "nonsense") {
		t.Errorf("expected loadError mentioning \"nonsense\", got %v", d.loadError)
	}
}

// TestBindKnownIDReturnsLoadedStep verifies Bind returns the *StepDef
// loaded by FromMarkdown, and setters on the returned value override
// the markdown-supplied content (Go-wins-on-conflict).
func TestBindKnownIDReturnsLoadedStep(t *testing.T) {
	d := New("").FromMarkdownBytes([]byte(`## Hello {#hello}

> note from md
`))
	if d.loadError != nil {
		t.Fatalf("loadError: %v", d.loadError)
	}

	s := d.Bind("hello")
	if s == nil {
		t.Fatal("Bind returned nil")
	}
	if s.note != "note from md" {
		t.Errorf("md note not exposed: %q", s.note)
	}

	s.Note("note from go")
	if d.items[0].(*StepDef).note != "note from go" {
		t.Errorf("Go .Note() didn't override md: %q",
			d.items[0].(*StepDef).note)
	}
}

// TestBindUnknownIDDeferredErrorAtExecute verifies binding to an id
// that doesn't appear in the markdown is recorded but doesn't panic;
// Execute then writes a clear error to stderr and aborts before any
// step runs.
func TestBindUnknownIDDeferredErrorAtExecute(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"test", "--non-interactive"}

	d := New("").FromMarkdownBytes([]byte(`## Real {#real}

> some note
`))
	d.WithRenderer(&recordingRenderer{})

	// Should not panic even with no FromMarkdown match.
	s := d.Bind("ghost")
	if s == nil {
		t.Fatal("Bind returned nil")
	}
	s.Run(func(ctx StepContext) *StepResult { return nil })

	out, errOut := captureStdoutStderr(t, func() { d.Execute() })
	_ = out

	if !strings.Contains(errOut, "Bind to unknown step id") || !strings.Contains(errOut, "ghost") {
		t.Errorf("Execute should report the bad bind on stderr; got:\n%s", errOut)
	}
}

// TestInputReplaceByName verifies StepDef.Input matches by Name and
// replaces in place, not appending. Critical for sidecar override
// semantics: md declares inputs, Go can swap out a parser without
// rebuilding the entire input list.
func TestInputReplaceByName(t *testing.T) {
	s := &StepDef{}
	s.Input(String().Named("x", "X").WithDefault("a"))
	s.Input(Int().Named("y", "Y").WithDefault(1))
	if len(s.inputs) != 2 {
		t.Fatalf("setup: inputs = %d, want 2", len(s.inputs))
	}

	custom := func(string) (any, error) { return "custom", nil }
	s.Input(String().Named("x", "X").WithParse(custom))

	if len(s.inputs) != 2 {
		t.Errorf("replace-by-name should preserve length, got %d", len(s.inputs))
	}
	if s.inputs[0].Name != "x" {
		t.Errorf("order lost; inputs[0] = %q, want \"x\"", s.inputs[0].Name)
	}
	if got, _ := s.inputs[0].Parse("anything"); got != "custom" {
		t.Errorf("custom parser not applied: %v", got)
	}
}

// TestMixedModeMDThenInline verifies that calling .Step() after
// FromMarkdown appends inline steps to the loaded ones, preserving
// md-order followed by Go-order.
func TestMixedModeMDThenInline(t *testing.T) {
	d := New("").FromMarkdownBytes([]byte(`## A {#a}

> note for a
`))
	d.Step("Inline").ID("b")

	if len(d.items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(d.items))
	}
	if d.items[0].(*StepDef).id != "a" || d.items[1].(*StepDef).id != "b" {
		t.Errorf("order lost: %+v", []item{d.items[0], d.items[1]})
	}
}

// TestFromMarkdownMissingFileSurfacesAtExecute verifies a non-existent
// path is not a panic — Execute prints a stderr error and aborts.
func TestFromMarkdownMissingFileSurfacesAtExecute(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"test", "--non-interactive"}

	d := New("X").FromMarkdown("/no/such/file.md")
	d.WithRenderer(&recordingRenderer{})

	_, errOut := captureStdoutStderr(t, func() { d.Execute() })

	if !strings.Contains(errOut, "FromMarkdown") || !strings.Contains(errOut, "no such file") {
		t.Errorf("expected stderr to describe missing file, got: %s", errOut)
	}
}

// TestSidecarLoadAndJSONIncludesKindOptions verifies the loader's
// Kind/Options metadata flows through to JSON output — closing the
// Phase 2 gap where input type info wasn't surfaced.
func TestSidecarLoadAndJSONIncludesKindOptions(t *testing.T) {
	d := New("").FromMarkdownBytes([]byte(`## Pick {#pick}

` + "```inputs" + `
- name: kind
  type: choice
  options: [a, b, c]
  default: a
` + "```" + `
`))
	if d.loadError != nil {
		t.Fatalf("loadError: %v", d.loadError)
	}

	out := d.JSON()
	for _, want := range []string{
		`"kind": "choice"`,
		`"options"`,
		`"a"`,
		`"b"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("JSON missing %q:\n%s", want, out)
		}
	}
}

// TestSidecarExecuteWithRecorder is the end-to-end check: load a
// sidecar, attach Run via Bind, execute non-interactively with a
// recorder, and verify the resulting trace mentions the right step
// ids and inputs. Pins the full sidecar→runtime→trace pipeline.
func TestSidecarExecuteWithRecorder(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"test", "--non-interactive"}

	src := []byte(`---
title: Triage
---

## Section {#intro}

Some prose explaining the demo.

## Pick {#pick}

> blockquote note

` + "```inputs" + `
- name: kind
  type: choice
  options: [a, b]
  default: a
` + "```" + `
`)

	rec := &MemoryRecorder{}
	d := New("placeholder").
		FromMarkdownBytes(src).
		WithRenderer(&recordingRenderer{}).
		WithRecorder(rec)

	var seen string
	d.Bind("pick").Run(func(ctx StepContext) *StepResult {
		seen = ctx.Inputs["kind"].(string)
		return nil
	})

	d.Execute()

	if seen != "a" {
		t.Errorf("Bind+Run didn't see md-declared default: %q", seen)
	}
	if len(rec.Entries) != 2 {
		t.Fatalf("expected 2 trace entries (section + step), got %d", len(rec.Entries))
	}
	if rec.Entries[0].Kind != KindSection || rec.Entries[0].Title != "Section" {
		t.Errorf("trace[0] = %+v, want section", rec.Entries[0])
	}
	if rec.Entries[1].Kind != KindStep || rec.Entries[1].StepID != "pick" {
		t.Errorf("trace[1] = %+v, want step pick", rec.Entries[1])
	}
	if rec.Entries[1].Inputs["kind"] != "a" {
		t.Errorf("trace[1].Inputs.kind = %v, want \"a\"", rec.Entries[1].Inputs["kind"])
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}
