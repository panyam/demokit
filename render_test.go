package demokit

import (
	"strings"
	"testing"
)

// fixtureDemoForRender builds a small demo with notes and refs so that
// per-entry and document renders have something to look up. Returns the
// demo plus a two-step trace covering both step and section entries.
func fixtureDemoForRender() (*Demo, []TraceEntry) {
	d := New("Branchy").Description("test render gen")
	d.Step("first").ID("a").
		Note("explanation of A").
		Ref(Ref{Name: "RFC X", URL: "https://rfc.example/X"})
	d.Step("second").ID("b")

	trace := []TraceEntry{
		{Kind: KindStep, Title: "first", StepID: "a", Visit: 1,
			Inputs: map[string]any{"name": "alice"},
			Output: "hello\nworld",
			Next:   "b"},
		{Kind: KindStep, Title: "second", StepID: "b", Visit: 1,
			Status: StatusInfo, Message: "fyi"},
	}
	return d, trace
}

// TestRenderEntryMDIsSelfContained verifies a per-entry markdown render
// is a fragment with no document-level chrome: it must not include the
// "## Walkthrough" header, the deduplicated "## References" section, or
// the title preamble. Layering enforcement — incremental renderers (WS
// embeds) depend on per-entry being a self-contained piece.
func TestRenderEntryMDIsSelfContained(t *testing.T) {
	d, trace := fixtureDemoForRender()
	ctx := RenderContext{Demo: d, Trace: trace}

	out := RenderEntryMD(ctx, trace[0], EntryOpts{StepNumber: 1})

	mustNotContain := []string{
		"# Branchy",
		"## Walkthrough",
		"## References",
	}
	for _, s := range mustNotContain {
		if strings.Contains(out, s) {
			t.Errorf("per-entry render should not contain %q, got:\n%s", s, out)
		}
	}

	// Step content must still be present (heading, note, inline refs,
	// inputs, output, jump arrow).
	mustContain := []string{
		"### 1. first",
		"explanation of A",
		"RFC X",
		"`name` = `alice`",
		"hello\nworld",
		"jumped to `b`",
	}
	for _, s := range mustContain {
		if !strings.Contains(out, s) {
			t.Errorf("per-entry render missing %q, got:\n%s", s, out)
		}
	}
}

// TestRenderEntryMDStepNumberIsHonored verifies that EntryOpts.StepNumber
// controls the heading prefix. Caller-supplied numbering is the contract
// for incremental render — without it, WS embeds couldn't display the
// correct absolute step index when pushing one fragment at a time.
func TestRenderEntryMDStepNumberIsHonored(t *testing.T) {
	d, trace := fixtureDemoForRender()
	ctx := RenderContext{Demo: d, Trace: trace}

	out42 := RenderEntryMD(ctx, trace[0], EntryOpts{StepNumber: 42})
	if !strings.Contains(out42, "### 42. first") {
		t.Errorf("StepNumber=42 not honored, got:\n%s", out42)
	}

	out1 := RenderEntryMD(ctx, trace[0], EntryOpts{StepNumber: 1})
	if !strings.Contains(out1, "### 1. first") {
		t.Errorf("StepNumber=1 not honored, got:\n%s", out1)
	}

	// Zero is the documented "no number" sentinel — heading falls back
	// to bare title without the "N. " prefix.
	out0 := RenderEntryMD(ctx, trace[0], EntryOpts{StepNumber: 0})
	if !strings.Contains(out0, "### first") {
		t.Errorf("StepNumber=0 should produce bare title heading, got:\n%s", out0)
	}
	if strings.Contains(out0, "### 0. first") {
		t.Errorf("StepNumber=0 should not render the literal 0, got:\n%s", out0)
	}
}

// TestRenderDocumentMDComposition verifies the document render is exactly
// the concatenation of preamble + per-entry calls + refs footer. This
// locks the layering: any future formatter divergence between
// RenderDocumentMD and a hand-rolled composition would break this test
// and signal that the document path has acquired logic that the per-
// entry path can't reproduce.
func TestRenderDocumentMDComposition(t *testing.T) {
	d, trace := fixtureDemoForRender()
	ctx := RenderContext{Demo: d, Trace: trace}

	got := RenderDocumentMD(ctx)

	var want strings.Builder
	want.WriteString("# Branchy\n\n")
	want.WriteString("test render gen\n\n")
	want.WriteString("## Walkthrough\n\n")
	stepIdx := 0
	for _, e := range trace {
		opts := EntryOpts{}
		if e.Kind == KindStep {
			stepIdx++
			opts.StepNumber = stepIdx
		}
		want.WriteString(RenderEntryMD(ctx, e, opts))
	}
	want.WriteString("## References\n\n")
	want.WriteString("- [RFC X](https://rfc.example/X)\n\n")

	if got != want.String() {
		t.Errorf("document render diverges from composed entries\n--- got ---\n%s\n--- want ---\n%s",
			got, want.String())
	}
}

