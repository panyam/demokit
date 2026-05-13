package tui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/panyam/demokit"
)

// stepWithVariants builds a minimal *demokit.StepDef carrying a single
// multi-variant verbatim block, hooked up to a demo so copyableBlocks
// can see the parent for IsBoxedVerbatim. Used by the parser tests.
func stepWithVariants(t *testing.T, boxed bool) *demokit.StepDef {
	t.Helper()
	d := demokit.New("t")
	if boxed {
		d.BoxedVerbatim()
	}
	return d.Step("S").ID("s").VerbatimVariants("Fetch",
		demokit.MakeVariant("curl", "bash", "curl -X GET ...").Default(),
		demokit.MakeVariant("python", "python", "requests.get(...)"),
	)
}

// stepWithSingleVariant builds a step with a one-snippet block. Used
// to assert single-variant blocks are copyable only when the demo
// opts in via BoxedVerbatim.
func stepWithSingleVariant(t *testing.T, boxed bool) *demokit.StepDef {
	t.Helper()
	d := demokit.New("t")
	if boxed {
		d.BoxedVerbatim()
	}
	return d.Step("S").ID("s").Shell("echo hi")
}

func TestCopyableBlocksMultiVariantAutoBoxed(t *testing.T) {
	// Multi-variant is copyable regardless of the demo flag — tabs need
	// a frame, so the box (and therefore copy affordance) is always on.
	step := stepWithVariants(t, false)
	r := New()
	got := r.copyableBlocks(step)
	if len(got) != 1 {
		t.Fatalf("multi-variant should be copyable without BoxedVerbatim; got %d copyable blocks", len(got))
	}
}

func TestCopyableBlocksSingleVariantHonorsFlag(t *testing.T) {
	r := New()
	t.Run("flag unset → not copyable (today's mouse-select behavior)", func(t *testing.T) {
		got := r.copyableBlocks(stepWithSingleVariant(t, false))
		if len(got) != 0 {
			t.Errorf("single-variant without BoxedVerbatim should not be copyable, got %d", len(got))
		}
	})
	t.Run("flag set → copyable", func(t *testing.T) {
		got := r.copyableBlocks(stepWithSingleVariant(t, true))
		if len(got) != 1 {
			t.Errorf("single-variant with BoxedVerbatim should be copyable, got %d", len(got))
		}
	})
}

func TestCopyableBlocksNilStep(t *testing.T) {
	if got := New().copyableBlocks(nil); got != nil {
		t.Errorf("nil step should return nil, got %v", got)
	}
}

func TestHandleCopyCommandBareC(t *testing.T) {
	// Route Copy() through a buffer so the OSC 52 path succeeds
	// deterministically and the test doesn't depend on the host
	// terminal supporting clipboard escapes.
	var buf bytes.Buffer
	demokit.SetClipboardWriter(&buf)
	demokit.EnableShellClipboardFallback(false)
	defer func() {
		demokit.SetClipboardWriter(nil)
		demokit.EnableShellClipboardFallback(true)
	}()

	r := New()
	step := stepWithVariants(t, false)
	copyables := r.copyableBlocks(step)

	msg := r.handleCopyCommand("c", copyables)
	if !strings.Contains(msg, "copied") || !strings.Contains(msg, "osc52") {
		t.Errorf("bare `c` should report copy success, got %q", msg)
	}
	// Default-marked variant is curl — the OSC 52 sequence must encode
	// that exact content. base64("curl -X GET ...") is the payload.
	if !strings.Contains(buf.String(), "Y3VybCAtWCBHRVQgLi4u") {
		t.Errorf("bare `c` should have copied curl (default); buf=%q", buf.String())
	}
}

func TestHandleCopyCommandNamedVariant(t *testing.T) {
	var buf bytes.Buffer
	demokit.SetClipboardWriter(&buf)
	demokit.EnableShellClipboardFallback(false)
	defer func() {
		demokit.SetClipboardWriter(nil)
		demokit.EnableShellClipboardFallback(true)
	}()

	r := New()
	step := stepWithVariants(t, false)
	copyables := r.copyableBlocks(step)

	msg := r.handleCopyCommand("c python", copyables)
	if !strings.Contains(msg, "copied") {
		t.Errorf("`c python` should copy, got %q", msg)
	}
	// base64("requests.get(...)")
	if !strings.Contains(buf.String(), "cmVxdWVzdHMuZ2V0KC4uLik=") {
		t.Errorf("`c python` should have copied the python variant; buf=%q", buf.String())
	}
}

func TestHandleCopyCommandUnknownLabel(t *testing.T) {
	r := New()
	step := stepWithVariants(t, false)
	copyables := r.copyableBlocks(step)

	msg := r.handleCopyCommand("c ruby", copyables)
	if !strings.Contains(msg, "no variant labeled") {
		t.Errorf("unknown label should produce a clear error, got %q", msg)
	}
}

func TestHandleCopyCommandUnknownVerb(t *testing.T) {
	r := New()
	step := stepWithVariants(t, false)
	copyables := r.copyableBlocks(step)

	msg := r.handleCopyCommand("xyz", copyables)
	if !strings.Contains(msg, "unknown command") {
		t.Errorf("unknown verb should produce a clear error, got %q", msg)
	}
}

func TestCopyPromptHintAdaptsToShape(t *testing.T) {
	t.Run("no copyables → plain Enter prompt", func(t *testing.T) {
		got := copyPromptHint(nil)
		if !strings.Contains(got, "Press Enter") || strings.Contains(got, "copy") {
			t.Errorf("expected plain Enter prompt without copy hint, got %q", got)
		}
	})
	t.Run("single-variant copyable → simple `c` hint, no <label> form", func(t *testing.T) {
		r := New()
		copyables := r.copyableBlocks(stepWithSingleVariant(t, true))
		got := copyPromptHint(copyables)
		if !strings.Contains(got, "type `c` to copy") || strings.Contains(got, "<label>") {
			t.Errorf("single-variant hint should not mention <label>, got %q", got)
		}
	})
	t.Run("multi-variant copyable → exposes `c <label>` form", func(t *testing.T) {
		r := New()
		copyables := r.copyableBlocks(stepWithVariants(t, false))
		got := copyPromptHint(copyables)
		if !strings.Contains(got, "<label>") {
			t.Errorf("multi-variant hint should mention <label>, got %q", got)
		}
	})
}
