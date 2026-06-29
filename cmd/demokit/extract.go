package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// runExtract converts a Go walkthrough into sidecar form: a demo.md for the
// content and a bindings.go skeleton for the behavior. Without --out it
// prints both to stdout for inspection.
func runExtract(args []string) error {
	fs := flag.NewFlagSet("extract", flag.ContinueOnError)
	outDir := fs.String("out", "", "directory to write demo.md and bindings.go (default: stdout)")
	// Accept the file before or after flags: stdlib flag stops at the first
	// positional, so pull a leading non-flag arg out before parsing.
	file := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		file, args = args[0], args[1:]
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if file == "" {
		file = fs.Arg(0)
	}
	if file == "" {
		return fmt.Errorf("usage: demokit extract <file.go> [--out dir]")
	}
	src, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	res, err := extractFile(src)
	if err != nil {
		return err
	}
	for _, w := range res.Warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", w)
	}
	if *outDir == "" {
		fmt.Printf("===== demo.md =====\n%s\n===== bindings.go =====\n%s\n", res.DemoMD, res.BindingsGo)
		return nil
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(*outDir, "demo.md"), []byte(res.DemoMD), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(*outDir, "bindings.go"), []byte(res.BindingsGo), 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote %s/demo.md and %s/bindings.go\n", *outDir, *outDir)
	return nil
}

// extractResult is the output of extractFile: the rendered sidecar markdown,
// a Go bindings skeleton, and any warnings about content that could not be
// statically extracted (left as TODO markers rather than dropped).
type extractResult struct {
	DemoMD     string
	BindingsGo string
	Warnings   []string
}

type chainCall struct {
	sel  string
	args []ast.Expr
}

type exVariant struct {
	label, lang, content string
	isDefault            bool
}

type exVerbatim struct {
	title    string
	variants []exVariant
}

type exItem struct {
	section  bool
	id       string
	title    string
	note     string
	body     string
	mermaid  []string
	verbatim []exVerbatim
	behavior []string // rendered ".Run(...)" etc, for the Bind skeleton
}

// extractFile parses a demokit walkthrough and returns the sidecar markdown
// plus a bindings.go skeleton. It handles the linear builder pattern
// (demokit.New(...) plus demo.Step(...)/demo.Section(...) chains with literal
// content); non-literal content and unrecognized helpers become TODO markers
// with a warning rather than silent drops.
func extractFile(src []byte) (*extractResult, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "input.go", src, 0)
	if err != nil {
		return nil, err
	}

	demoVar, newCalls, body := findDemo(file)
	if demoVar == "" {
		return nil, fmt.Errorf("no demokit.New(...) assignment found")
	}

	res := &extractResult{}
	title, desc, actors := frontmatter(newCalls, &res.Warnings)

	used := map[string]int{}
	var items []exItem
	for _, stmt := range body.List {
		expr := stmtExpr(stmt)
		if expr == nil {
			continue
		}
		root, calls, ok := flattenChain(expr)
		if !ok || root != demoVar || len(calls) == 0 {
			continue
		}
		switch calls[0].sel {
		case "Step":
			items = append(items, parseStep(calls, src, fset, used, &res.Warnings))
		case "Section":
			items = append(items, parseSection(calls, used, &res.Warnings))
		}
	}

	res.DemoMD = renderMarkdown(title, desc, actors, items)
	res.BindingsGo = renderBindings(title, items)
	return res, nil
}

// findDemo locates the `demoVar := demokit.New(...)...` assignment and the
// function body that contains it, returning the demo variable name, the New
// chain's calls, and the enclosing block.
func findDemo(file *ast.File) (demoVar string, newCalls []chainCall, body *ast.BlockStmt) {
	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Body == nil {
			continue
		}
		for _, stmt := range fd.Body.List {
			as, ok := stmt.(*ast.AssignStmt)
			if !ok || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
				continue
			}
			_, calls, ok := flattenChain(as.Rhs[0])
			if !ok || calls[0].sel != "New" {
				continue
			}
			if id, ok := as.Lhs[0].(*ast.Ident); ok {
				return id.Name, calls, fd.Body
			}
		}
	}
	return "", nil, nil
}

// flattenChain unwinds a method-call chain (a.B(...).C(...)) into its root
// identifier and the calls in source order.
func flattenChain(e ast.Expr) (root string, calls []chainCall, ok bool) {
	var rev []chainCall
	for {
		ce, isCall := e.(*ast.CallExpr)
		if !isCall {
			break
		}
		se, isSel := ce.Fun.(*ast.SelectorExpr)
		if !isSel {
			break
		}
		rev = append(rev, chainCall{sel: se.Sel.Name, args: ce.Args})
		e = se.X
	}
	id, isID := e.(*ast.Ident)
	if !isID || len(rev) == 0 {
		return "", nil, false
	}
	for i := len(rev) - 1; i >= 0; i-- {
		calls = append(calls, rev[i])
	}
	return id.Name, calls, true
}

func stmtExpr(s ast.Stmt) ast.Expr {
	switch x := s.(type) {
	case *ast.ExprStmt:
		return x.X
	case *ast.AssignStmt:
		if len(x.Rhs) == 1 {
			return x.Rhs[0]
		}
	}
	return nil
}

