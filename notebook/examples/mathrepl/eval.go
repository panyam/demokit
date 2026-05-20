package main

import (
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/expr-lang/expr"
)

// Env holds the math REPL's variable + function environment. It
// wraps github.com/expr-lang/expr — expressions go through
// expr.Eval, plus a small assignment syntax (`x = <expr>`) layered
// on top because expr is expression-only.
type Env struct {
	vars map[string]any
}

// NewEnv returns an Env with math.Pi / math.E and the common
// math.* functions pre-bound.
func NewEnv() *Env {
	return &Env{vars: map[string]any{
		"pi":    math.Pi,
		"e":     math.E,
		"sin":   math.Sin,
		"cos":   math.Cos,
		"tan":   math.Tan,
		"sqrt":  math.Sqrt,
		"log":   math.Log,
		"log10": math.Log10,
		"exp":   math.Exp,
		"abs":   math.Abs,
		"floor": math.Floor,
		"ceil":  math.Ceil,
		"pow":   math.Pow,
	}}
}

// SetVar binds name to value (used by PlotCell to drive the free
// variable across the x range).
func (e *Env) SetVar(name string, value any) { e.vars[name] = value }

// assignRe matches `<ident> = <expr>` — the layered assignment
// syntax. expr-lang doesn't include assignment, so we strip it
// here and eval the right side.
var assignRe = regexp.MustCompile(`^\s*([a-zA-Z_]\w*)\s*=\s*(.+)$`)

// Eval evaluates src against the environment. If src is an
// assignment, the value is bound to the name AND returned.
func (e *Env) Eval(src string) (any, error) {
	src = strings.TrimSpace(src)
	if m := assignRe.FindStringSubmatch(src); m != nil {
		val, err := e.evalExpr(m[2])
		if err != nil {
			return nil, err
		}
		e.vars[m[1]] = val
		return val, nil
	}
	return e.evalExpr(src)
}

// evalExpr is the pure expression-eval path (no assignment).
func (e *Env) evalExpr(src string) (any, error) {
	out, err := expr.Eval(src, e.vars)
	if err != nil {
		return nil, fmt.Errorf("%s", trimExprError(err.Error()))
	}
	return out, nil
}

// AsFloat coerces an Eval result to float64 for plotting; returns
// NaN if the value isn't numeric. Plot callers use NaN as the
// "skip this sample" signal.
func AsFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int32:
		return float64(x)
	case int64:
		return float64(x)
	default:
		return math.NaN()
	}
}

// trimExprError shortens expr-lang's verbose multi-line error
// format to the first line, which is the actual message — the
// rest is source-position context too noisy for the REPL result.
func trimExprError(msg string) string {
	if i := strings.IndexByte(msg, '\n'); i >= 0 {
		return msg[:i]
	}
	return msg
}
