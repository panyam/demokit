package demokit

import (
	"reflect"
	"strings"
	"testing"
)

// loadOneStep loads a sidecar body (no frontmatter) and returns the
// single step it must contain, failing otherwise.
func loadOneStep(t *testing.T, md string) *StepDef {
	t.Helper()
	d := New("").FromMarkdownBytes([]byte(md))
	if d.loadError != nil {
		t.Fatalf("loadError: %v", d.loadError)
	}
	if len(d.items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(d.items))
	}
	step, ok := d.items[0].(*StepDef)
	if !ok {
		t.Fatalf("items[0] = %T, want *StepDef", d.items[0])
	}
	return step
}

// TestSidecarVerbatimSingleNoLang: a lone verbatim fence with no language
// and no label is the markdown equivalent of VerbatimLang(title, "", body)
// — one block, one unlabeled variant.
func TestSidecarVerbatimSingleNoLang(t *testing.T) {
	step := loadOneStep(t, "## Step {#s}\n\n"+
		"~~~ {verbatim=\"Just text\"}\nhello world\n~~~\n")

	if len(step.verbatim) != 1 {
		t.Fatalf("verbatim blocks = %d, want 1", len(step.verbatim))
	}
	b := step.verbatim[0]
	if b.Label != "Just text" {
		t.Errorf("block label = %q, want %q", b.Label, "Just text")
	}
	if len(b.Variants) != 1 {
		t.Fatalf("variants = %d, want 1", len(b.Variants))
	}
	v := b.Variants[0]
	if v.Label != "" || v.Lang != "" || v.Content != "hello world" || v.IsDefault {
		t.Errorf("variant = %+v", v)
	}
}

// TestSidecarVerbatimSingleWithLangAndLabel: a single fence carrying a
// language and a label becomes a one-variant block with that sub-label.
func TestSidecarVerbatimSingleWithLangAndLabel(t *testing.T) {
	step := loadOneStep(t, "## Step {#s}\n\n"+
		"~~~json {verbatim=\"Server response\" label=body}\n{\"ok\":true}\n~~~\n")

	if len(step.verbatim) != 1 || len(step.verbatim[0].Variants) != 1 {
		t.Fatalf("verbatim shape = %+v", step.verbatim)
	}
	v := step.verbatim[0].Variants[0]
	if v.Label != "body" || v.Lang != "json" || v.Content != `{"ok":true}` {
		t.Errorf("variant = %+v", v)
	}
}

// TestSidecarVerbatimMultiVariant: three consecutive fences sharing one
// verbatim title merge into a single block with three variants in source
// order, mirroring VerbatimVariants.
func TestSidecarVerbatimMultiVariant(t *testing.T) {
	step := loadOneStep(t, "## Step {#s}\n\n"+
		"~~~bash {verbatim=\"Reproduce\" label=curl default=true}\ncurl x\n~~~\n\n"+
		"~~~python {verbatim=\"Reproduce\" label=python}\nrequests.get()\n~~~\n\n"+
		"~~~go {verbatim=\"Reproduce\" label=go}\nhttp.Get()\n~~~\n")

	if len(step.verbatim) != 1 {
		t.Fatalf("verbatim blocks = %d, want 1 (expected merge)", len(step.verbatim))
	}
	b := step.verbatim[0]
	if b.Label != "Reproduce" {
		t.Errorf("block label = %q", b.Label)
	}
	gotLabels := []string{}
	for _, v := range b.Variants {
		gotLabels = append(gotLabels, v.Label)
	}
	if !equalStrings(gotLabels, []string{"curl", "python", "go"}) {
		t.Errorf("variant order = %v, want [curl python go]", gotLabels)
	}
	if b.Variants[0].Lang != "bash" || b.Variants[1].Lang != "python" || b.Variants[2].Lang != "go" {
		t.Errorf("langs = %q/%q/%q", b.Variants[0].Lang, b.Variants[1].Lang, b.Variants[2].Lang)
	}
}

// TestSidecarVerbatimDefaultFlag: default=true marks the right variant and
// only that one.
func TestSidecarVerbatimDefaultFlag(t *testing.T) {
	step := loadOneStep(t, "## Step {#s}\n\n"+
		"~~~bash {verbatim=\"R\" label=a}\na\n~~~\n\n"+
		"~~~bash {verbatim=\"R\" label=b default=true}\nb\n~~~\n")

	vs := step.verbatim[0].Variants
	if len(vs) != 2 {
		t.Fatalf("variants = %d, want 2", len(vs))
	}
	if vs[0].IsDefault {
		t.Errorf("variant a should not be default")
	}
	if !vs[1].IsDefault {
		t.Errorf("variant b should be default")
	}
}