func frontmatter(calls []chainCall, warnings *[]string) (title, desc string, actors [][2]string) {
	for _, c := range calls {
		switch c.sel {
		case "New":
			title = litArg(c, 0, "title", warnings)
		case "Description":
			desc = litArg(c, 0, "description", warnings)
		case "Actors":
			for _, a := range c.args {
				if id, label, ok := parseActor(a); ok {
					actors = append(actors, [2]string{id, label})
				}
			}
		}
	}
	return
}

func parseActor(e ast.Expr) (id, label string, ok bool) {
	ce, isCall := e.(*ast.CallExpr)
	if !isCall {
		return
	}
	se, isSel := ce.Fun.(*ast.SelectorExpr)
	if !isSel || se.Sel.Name != "Actor" || len(ce.Args) < 2 {
		return
	}
	id, ok1 := stringLit(ce.Args[0])
	label, ok2 := stringLit(ce.Args[1])
	return id, label, ok1 && ok2
}

func parseStep(calls []chainCall, src []byte, fset *token.FileSet, used map[string]int, warnings *[]string) exItem {
	it := exItem{}
	var explicitID string
	for _, c := range calls {
		switch c.sel {
		case "Step":
			it.title = litArg(c, 0, "step title", warnings)
		case "ID":
			explicitID = litArg(c, 0, "id", warnings)
		case "Note":
			var lines []string
			for i := range c.args {
				lines = append(lines, litArg(c, i, "note", warnings))
			}
			it.note = appendNote(it.note, strings.Join(lines, "\n"))
		case "Arrow":
			it.mermaid = append(it.mermaid, fmt.Sprintf("%s ->> %s: %s",
				litArg(c, 0, "arrow", warnings), litArg(c, 1, "arrow", warnings), litArg(c, 2, "arrow", warnings)))
		case "DashedArrow":
			it.mermaid = append(it.mermaid, fmt.Sprintf("%s -->> %s: %s",
				litArg(c, 0, "arrow", warnings), litArg(c, 1, "arrow", warnings), litArg(c, 2, "arrow", warnings)))
		case "VerbatimVariants":
			it.verbatim = append(it.verbatim, parseVerbatimVariants(c, warnings))
		case "VerbatimLang":
			it.verbatim = append(it.verbatim, exVerbatim{title: litArg(c, 0, "verbatim", warnings),
				variants: []exVariant{{lang: litArg(c, 1, "verbatim", warnings), content: litArg(c, 2, "verbatim", warnings)}}})
		case "Verbatim":
			it.verbatim = append(it.verbatim, exVerbatim{title: litArg(c, 0, "verbatim", warnings),
				variants: []exVariant{{content: litArg(c, 1, "verbatim", warnings)}}})
		case "Shell":
			it.verbatim = append(it.verbatim, exVerbatim{variants: []exVariant{{lang: "bash", content: litArg(c, 0, "shell", warnings)}}})
		case "Run", "Coalesce", "Parse", "Timeout", "Cancellable", "InputTimeout":
			it.behavior = append(it.behavior, renderCall(c, src, fset))
		case "Input":
			*warnings = append(*warnings, fmt.Sprintf("step %q: Input(...) not extracted; declare it in a demo.md inputs block", it.title))
			it.note = appendNote(it.note, "TODO(extract): declare this step's inputs in an `inputs` block")
		}
	}
	it.id = uniqueID(explicitID, it.title, used)
	return it
}

func parseSection(calls []chainCall, used map[string]int, warnings *[]string) exItem {
	it := exItem{section: true}
	c := calls[0]
	it.title = litArg(c, 0, "section title", warnings)
	var lines []string
	for i := 1; i < len(c.args); i++ {
		lines = append(lines, litArg(c, i, "section body", warnings))
	}
	it.body = strings.Join(lines, "\n")
	it.id = uniqueID("", it.title, used)
	return it
}

func parseVerbatimVariants(c chainCall, warnings *[]string) exVerbatim {
	v := exVerbatim{}
	if len(c.args) > 0 {
		v.title, _ = stringLit(c.args[0])
	}
	for _, a := range c.args[1:] {
		if va, ok := parseVariant(a); ok {
			v.variants = append(v.variants, va)
			continue
		}
		*warnings = append(*warnings, fmt.Sprintf("verbatim %q: a variant is not a literal MakeVariant(...) — emitted TODO", v.title))
		v.variants = append(v.variants, exVariant{content: "TODO(extract): variant not statically extractable"})
	}
	return v
}

func parseVariant(e ast.Expr) (exVariant, bool) {
	var v exVariant
	if ce, isCall := e.(*ast.CallExpr); isCall {
		if se, isSel := ce.Fun.(*ast.SelectorExpr); isSel && se.Sel.Name == "Default" {
			v.isDefault = true
			e = se.X
		}
	}
	ce, isCall := e.(*ast.CallExpr)
	if !isCall {
		return v, false
	}
	se, isSel := ce.Fun.(*ast.SelectorExpr)
	if !isSel || se.Sel.Name != "MakeVariant" || len(ce.Args) < 3 {
		return v, false
	}
	label, ok1 := stringLit(ce.Args[0])
	lang, ok2 := stringLit(ce.Args[1])
	content, ok3 := stringLit(ce.Args[2])
	v.label, v.lang, v.content = label, lang, content
	return v, ok1 && ok2 && ok3
}

