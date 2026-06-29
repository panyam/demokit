package main

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

const fixtureWalkthrough = `package main

import (
	"fmt"
	"github.com/panyam/demokit"
)

func main() {
	demo := demokit.New("Fixture Demo").Description("A test demo")

	demo.Section("Intro", "first line")

	demo.Step("Connect").ID("connect").
		Note("a note").
		VerbatimVariants("Reproduce", demokit.MakeVariant("curl", "bash", "curl x").Default()).
		Coalesce(func(m map[string]any) any { return m["x"] }).
		Run(func(ctx demokit.StepContext) *demokit.StepResult {
			fmt.Println("hi from connect")
			return nil
		})

	demo.Step("No id step").
		Run(func(ctx demokit.StepContext) *demokit.StepResult { return nil })

	demo.Step("Content only").ID("content").
		Note("just prose, no behavior")

	demo.Execute()
}
`

func extractFixture(t *testing.T) *extractResult {
	t.Helper()
	res, err := extractBindings([]byte(fixtureWalkthrough))
	if err != nil {
		t.Fatalf("extractBindings: %v", err)
	}
	return res
}

// TestExtractBindingsParse verifies the skeleton is valid Go, carries the Run
// and Coalesce closures verbatim, and binds them by step id.
func TestExtractBindingsParse(t *testing.T) {
	res := extractFixture(t)
	if _, err := parser.ParseFile(token.NewFileSet(), "bindings.go", res.BindingsGo, 0); err != nil {
		t.Fatalf("bindings.go does not parse: %v\n%s", err, res.BindingsGo)
	}
	for _, want := range []string{
		`demokit.New("Fixture Demo")`,
		`demo.Bind("connect")`,
		`.Coalesce(func(m map[string]any) any`,
		`fmt.Println("hi from connect")`,
		`harness.Run(demo)`,
	} {
		if !strings.Contains(res.BindingsGo, want) {
			t.Errorf("bindings.go missing %q", want)
		}
	}
}

// TestExtractSkipsContentOnlySteps verifies a step with no behavior is not
// bound (its content is demo.md's job, not bindings.go's).
func TestExtractSkipsContentOnlySteps(t *testing.T) {
	res := extractFixture(t)
	if strings.Contains(res.BindingsGo, `demo.Bind("content")`) {
		t.Errorf("content-only step should not be bound:\n%s", res.BindingsGo)
	}
}

// TestExtractMissingIDWarns verifies a behavior-bearing step without an
// explicit ID produces a warning and a TODO placeholder id (so the author
// notices rather than getting a silently-wrong join key).
func TestExtractMissingIDWarns(t *testing.T) {
	res := extractFixture(t)
	var warned bool
	for _, w := range res.Warnings {
		if strings.Contains(w, "No id step") && strings.Contains(w, ".ID()") {
			warned = true
		}
	}
	if !warned {
		t.Errorf("expected a missing-id warning, got %v", res.Warnings)
	}
	if !strings.Contains(res.BindingsGo, "TODO-set-id-no-id-step") {
		t.Errorf("expected a TODO placeholder id in bindings.go:\n%s", res.BindingsGo)
	}
}

func TestExtractNoDemoErrors(t *testing.T) {
	_, err := extractBindings([]byte("package main\nfunc main() {}\n"))
	if err == nil {
		t.Fatal("expected error when no demokit.New(...) is present")
	}
}
