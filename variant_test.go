package demokit

import (
	"strings"
	"testing"
)

func TestVariantMakeAndDefault(t *testing.T) {
	v := MakeVariant("curl", "bash", "curl -X GET http://x")
	if v.Label != "curl" || v.Lang != "bash" || v.Content != "curl -X GET http://x" {
		t.Fatalf("MakeVariant fields = %+v", v)
	}
	if v.IsDefault {
		t.Error("MakeVariant should leave IsDefault=false")
	}
	d := v.Default()
	if !d.IsDefault {
		t.Error(".Default() should set IsDefault=true")
	}
	// Default must not mutate the receiver — Variant is a value type.
	if v.IsDefault {
		t.Error("Variant.Default mutated the receiver — must be value-semantic")
	}
}

func TestSingleVariantBackcompat(t *testing.T) {
	tests := []struct {
		name        string
		build       func() *StepDef
		wantLabel   string
		wantLang    string
		wantContent string
	}{
		{
			name:        "Verbatim",
			build:       func() *StepDef { return (&StepDef{}).Verbatim("hello", "world") },
			wantLabel:   "hello",
			wantContent: "world",
		},
		{
			name:        "VerbatimLang",
			build:       func() *StepDef { return (&StepDef{}).VerbatimLang("config", "yaml", "k: v") },
			wantLabel:   "config",
			wantLang:    "yaml",
			wantContent: "k: v",
		},
		{
			name:        "Shell",
			build:       func() *StepDef { return (&StepDef{}).Shell("echo hi") },
			wantLang:    "bash",
			wantContent: "echo hi",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			blocks := tc.build().VerbatimBlocks()
			if len(blocks) != 1 {
				t.Fatalf("want 1 block, got %d", len(blocks))
			}
			b := blocks[0]
			if b.Label != tc.wantLabel {
				t.Errorf("block label = %q, want %q", b.Label, tc.wantLabel)
			}
			if len(b.Variants) != 1 {
				t.Fatalf("single-variant constructor produced %d variants, want 1", len(b.Variants))
			}
			va := b.Variants[0]
			if va.Lang != tc.wantLang || va.Content != tc.wantContent {
				t.Errorf("variant = %+v, want lang=%q content=%q", va, tc.wantLang, tc.wantContent)
			}
		})
	}
}

func TestVerbatimVariantsConstructor(t *testing.T) {
	s := (&StepDef{}).VerbatimVariants("Fetch user",
		MakeVariant("curl", "bash", "curl -X GET ...").Default(),
		MakeVariant("python", "python", "requests.get(...)"),
		MakeVariant("go", "go", "http.Get(...)"),
	)
	blocks := s.VerbatimBlocks()
	if len(blocks) != 1 {
		t.Fatalf("want 1 block, got %d", len(blocks))
	}
	b := blocks[0]
	if b.Label != "Fetch user" {
		t.Errorf("block label = %q", b.Label)
	}
	if len(b.Variants) != 3 {
		t.Fatalf("variants len = %d, want 3", len(b.Variants))
	}
	if !b.Variants[0].IsDefault || b.Variants[1].IsDefault || b.Variants[2].IsDefault {
		t.Errorf("expected only the first variant to be Default, got %+v", b.Variants)
	}
	wantLabels := []string{"curl", "python", "go"}
	for i, va := range b.Variants {
		if va.Label != wantLabels[i] {
			t.Errorf("variants[%d].Label = %q, want %q", i, va.Label, wantLabels[i])
		}
	}
}

func TestBoxedVerbatimFlag(t *testing.T) {
	d := New("t")
	if d.IsBoxedVerbatim() {
		t.Error("IsBoxedVerbatim should default to false")
	}
	d.BoxedVerbatim()
	if !d.IsBoxedVerbatim() {
		t.Error("IsBoxedVerbatim should return true after BoxedVerbatim()")
	}
}

func TestStepCarriesDemoBackpointer(t *testing.T) {
	d := New("t")
	s := d.Step("foo")
	if s.Demo() != d {
		t.Errorf("Step.Demo() back-pointer not wired; got %p, want %p", s.Demo(), d)
	}
}

