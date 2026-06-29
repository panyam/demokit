package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
)

// runExtract converts a Go walkthrough's behavior into a bindings.go skeleton:
// each step's Run/Coalesce/… closures, rewired from Step(...).Run to
// Bind(id).Run. Content (notes, arrows, verbatim, inputs) is NOT parsed here —
// generate demo.md from the demo itself with `--doc sidecar`, which walks the
// live Demo and so handles everything this static pass cannot.
func runExtract(args []string) error {
	fs := flag.NewFlagSet("extract", flag.ContinueOnError)
	out := fs.String("out", "", "file to write the bindings skeleton (default: stdout)")
	// Accept the file before or after flags (stdlib flag stops at the first
	// positional).
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
		return fmt.Errorf("usage: demokit extract <file.go> [--out file]")
	}
	src, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	res, err := extractBindings(src)
	if err != nil {
		return err
	}
	for _, w := range res.Warnings {
		fmt.Fprintf(os.Stderr, "warning: %s\n", w)
	}
	fmt.Fprintln(os.Stderr, "note: generate the matching demo.md with:  go run <demo-dir> --doc sidecar > demo.md")
	if *out == "" {
		fmt.Print(res.BindingsGo)
		return nil
	}
	if err := os.WriteFile(*out, []byte(res.BindingsGo), 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote %s\n", *out)
	return nil
}

// extractResult is the output of extractBindings: the bindings skeleton plus
// warnings (e.g. a behavior-bearing step missing an explicit id).
type extractResult struct {
	BindingsGo string
	Warnings   []string
}

type chainCall struct {
	sel  string
	args []ast.Expr
}

type exStep struct {
	id       string
	title    string
	behavior []string // rendered ".Run(...)" / ".Coalesce(...)" etc.
}

// extractBindings parses a walkthrough and returns a bindings.go skeleton that
// binds each step's behavior by id. Only the behavior calls and the step's
// id/title are read; everything else is left to `--doc sidecar`.
func extractBindings(src []byte) (*extractResult, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "input.go", src, 0)
	if err != nil {
		return nil, err
	}
	demoVar, title, body := findDemo(f)
	if demoVar == "" {
		return nil, fmt.Errorf("no demokit.New(...) assignment found")
	}

	res := &extractResult{}
	var steps []exStep
	for _, stmt := range body.List {
		expr := stmtExpr(stmt)
		if expr == nil {
			continue
		}
		root, calls, ok := flattenChain(expr)
		if !ok || root != demoVar || len(calls) == 0 || calls[0].sel != "Step" {
			continue
		}
		if st := parseStepBehavior(calls, src, fset, &res.Warnings); len(st.behavior) > 0 {
			steps = append(steps, st)
		}
	}
	res.BindingsGo = renderBindings(title, steps)
	return res, nil
}

// findDemo locates the `demoVar := demokit.New("title")...` assignment and the
// enclosing function body, returning the demo variable, the literal title (if
// any), and the block to scan for step chains.
func findDemo(file *ast.File) (demoVar, title string, body *ast.BlockStmt) {
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
			id, ok := as.Lhs[0].(*ast.Ident)
			if !ok {
				continue
			}
			t := ""
			if len(calls[0].args) == 1 {
				t, _ = stringLit(calls[0].args[0])
			}
			return id.Name, t, fd.Body
		}
	}
	return "", "", nil
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

func parseStepBehavior(calls []chainCall, src []byte, fset *token.FileSet, warnings *[]string) exStep {
	var st exStep
	for _, c := range calls {
		switch c.sel {
		case "Step":
			if len(c.args) > 0 {
				st.title, _ = stringLit(c.args[0])
			}
		case "ID":
			if len(c.args) > 0 {
				st.id, _ = stringLit(c.args[0])
			}
		case "Run", "Coalesce", "Parse", "Timeout", "Cancellable", "InputTimeout":
			st.behavior = append(st.behavior, renderCall(c, src, fset))
		}
	}
	if len(st.behavior) > 0 && st.id == "" {
		*warnings = append(*warnings, fmt.Sprintf("step %q has behavior but no .ID(); add an explicit id so demo.md and the binding agree", st.title))
		st.id = "TODO-set-id-" + slugify(st.title)
	}
	return st
}

// renderCall reconstructs a behavior call (".Run(...)", ".Coalesce(...)") by
// slicing the original source for its arguments, so closures (and their
// comments and formatting) carry over verbatim.
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

func renderBindings(title string, steps []exStep) string {
	if title == "" {
		title = "TODO: set title (overridden by demo.md frontmatter)"
	}
	var b strings.Builder
	b.WriteString("package main\n\n")
	b.WriteString("import (\n\t_ \"embed\"\n\n\t\"github.com/panyam/demokit\"\n\t\"github.com/panyam/demokit/harness\"\n)\n\n")
	b.WriteString("//go:embed demo.md\nvar demoMD []byte\n\n")
	b.WriteString("// Generated by `demokit extract`. Content lives in demo.md (generate it\n")
	b.WriteString("// with `--doc sidecar`); this file keeps the behavior. Fix imports and any\n")
	b.WriteString("// TODO markers, then delete the original walkthrough.\n")
	b.WriteString("func main() {\n")
	fmt.Fprintf(&b, "\tdemo := demokit.New(%q).FromMarkdownBytes(demoMD)\n\n", title)
	for _, st := range steps {
		fmt.Fprintf(&b, "\tdemo.Bind(%q)", st.id)
		for _, beh := range st.behavior {
			b.WriteString(beh)
		}
		b.WriteString("\n\n")
	}
	b.WriteString("\tharness.Run(demo)\n}\n")
	return b.String()
}

// slugify mirrors demokit's heading slugifier; used only for the placeholder
// id in the missing-id warning path.
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
