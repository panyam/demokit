package demokit

import (
	"strings"
	"testing"
)

// TestSidecarRoundTrip builds an inline demo exercising every content kind,
// emits sidecar markdown, loads it back, and checks the content survives —
// the load-bearing guarantee that Sidecar is the inverse of FromMarkdown.
func TestSidecarRoundTrip(t *testing.T) {
	orig := New("Round Trip").
		Description("exercise every content kind").
		Actors(Actor("A", "Alpha"), Actor("B", "Beta"))
	orig.Section("Intro", "first line", "second line")
	orig.Step("Connect").ID("connect").
		Note("a note", "second paragraph").
		Arrow("A", "B", "hello").
		DashedArrow("B", "A", "ack").
		Input(Choice("x", "y").Named("pick", "Pick one").WithDefault("x")).
		VerbatimVariants("Reproduce",
			MakeVariant("curl", "bash", "curl x").Default(),
			MakeVariant("go", "go", "http.Get()"))

	md := orig.Sidecar()

	loaded := New("placeholder").FromMarkdownBytes([]byte(md))
	if loaded.loadError != nil {
		t.Fatalf("emitted sidecar did not load: %v\n---\n%s", loaded.loadError, md)
	}

	if loaded.title != "Round Trip" || loaded.description != "exercise every content kind" {
		t.Errorf("frontmatter lost: title=%q desc=%q", loaded.title, loaded.description)
	}
	if len(loaded.actors) != 2 || loaded.actors[1].Label != "Beta" {
		t.Errorf("actors lost: %+v", loaded.actors)
	}
	if len(loaded.items) != 2 {
		t.Fatalf("items = %d, want 2 (section + step)", len(loaded.items))
	}

	sec, ok := loaded.items[0].(*SectionDef)
	if !ok || !strings.Contains(sec.body, "second line") {
		t.Errorf("section round-trip lost body: %+v", loaded.items[0])
	}

	step, ok := loaded.items[1].(*StepDef)
	if !ok {
		t.Fatalf("items[1] = %T, want *StepDef", loaded.items[1])
	}
	if step.id != "connect" {
		t.Errorf("step id = %q, want connect", step.id)
	}
	if !strings.Contains(step.note, "a note") || !strings.Contains(step.note, "second paragraph") {
		t.Errorf("note lost: %q", step.note)
	}
	if len(step.arrows) != 2 || step.arrows[0].label != "hello" || !step.arrows[1].dashed {
		t.Errorf("arrows lost: %+v", step.arrows)
	}
	if len(step.inputs) != 1 || step.inputs[0].Name != "pick" || step.inputs[0].Kind != "choice" {
		t.Errorf("inputs lost: %+v", step.inputs)
	}
	if len(step.verbatim) != 1 || len(step.verbatim[0].Variants) != 2 {
		t.Fatalf("verbatim lost: %+v", step.verbatim)
	}
	if v := step.verbatim[0]; v.Label != "Reproduce" || !v.Variants[0].IsDefault || v.Variants[0].Content != "curl x" {
		t.Errorf("verbatim round-trip wrong: %+v", v)
	}
}

// TestSidecarDedupesIDs verifies steps without explicit ids get slugified,
// deduplicated ids.
func TestSidecarDedupesIDs(t *testing.T) {
	d := New("T")
	d.Step("Same Title")
	d.Step("Same Title")
	md := d.Sidecar()
	for _, want := range []string{"{#same-title}", "{#same-title-2}"} {
		if !strings.Contains(md, want) {
			t.Errorf("sidecar missing %q\n%s", want, md)
		}
	}
}
