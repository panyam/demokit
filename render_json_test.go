package demokit

import (
	"encoding/json"
	"strings"
	"testing"
)

// fixtureDemoForJSON builds a demo exercising every projected field
// (actors, step with note/arrows/refs/inputs, section).
func fixtureDemoForJSON() (*Demo, []TraceEntry) {
	d := New("JSON Demo").
		Description("for JSON shape tests").
		Actors(Actor("App", "Application"), Actor("AS", "Auth Server"))
	d.Step("Pick a symptom").ID("triage").
		Note("most failures fall into a handful of buckets").
		Ref(Ref{Name: "RFC 6749", URL: "https://rfc.example/6749"}).
		Input(Choice("expired", "scope").Named("symptom", "Symptom").WithDefault("expired"))
	d.Step("Recover").ID("recover").
		Arrow("App", "AS", "POST /token (refresh)").
		DashedArrow("AS", "App", "{access_token}")
	d.Section("How it works", "you'll be asked to pick a failure")

	trace := []TraceEntry{
		{Kind: KindStep, Title: "Pick a symptom", StepID: "triage", Visit: 1,
			Inputs: map[string]any{"symptom": "expired"}, Next: "recover"},
		{Kind: KindStep, Title: "Recover", StepID: "recover", Visit: 1},
	}
	return d, trace
}

// TestRenderDocumentJSONStaticShape verifies the static JSON envelope
// has the expected demo block, no trace key, lowercase tags on Actors
// and Refs, and projected step fields. Embed hosts depend on this exact
// shape — drift here breaks every consumer.
func TestRenderDocumentJSONStaticShape(t *testing.T) {
	d, _ := fixtureDemoForJSON()

	out := RenderDocumentJSON(RenderContext{Demo: d})

	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("RenderDocumentJSON produced invalid JSON: %v\n%s", err, out)
	}

	if _, ok := parsed["trace"]; ok {
		t.Errorf("static JSON should omit \"trace\" key (embed hosts use it as a mode discriminator)")
	}

	demo, ok := parsed["demo"].(map[string]any)
	if !ok {
		t.Fatalf("missing \"demo\" object in static JSON: %s", out)
	}
	if demo["title"] != "JSON Demo" {
		t.Errorf("demo.title = %v, want %q", demo["title"], "JSON Demo")
	}
	if demo["description"] != "for JSON shape tests" {
		t.Errorf("demo.description wrong: %v", demo["description"])
	}

	actors, ok := demo["actors"].([]any)
	if !ok || len(actors) != 2 {
		t.Fatalf("demo.actors not a 2-element list: %v", demo["actors"])
	}
	a0 := actors[0].(map[string]any)
	if a0["id"] != "App" || a0["label"] != "Application" {
		t.Errorf("actor[0] = %v; expected lowercase id/label tags", a0)
	}
	if _, hasUpper := a0["ID"]; hasUpper {
		t.Errorf("actor[0] should not expose uppercase \"ID\" (json tags must be lowercase)")
	}

	items, ok := demo["items"].([]any)
	if !ok || len(items) != 3 {
		t.Fatalf("demo.items not a 3-element list: %v", demo["items"])
	}
	step0 := items[0].(map[string]any)
	if step0["kind"] != "step" || step0["id"] != "triage" {
		t.Errorf("items[0] wrong: %v", step0)
	}
	if step0["note"] == nil {
		t.Errorf("items[0].note missing")
	}

	refs, ok := step0["refs"].([]any)
	if !ok || len(refs) != 1 {
		t.Fatalf("items[0].refs not a 1-element list: %v", step0["refs"])
	}
	r0 := refs[0].(map[string]any)
	if r0["name"] != "RFC 6749" || r0["url"] != "https://rfc.example/6749" {
		t.Errorf("ref[0] = %v; expected lowercase name/url tags", r0)
	}

	step1 := items[1].(map[string]any)
	arrows, ok := step1["arrows"].([]any)
	if !ok || len(arrows) != 2 {
		t.Fatalf("items[1].arrows not a 2-element list: %v", step1["arrows"])
	}
	dashed := arrows[1].(map[string]any)["dashed"]
	if dashed != true {
		t.Errorf("arrow[1].dashed = %v, want true", dashed)
	}

	sec := items[2].(map[string]any)
	if sec["kind"] != "section" || sec["title"] != "How it works" {
		t.Errorf("items[2] wrong: %v", sec)
	}
}

// TestRenderDocumentJSONInputsOmitParseFunc verifies a step's declared
// inputs serialize via the view shim — Name/Prompt/Default — and never
// surface the Parse closure. A bare json.Marshal of InputDef would
// panic the encoder; this is the regression guard for that.
func TestRenderDocumentJSONInputsOmitParseFunc(t *testing.T) {
	d, _ := fixtureDemoForJSON()

	out := RenderDocumentJSON(RenderContext{Demo: d})

	if strings.Contains(out, "\"Parse\"") || strings.Contains(out, "\"parse\"") {
		t.Errorf("JSON output should not expose the Parse closure: %s", out)
	}

	var parsed map[string]any
	_ = json.Unmarshal([]byte(out), &parsed)
	items := parsed["demo"].(map[string]any)["items"].([]any)
	step0 := items[0].(map[string]any)
	inputs, ok := step0["inputs"].([]any)
	if !ok || len(inputs) != 1 {
		t.Fatalf("step0.inputs not a 1-element list: %v", step0["inputs"])
	}
	in := inputs[0].(map[string]any)
	if in["name"] != "symptom" || in["prompt"] != "Symptom" || in["default"] != "expired" {
		t.Errorf("inputs[0] = %v; expected projected name/prompt/default", in)
	}
}

