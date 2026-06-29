package main

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/panyam/demokit"
)

const fixtureWalkthrough = `package main

import (
	"fmt"
	"github.com/panyam/demokit"
)

func main() {
	demo := demokit.New("Fixture Demo").
		Description("A test demo").
		Actors(demokit.Actor("A", "Alpha"), demokit.Actor("B", "Beta"))

	demo.Section("Intro", "first line", "second line")

	demo.Step("Connect").ID("connect").
		Note("a note", "line two").
		Arrow("A", "B", "hello").
		DashedArrow("B", "A", "ack").
		VerbatimVariants("Reproduce",
			demokit.MakeVariant("curl", "bash", "curl x").Default(),
			demokit.MakeVariant("go", "go", "http.Get()")).
		Run(func(ctx demokit.StepContext) *demokit.StepResult {
			fmt.Println("hi from connect")
			return nil
		})

	demo.Step("Same Title").
		Run(func(ctx demokit.StepContext) *demokit.StepResult { return nil })

	demo.Step("Same Title").
		Run(func(ctx demokit.StepContext) *demokit.StepResult { return nil })

	demo.Step("Has input").ID("inp").
		Input(demokit.Choice("x", "y").Named("k", "K")).
		Run(func(ctx demokit.StepContext) *demokit.StepResult { return nil })

	demo.Execute()
}
`

func extractFixture(t *testing.T) *extractResult {
	t.Helper()
	res, err := extractFile([]byte(fixtureWalkthrough))
	if err != nil {
		t.Fatalf("extractFile: %v", err)
	}
	return res
}

// TestExtractContentToMarkdown checks that frontmatter, a section, notes,
// mermaid arrows, and a multi-variant verbatim block all land in demo.md in
// the expected sidecar form.
func TestExtractContentToMarkdown(t *testing.T) {
	md := extractFixture(t).DemoMD
	wants := []string{
		"title: Fixture Demo",
		"description: A test demo",
		"- { id: A, label: Alpha }",
		"## Intro {#intro}",
		"first line\nsecond line",
		"## Connect {#connect}",
		"> a note\n> line two",
		"A ->> B: hello",
		"B -->> A: ack",
		`~~~bash {verbatim="Reproduce" label=curl default=true}`,
		"curl x",
		`~~~go {verbatim="Reproduce" label=go}`,
		"http.Get()",
	}
	for _, w := range wants {
		if !strings.Contains(md, w) {
			t.Errorf("demo.md missing %q\n---\n%s", w, md)
		}
	}
}

// TestExtractUniqueIDs verifies colliding slugs are disambiguated in source
// order and an explicit ID is preserved.
func TestExtractUniqueIDs(t *testing.T) {
	md := extractFixture(t).DemoMD
	for _, want := range []string{"## Connect {#connect}", "## Same Title {#same-title}", "## Same Title {#same-title-2}", "## Has input {#inp}"} {
		if !strings.Contains(md, want) {
			t.Errorf("demo.md missing heading/id %q", want)
		}
	}
}

// TestExtractInputBecomesTodo verifies a step with Input(...) yields a warning
// and a TODO marker rather than a silent drop.
func TestExtractInputBecomesTodo(t *testing.T) {
	res := extractFixture(t)
	if !strings.Contains(res.DemoMD, "TODO(extract)") {
		t.Errorf("expected a TODO marker in demo.md for the Input step")
	}
	var found bool
	for _, w := range res.Warnings {
		if strings.Contains(w, "Input(") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected an Input warning, got %v", res.Warnings)
	}
}

// TestExtractDemoMDRoundTrips is the load-bearing check: the generated demo.md
// must parse through demokit's own loader without error.
func TestExtractDemoMDRoundTrips(t *testing.T) {
	res := extractFixture(t)
	// Loading the generated md and re-rendering proves it parsed into items
	// (a load error would leave the demo empty).
	md := demokit.New("x").FromMarkdownBytes([]byte(res.DemoMD)).Markdown()
	for _, want := range []string{"Fixture Demo", "Connect", "Reproduce"} {
		if !strings.Contains(md, want) {
			t.Errorf("loaded+rendered demo missing %q:\n%s", want, md)
		}
	}
}

// TestExtractBindingsParse verifies bindings.go is valid Go and carries the
// Run closures over verbatim, keyed by step id.
func TestExtractBindingsParse(t *testing.T) {
	res := extractFixture(t)
	if _, err := parser.ParseFile(token.NewFileSet(), "bindings.go", res.BindingsGo, 0); err != nil {
		t.Fatalf("bindings.go does not parse: %v\n%s", err, res.BindingsGo)
	}
	for _, want := range []string{`demo.Bind("connect")`, `fmt.Println("hi from connect")`, `.Run(`} {
		if !strings.Contains(res.BindingsGo, want) {
			t.Errorf("bindings.go missing %q", want)
		}
	}
}

func TestExtractNoDemoErrors(t *testing.T) {
	_, err := extractFile([]byte("package main\nfunc main() {}\n"))
	if err == nil {
		t.Fatal("expected error when no demokit.New(...) is present")
	}
}
