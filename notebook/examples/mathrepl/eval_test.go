package main

import (
	"math"
	"testing"
)

func TestEvalArithmeticAndPrecedence(t *testing.T) {
	env := NewEnv()
	v, err := env.Eval("2 + 2 * 3")
	if err != nil {
		t.Fatalf("Eval error: %v", err)
	}
	if AsFloat(v) != 8 {
		t.Errorf("2 + 2*3 = %v, want 8", v)
	}
}

func TestEvalMathFunctions(t *testing.T) {
	env := NewEnv()
	v, err := env.Eval("sin(pi/2)")
	if err != nil {
		t.Fatalf("Eval error: %v", err)
	}
	if got := AsFloat(v); math.Abs(got-1) > 1e-9 {
		t.Errorf("sin(pi/2) = %v, want ~1", got)
	}
}

func TestEvalAssignmentBindsAndReturnsValue(t *testing.T) {
	env := NewEnv()
	if _, err := env.Eval("x = 5"); err != nil {
		t.Fatalf("assign error: %v", err)
	}
	v, err := env.Eval("x * 2")
	if err != nil {
		t.Fatalf("Eval x*2 error: %v", err)
	}
	if AsFloat(v) != 10 {
		t.Errorf("x*2 after x=5 = %v, want 10", v)
	}
}

func TestEvalUnknownFunctionErrors(t *testing.T) {
	env := NewEnv()
	if _, err := env.Eval("nope(1)"); err == nil {
		t.Error("unknown function should error")
	}
}

func TestEvalErrorMessageIsSingleLine(t *testing.T) {
	env := NewEnv()
	_, err := env.Eval("bad syntax !!!")
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, c := range err.Error() {
		if c == '\n' {
			t.Errorf("error message should be single-line for the REPL; got:\n%s", err.Error())
			break
		}
	}
}

func TestAsFloatCoercesNumericTypes(t *testing.T) {
	cases := []struct {
		in   any
		want float64
	}{
		{3.14, 3.14},
		{int(7), 7},
		{int64(11), 11},
	}
	for _, tc := range cases {
		if got := AsFloat(tc.in); got != tc.want {
			t.Errorf("AsFloat(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
	if got := AsFloat("not a number"); !math.IsNaN(got) {
		t.Errorf("AsFloat(non-numeric) = %v, want NaN", got)
	}
}