// TestRenderDocumentJSONTraceRoundTrip verifies the trace section
// survives the RenderDocumentJSON → json.Unmarshal round-trip with
// step IDs and inputs intact. Embed hosts that re-parse the JSON output
// depend on this contract.
func TestRenderDocumentJSONTraceRoundTrip(t *testing.T) {
	d, trace := fixtureDemoForJSON()

	out := RenderDocumentJSON(RenderContext{Demo: d, Trace: trace})

	var parsed struct {
		Trace []TraceEntry `json:"trace"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("trace round-trip failed: %v", err)
	}
	if len(parsed.Trace) != 2 {
		t.Fatalf("trace length = %d, want 2", len(parsed.Trace))
	}
	if parsed.Trace[0].StepID != "triage" || parsed.Trace[0].Next != "recover" {
		t.Errorf("trace[0] wrong: %+v", parsed.Trace[0])
	}
	// JSON unmarshal widens numbers to float64 — assert input value
	// survives, ignoring numeric-type drift.
	if parsed.Trace[0].Inputs["symptom"] != "expired" {
		t.Errorf("trace[0] inputs.symptom = %v, want \"expired\"", parsed.Trace[0].Inputs["symptom"])
	}
}

// TestJSONFromTraceWrapperEquivalence verifies the legacy-style
// JSONFromTrace wrapper is byte-identical to a direct RenderDocumentJSON
// call. Pins the wrapper as a no-op shim, mirroring the markdown/html
// wrapper-equivalence tests.
func TestJSONFromTraceWrapperEquivalence(t *testing.T) {
	d, trace := fixtureDemoForJSON()

	viaWrapper := JSONFromTrace(d, trace)
	viaContext := RenderDocumentJSON(RenderContext{Demo: d, Trace: trace})

	if viaWrapper != viaContext {
		t.Errorf("wrapper output differs from RenderContext output\n--- wrapper ---\n%s\n--- context ---\n%s",
			viaWrapper, viaContext)
	}
}

// TestRenderDocumentJSONVerbatimProjection verifies a step's verbatim
// blocks project into the JSON envelope under "verbatim" with a
// "variants" array per block. Single-snippet blocks produce a
// one-element variants array — embed hosts read the same shape
// unconditionally regardless of whether the author used Verbatim()
// or VerbatimVariants().
func TestRenderDocumentJSONVerbatimProjection(t *testing.T) {
	d := New("V").Description("d")
	d.Step("only").ID("only").
		VerbatimLang("the wire", "bash", `curl -s http://x`).
		Verbatim("", "no-label content")

	out := RenderDocumentJSON(RenderContext{Demo: d})

	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	items := parsed["demo"].(map[string]any)["items"].([]any)
	step0 := items[0].(map[string]any)
	verb, ok := step0["verbatim"].([]any)
	if !ok || len(verb) != 2 {
		t.Fatalf("verbatim not a 2-element list: %v", step0["verbatim"])
	}

	v0 := verb[0].(map[string]any)
	if v0["label"] != "the wire" {
		t.Errorf("verbatim[0].label = %v, want %q", v0["label"], "the wire")
	}
	vars0 := v0["variants"].([]any)
	if len(vars0) != 1 {
		t.Fatalf("verbatim[0].variants len = %d, want 1", len(vars0))
	}
	va0 := vars0[0].(map[string]any)
	if va0["lang"] != "bash" || va0["content"] != "curl -s http://x" {
		t.Errorf("verbatim[0].variants[0] = %v", va0)
	}
	if _, hasLabel := va0["label"]; hasLabel {
		t.Errorf("variants[0].label should be omitted (omitempty) for single-variant block, got %v", va0)
	}

	v1 := verb[1].(map[string]any)
	if _, hasLabel := v1["label"]; hasLabel {
		t.Errorf("verbatim[1] should omit empty outer label, got %v", v1)
	}
	vars1 := v1["variants"].([]any)
	if len(vars1) != 1 {
		t.Fatalf("verbatim[1].variants len = %d, want 1", len(vars1))
	}
	va1 := vars1[0].(map[string]any)
	if _, hasLang := va1["lang"]; hasLang {
		t.Errorf("verbatim[1].variants[0] should omit empty lang, got %v", va1)
	}
	if va1["content"] != "no-label content" {
		t.Errorf("verbatim[1].variants[0].content = %v", va1["content"])
	}
}

// TestDemoJSONStaticEntryPoint verifies Demo.JSON() — the static-mode
// counterpart to Demo.Markdown() — produces the same envelope as
// RenderDocumentJSON with no trace.
func TestDemoJSONStaticEntryPoint(t *testing.T) {
	d, _ := fixtureDemoForJSON()

	got := d.JSON()
	want := RenderDocumentJSON(RenderContext{Demo: d})

	if got != want {
		t.Errorf("Demo.JSON() differs from RenderDocumentJSON(static)\n--- got ---\n%s\n--- want ---\n%s",
			got, want)
	}
}