// renderCall reconstructs a behavior call (".Run(...)", ".Coalesce(...)") by
// slicing the original source for its arguments, so closures are carried over
// verbatim into the bindings skeleton.
func renderCall(c chainCall, src []byte, fset *token.FileSet) string {
	if len(c.args) == 0 {
		return "." + c.sel + "()"
	}
	from := fset.Position(c.args[0].Pos()).Offset
	to := fset.Position(c.args[len(c.args)-1].End()).Offset
	return "." + c.sel + "(" + string(src[from:to]) + ")"
}

func stringLit(e ast.Expr) (string, bool) {
	bl, ok := e.(*ast.BasicLit)
	if !ok || bl.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(bl.Value)
	if err != nil {
		return "", false
	}
	return s, true
}

// litArg returns the i-th argument as a string literal, or "TODO(extract)…"
// plus a warning when it is missing or not a literal.
func litArg(c chainCall, i int, what string, warnings *[]string) string {
	if i >= len(c.args) {
		return ""
	}
	if s, ok := stringLit(c.args[i]); ok {
		return s
	}
	*warnings = append(*warnings, fmt.Sprintf("%s: argument %d is not a string literal — emitted TODO", what, i))
	return "TODO(extract): non-literal " + what
}

func appendNote(existing, add string) string {
	if existing == "" {
		return add
	}
	return existing + "\n\n" + add
}

func uniqueID(explicit, title string, used map[string]int) string {
	id := explicit
	if id == "" {
		id = slugify(title)
	}
	if id == "" {
		id = "step"
	}
	n := used[id]
	used[id]++
	if n == 0 {
		return id
	}
	return fmt.Sprintf("%s-%d", id, n+1)
}

// slugify mirrors demokit's heading slugifier (ASCII lower, non-alnum runs to
// single hyphens). Duplicated here because demokit's copy is unexported.
func slugify(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteRune('-')
				prevDash = true
			}
		}
	}
	return strings.TrimRight(b.String(), "-")
}

func renderMarkdown(title, desc string, actors [][2]string, items []exItem) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("title: " + title + "\n")
	if desc != "" {
		b.WriteString("description: " + desc + "\n")
	}
	if len(actors) > 0 {
		b.WriteString("actors:\n")
		for _, a := range actors {
			b.WriteString(fmt.Sprintf("  - { id: %s, label: %s }\n", a[0], a[1]))
		}
	}
	b.WriteString("---\n\n")

	for _, it := range items {
		b.WriteString("## " + it.title + " {#" + it.id + "}\n\n")
		if it.section {
			if it.body != "" {
				b.WriteString(it.body + "\n\n")
			}
			continue
		}
		if it.note != "" {
			for line := range strings.SplitSeq(it.note, "\n") {
				if line == "" {
					b.WriteString(">\n")
				} else {
					b.WriteString("> " + line + "\n")
				}
			}
			b.WriteString("\n")
		}
		if len(it.mermaid) > 0 {
			b.WriteString("```mermaid\n")
			for _, m := range it.mermaid {
				b.WriteString(m + "\n")
			}
			b.WriteString("```\n\n")
		}
		for _, v := range it.verbatim {
			for _, va := range v.variants {
				attrs := []string{fmt.Sprintf("verbatim=%q", v.title)}
				if va.label != "" {
					attrs = append(attrs, "label="+va.label)
				}
				if va.isDefault {
					attrs = append(attrs, "default=true")
				}
				b.WriteString("~~~" + va.lang + " {" + strings.Join(attrs, " ") + "}\n")
				b.WriteString(va.content + "\n~~~\n")
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}

func renderBindings(title string, items []exItem) string {
	var b strings.Builder
	b.WriteString("package main\n\n")
	b.WriteString("import (\n\t_ \"embed\"\n\n\t\"github.com/panyam/demokit\"\n\t\"github.com/panyam/demokit/harness\"\n)\n\n")
	b.WriteString("//go:embed demo.md\nvar demoMD []byte\n\n")
	b.WriteString("// Generated by `demokit extract`. Fix imports and any TODO(extract)\n")
	b.WriteString("// markers, then delete the original walkthrough.\n")
	b.WriteString("func main() {\n")
	b.WriteString(fmt.Sprintf("\tdemo := demokit.New(%q).FromMarkdownBytes(demoMD)\n\n", title))
	for _, it := range items {
		if len(it.behavior) == 0 {
			continue
		}
		b.WriteString(fmt.Sprintf("\tdemo.Bind(%q)", it.id))
		for _, beh := range it.behavior {
			b.WriteString(beh)
		}
		b.WriteString("\n\n")
	}
	b.WriteString("\tharness.Run(demo)\n}\n")
	return b.String()
}