// TestRenderDocumentMDEmptyTrace verifies the empty-trace marker is
// preserved so that doc generation against an empty recording produces
// a recognizable stub rather than an empty file.
func TestRenderDocumentMDEmptyTrace(t *testing.T) {
	d := New("Empty")
	got := RenderDocumentMD(RenderContext{Demo: d, Trace: nil})
	if !strings.Contains(got, "_(empty trace)_") {
		t.Errorf("empty-trace marker missing:\n%s", got)
	}
}

// TestRenderEntryMDSection verifies section entries render heading and
// body without the step-numbering scheme. Sections must work in
// incremental render too, not only in the document loop.
func TestRenderEntryMDSection(t *testing.T) {
	ctx := RenderContext{}
	entry := TraceEntry{Kind: KindSection, Title: "Setup", Body: "do this first"}

	out := RenderEntryMD(ctx, entry, EntryOpts{StepNumber: 1})

	if !strings.Contains(out, "### Setup") {
		t.Errorf("section heading missing, got:\n%s", out)
	}
	if !strings.Contains(out, "do this first") {
		t.Errorf("section body missing, got:\n%s", out)
	}
	// Sections never get a numeric prefix — the StepNumber argument is
	// for steps only.
	if strings.Contains(out, "### 1. Setup") {
		t.Errorf("section should not be numbered, got:\n%s", out)
	}
}

// TestRenderEntryHTMLIsSelfContained verifies the HTML per-entry render
// emits no doctype, head, body, or closing tags. Mirror of the markdown
// layering check.
func TestRenderEntryHTMLIsSelfContained(t *testing.T) {
	d, trace := fixtureDemoForRender()
	ctx := RenderContext{Demo: d, Trace: trace}

	out := RenderEntryHTML(ctx, trace[0], EntryOpts{StepNumber: 1})

	mustNotContain := []string{
		"<!doctype",
		"<html",
		"<head",
		"<body",
		"</body",
		"</html",
		"<style",
	}
	for _, s := range mustNotContain {
		if strings.Contains(strings.ToLower(out), s) {
			t.Errorf("per-entry HTML should not contain %q, got:\n%s", s, out)
		}
	}

	// The fragment must still carry the step content.
	if !strings.Contains(out, "<h2>1. first") {
		t.Errorf("HTML per-entry missing numbered heading, got:\n%s", out)
	}
}

// TestRenderDocumentHTMLComposition verifies the HTML document is
// preamble + per-entry fragments + closing tags. Same layering
// guarantee as the markdown side.
func TestRenderDocumentHTMLComposition(t *testing.T) {
	d, trace := fixtureDemoForRender()
	ctx := RenderContext{Demo: d, Trace: trace}

	got := RenderDocumentHTML(ctx)

	// Locate the body opening — everything between it and </body> must
	// be exactly the concatenation of per-entry fragments.
	bodyStart := strings.Index(got, "<body>\n")
	if bodyStart < 0 {
		t.Fatalf("RenderDocumentHTML missing <body>: %s", got)
	}
	bodyContent := got[bodyStart+len("<body>\n"):]
	bodyEnd := strings.Index(bodyContent, "</body>")
	if bodyEnd < 0 {
		t.Fatalf("RenderDocumentHTML missing </body>: %s", got)
	}
	bodyContent = bodyContent[:bodyEnd]

	// Strip the h1 + description preamble that lives inside <body>.
	h1End := strings.Index(bodyContent, "</h1>\n")
	if h1End < 0 {
		t.Fatalf("missing </h1> in body content: %s", bodyContent)
	}
	afterH1 := bodyContent[h1End+len("</h1>\n"):]
	pEnd := strings.Index(afterH1, "</p>\n")
	if pEnd >= 0 {
		afterH1 = afterH1[pEnd+len("</p>\n"):]
	}

	var entries strings.Builder
	stepIdx := 0
	for _, e := range trace {
		opts := EntryOpts{}
		if e.Kind == KindStep {
			stepIdx++
			opts.StepNumber = stepIdx
		}
		entries.WriteString(RenderEntryHTML(ctx, e, opts))
	}

	if afterH1 != entries.String() {
		t.Errorf("HTML document body diverges from composed entries\n--- doc body ---\n%s\n--- composed ---\n%s",
			afterH1, entries.String())
	}
}
