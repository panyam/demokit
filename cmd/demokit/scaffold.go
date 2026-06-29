package main

import (
	"bytes"
	"embed"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

//go:embed templates
var templatesFS embed.FS

// validKinds is the set of per-example starters, ordered as a gradient:
// narrated (content only) ⊂ live (content + bound behavior) ⊂ branching
// (Go-driven routing/state).
var validKinds = map[string]bool{"narrated": true, "live": true, "branching": true}

// runInit scaffolds a project: the base walkthrough.mk plus one sample
// example so `make demo` works immediately. The optional positional arg is
// the target directory (default "."). --kind picks the sample's starter.
func runInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	kind := fs.String("kind", "live", "starter kind for the sample example: narrated|live|branching")
	if err := fs.Parse(args); err != nil {
		return err
	}
	root := "."
	if fs.NArg() >= 1 {
		root = fs.Arg(0)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	mk, err := templatesFS.ReadFile("templates/walkthrough.mk")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "walkthrough.mk"), mk, 0o644); err != nil {
		return err
	}
	if err := scaffoldExample(root, "hello", *kind); err != nil {
		return err
	}
	fmt.Printf("Scaffolded demokit project in %s:\n"+
		"  walkthrough.mk   base Makefile fragment\n"+
		"  hello/           %s sample\n\n"+
		"Next: cd %s && make demo\n", root, *kind, filepath.Join(root, "hello"))
	return nil
}

// runNew adds one example directory from a starter. Usage:
//
//	demokit new <name> [--kind=narrated|live|branching] [--dir=.]
func runNew(args []string) error {
	fs := flag.NewFlagSet("new", flag.ContinueOnError)
	kind := fs.String("kind", "live", "starter kind: narrated|live|branching")
	dir := fs.String("dir", ".", "parent directory to create the example in")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: demokit new <name> [--kind=narrated|live|branching] [--dir=.]")
	}
	return scaffoldExample(*dir, fs.Arg(0), *kind)
}

// scaffoldExample renders the starter of the given kind into root/name,
// substituting the title (derived from name) into each template, and writes
// a per-example Makefile that includes the project's walkthrough.mk. It
// refuses to overwrite an existing directory.
func scaffoldExample(root, name, kind string) error {
	if !validKinds[kind] {
		return fmt.Errorf("unknown kind %q (want narrated|live|branching)", kind)
	}
	dir := filepath.Join(root, name)
	if _, err := os.Stat(dir); err == nil {
		return fmt.Errorf("%s already exists", dir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	data := struct{ Name, Title string }{Name: name, Title: titleize(name)}
	entries, err := templatesFS.ReadDir("templates/" + kind)
	if err != nil {
		return err
	}
	for _, e := range entries {
		raw, err := templatesFS.ReadFile("templates/" + kind + "/" + e.Name())
		if err != nil {
			return err
		}
		tmpl, err := template.New(e.Name()).Parse(string(raw))
		if err != nil {
			return err
		}
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, data); err != nil {
			return err
		}
		out := filepath.Join(dir, strings.TrimSuffix(e.Name(), ".tmpl"))
		if err := os.WriteFile(out, buf.Bytes(), 0o644); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "Makefile"), []byte("include ../walkthrough.mk\n"), 0o644); err != nil {
		return err
	}
	fmt.Printf("Created %s (%s)\n", dir, kind)
	return nil
}

// titleize turns an example name into a demo title: "my-cool-demo" ->
// "My Cool Demo". Hyphens and underscores become spaces; each word is
// capitalized.
func titleize(name string) string {
	fields := strings.FieldsFunc(name, func(r rune) bool { return r == '-' || r == '_' })
	for i, f := range fields {
		if f == "" {
			continue
		}
		fields[i] = strings.ToUpper(f[:1]) + f[1:]
	}
	if len(fields) == 0 {
		return name
	}
	return strings.Join(fields, " ")
}