func TestVariantSelectionApply(t *testing.T) {
	vs := []Variant{
		{Label: "curl", IsDefault: true},
		{Label: "python"},
		{Label: "go"},
	}
	t.Run("Default with one marked → just that one", func(t *testing.T) {
		got := VariantSelectionDefault().Apply(vs)
		if len(got) != 1 || got[0].Label != "curl" {
			t.Errorf("default selection = %+v, want only curl", got)
		}
	})
	t.Run("Default with no marker → all variants (lossless)", func(t *testing.T) {
		all := []Variant{{Label: "a"}, {Label: "b"}}
		got := VariantSelectionDefault().Apply(all)
		if len(got) != 2 {
			t.Errorf("default with no marker should return all, got %+v", got)
		}
	})
	t.Run("All", func(t *testing.T) {
		got := VariantSelectionAll().Apply(vs)
		if len(got) != 3 {
			t.Errorf("All selection len = %d, want 3", len(got))
		}
	})
	t.Run("Named match", func(t *testing.T) {
		got := VariantSelectionNamed("python").Apply(vs)
		if len(got) != 1 || got[0].Label != "python" {
			t.Errorf("named=python = %+v", got)
		}
	})
	t.Run("Named is case-insensitive", func(t *testing.T) {
		got := VariantSelectionNamed("CURL").Apply(vs)
		if len(got) != 1 || got[0].Label != "curl" {
			t.Errorf("named=CURL should match curl, got %+v", got)
		}
	})
	t.Run("Named miss → empty (dropped from output)", func(t *testing.T) {
		got := VariantSelectionNamed("ruby").Apply(vs)
		if len(got) != 0 {
			t.Errorf("named=ruby should drop the block, got %+v", got)
		}
	})
}

func TestDemoVariantSelectionFromFlag(t *testing.T) {
	cases := map[string]string{
		"":        "default",
		"default": "default",
		"all":     "all",
		"curl":    "named",
	}
	for flag, kind := range cases {
		t.Run("flag="+flag, func(t *testing.T) {
			d := New("t")
			d.flagVariant = flag
			sel := d.VariantSelection()
			// Indirect check: feed a known variant set and inspect the
			// output shape.
			vs := []Variant{
				{Label: "curl", IsDefault: true},
				{Label: "python"},
			}
			out := sel.Apply(vs)
			switch kind {
			case "default":
				if len(out) != 1 || out[0].Label != "curl" {
					t.Errorf("default behavior wrong: %+v", out)
				}
			case "all":
				if len(out) != 2 {
					t.Errorf("all behavior wrong: %+v", out)
				}
			case "named":
				if len(out) != 1 || out[0].Label != "curl" {
					t.Errorf("named=curl wrong: %+v", out)
				}
			}
		})
	}
}

func TestMarkdownEmitsAllVariantsByDefault(t *testing.T) {
	d := New("V").Description("d")
	d.Step("only").ID("only").
		VerbatimVariants("Fetch",
			MakeVariant("curl", "bash", "curl -X GET ..."),
			MakeVariant("python", "python", "requests.get(...)"),
		)

	out := d.Markdown()
	mustContain := []string{
		"#### Fetch",
		"**curl**",
		"**python**",
		"curl -X GET ...",
		"requests.get(...)",
	}
	for _, s := range mustContain {
		if !strings.Contains(out, s) {
			t.Errorf("markdown missing %q\n--- output ---\n%s", s, out)
		}
	}
}

func TestMarkdownFiltersByVariantFlag(t *testing.T) {
	d := New("V").Description("d")
	d.flagVariant = "python"
	d.Step("only").ID("only").
		VerbatimVariants("Fetch",
			MakeVariant("curl", "bash", "curl -X GET ..."),
			MakeVariant("python", "python", "requests.get(...)"),
		)

	out := d.Markdown()
	if strings.Contains(out, "curl -X GET") {
		t.Errorf("--variant=python should drop the curl variant; got:\n%s", out)
	}
	if !strings.Contains(out, "requests.get(...)") {
		t.Errorf("--variant=python should keep python; got:\n%s", out)
	}
}

func TestMarkdownDefaultMarkerPicksOne(t *testing.T) {
	d := New("V").Description("d")
	d.Step("only").ID("only").
		VerbatimVariants("Fetch",
			MakeVariant("curl", "bash", "curl -X GET ..."),
			MakeVariant("python", "python", "requests.get(...)").Default(),
		)

	out := d.Markdown()
	if strings.Contains(out, "curl -X GET") {
		t.Errorf("default-marked python should suppress the others; curl still present:\n%s", out)
	}
	if !strings.Contains(out, "requests.get(...)") {
		t.Errorf("default-marked python missing from output:\n%s", out)
	}
}
