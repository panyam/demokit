package main

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/panyam/demokit"
)

func TestTitleize(t *testing.T) {
	cases := map[string]string{
		"hello":         "Hello",
		"my-cool-demo":  "My Cool Demo",
		"token_refresh": "Token Refresh",
	}
	for in, want := range cases {
		if got := titleize(in); got != want {
			t.Errorf("titleize(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestScaffoldKindsProduceValidProject verifies every starter writes a
// parseable main.go, and that narrated/live sidecars load through demokit's
// own markdown loader — the real acceptance check for generated content.
func TestScaffoldKindsProduceValidProject(t *testing.T) {
	for _, kind := range []string{"narrated", "live", "branching"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			if err := scaffoldExample(root, "ex", kind); err != nil {
				t.Fatalf("scaffoldExample: %v", err)
			}
			dir := filepath.Join(root, "ex")

			mainSrc, err := os.ReadFile(filepath.Join(dir, "main.go"))
			if err != nil {
				t.Fatalf("read main.go: %v", err)
			}
			if _, err := parser.ParseFile(token.NewFileSet(), "main.go", mainSrc, 0); err != nil {
				t.Errorf("generated main.go does not parse: %v", err)
			}
			if !strings.Contains(string(mainSrc), `demokit.New("Ex")`) {
				t.Errorf("generated main.go missing titleized New(\"Ex\")")
			}

			mdPath := filepath.Join(dir, "demo.md")
			md, err := os.ReadFile(mdPath)
			switch kind {
			case "narrated", "live":
				if err != nil {
					t.Fatalf("expected demo.md for kind %s: %v", kind, err)
				}
				out := demokit.New("x").FromMarkdownBytes(md).Markdown()
				if !strings.Contains(out, "Ex") {
					t.Errorf("generated demo.md did not load/render:\n%s", out)
				}
			case "branching":
				if err == nil {
					t.Errorf("branching kind should not emit demo.md")
				}
			}

			if _, err := os.Stat(filepath.Join(dir, "Makefile")); err != nil {
				t.Errorf("missing per-example Makefile: %v", err)
			}
		})
	}
}

func TestScaffoldInvalidKind(t *testing.T) {
	if err := scaffoldExample(t.TempDir(), "ex", "bogus"); err == nil {
		t.Fatal("expected error for unknown kind")
	}
}

func TestScaffoldRefusesExisting(t *testing.T) {
	root := t.TempDir()
	if err := scaffoldExample(root, "ex", "live"); err != nil {
		t.Fatalf("first scaffold: %v", err)
	}
	if err := scaffoldExample(root, "ex", "live"); err == nil {
		t.Fatal("expected error when target dir already exists")
	}
}

// TestInitScaffoldsProject verifies init writes the base Makefile fragment and
// a runnable sample example.
func TestInitScaffoldsProject(t *testing.T) {
	root := t.TempDir()
	if err := runInit([]string{root}); err != nil {
		t.Fatalf("runInit: %v", err)
	}
	for _, p := range []string{"walkthrough.mk", "hello/main.go", "hello/demo.md", "hello/Makefile"} {
		if _, err := os.Stat(filepath.Join(root, p)); err != nil {
			t.Errorf("init did not create %s: %v", p, err)
		}
	}
}