// TestSidecarVerbatimOnlyHeadingIsStep: a heading whose only content is a
// verbatim block classifies as a step, not a prose section.
func TestSidecarVerbatimOnlyHeadingIsStep(t *testing.T) {
	d := New("").FromMarkdownBytes([]byte("## Snippet {#snip}\n\n" +
		"~~~bash {verbatim=\"Run it\"}\nmake\n~~~\n"))
	if d.loadError != nil {
		t.Fatalf("loadError: %v", d.loadError)
	}
	if _, ok := d.items[0].(*StepDef); !ok {
		t.Fatalf("verbatim-only heading classified as %T, want *StepDef", d.items[0])
	}
}

// TestSidecarVerbatimNestedFence: a triple-backtick block inside a
// tilde-fenced verbatim body is preserved character-exact.
func TestSidecarVerbatimNestedFence(t *testing.T) {
	body := "Here is code:\n```go\nx := 1\n```\ndone"
	step := loadOneStep(t, "## Step {#s}\n\n"+
		"~~~markdown {verbatim=\"Doc\"}\n"+body+"\n~~~\n")

	got := step.verbatim[0].Variants[0].Content
	if got != body {
		t.Errorf("nested-fence content not preserved:\n got: %q\nwant: %q", got, body)
	}
}

// TestSidecarVerbatimPassthroughUnaffected: a plain fenced block with no
// verbatim attribute still passes through to the section/step body and
// does not become a verbatim block (existing behavior unbroken).
func TestSidecarVerbatimPassthroughUnaffected(t *testing.T) {
	d := New("").FromMarkdownBytes([]byte("## Notes {#n}\n\nsome prose\n\n```bash\nls -la\n```\n"))
	if d.loadError != nil {
		t.Fatalf("loadError: %v", d.loadError)
	}
	sec, ok := d.items[0].(*SectionDef)
	if !ok {
		t.Fatalf("items[0] = %T, want *SectionDef (no verbatim attr → not a step)", d.items[0])
	}
	if wantSub := "ls -la"; !strings.Contains(sec.body, wantSub) {
		t.Errorf("plain fence lost from body: %q", sec.body)
	}
}

// TestFenceVerbatimAttrs unit-tests the attribute reader directly,
// including the cases goldmark's parser accepts and rejects.
func TestFenceVerbatimAttrs(t *testing.T) {
	cases := []struct {
		name                  string
		info                  string
		wantTitle, wantLabel  string
		wantVerbatim, wantDef bool
	}{
		{"full", `bash {verbatim="Reproduce on the wire" label=curl default=true}`, "Reproduce on the wire", "curl", true, true},
		{"title only", `bash {verbatim="X"}`, "X", "", true, false},
		{"no braces", `bash`, "", "", false, false},
		{"not verbatim", `bash {label=curl}`, "", "", false, false},
		{"bare default rejected", `bash {verbatim="X" default}`, "", "", false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			title, label, isVerb, isDef := fenceVerbatimAttrs(c.info)
			if title != c.wantTitle || label != c.wantLabel || isVerb != c.wantVerbatim || isDef != c.wantDef {
				t.Errorf("got (%q,%q,%v,%v) want (%q,%q,%v,%v)",
					title, label, isVerb, isDef, c.wantTitle, c.wantLabel, c.wantVerbatim, c.wantDef)
			}
		})
	}
}

// TestSidecarVerbatimRoundTripMatchesGo: a multi-variant block authored in
// markdown produces the exact same stored structure as the equivalent
// VerbatimVariants Go call — the load path and the builder path converge
// on one StepDef shape.
func TestSidecarVerbatimRoundTripMatchesGo(t *testing.T) {
	md := "## Step {#s}\n\n" +
		"~~~bash {verbatim=\"Reproduce\" label=curl default=true}\ncurl x\n~~~\n\n" +
		"~~~go {verbatim=\"Reproduce\" label=go}\nhttp.Get()\n~~~\n"
	loaded := loadOneStep(t, md)

	exp := &StepDef{}
	exp.VerbatimVariants("Reproduce",
		MakeVariant("curl", "bash", "curl x").Default(),
		MakeVariant("go", "go", "http.Get()"),
	)

	if !reflect.DeepEqual(loaded.verbatim, exp.verbatim) {
		t.Errorf("round-trip mismatch:\n loaded: %+v\n    go: %+v", loaded.verbatim, exp.verbatim)
	}
}
