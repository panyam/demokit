package demokit

import (
	"os"
	"reflect"
	"testing"
)

// TestFilterArgsBuiltInStripsOnly verifies the four built-in flags are
// stripped (with both spaced and = forms for value flags) and nothing
// else is touched. This is the load-bearing case — examples rely on
// these four being inverted from demokit's own scanner.
func TestFilterArgsBuiltInStripsOnly(t *testing.T) {
	got := FilterArgs([]string{
		"--tui",
		"--non-interactive",
		"--doc", "md",
		"--from=trace.json",
		"-addr", ":8081", // unrelated, must pass through
		"positional",     // positional, must pass through
	})
	want := []string{"-addr", ":8081", "positional"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("FilterArgs built-in:\n got:  %v\n want: %v", got, want)
	}
}

// TestFilterArgsExtraBoolFlag verifies a caller-supplied bare flag is
// stripped, both forms.
func TestFilterArgsExtraBoolFlag(t *testing.T) {
	got := FilterArgs(
		[]string{"--serve", "-addr", ":8081"},
		BoolFlag("--serve"),
	)
	want := []string{"-addr", ":8081"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("FilterArgs+BoolFlag:\n got:  %v\n want: %v", got, want)
	}
}

// TestFilterArgsExtraValueFlagBothForms verifies a caller-supplied
// value flag is stripped in both `--flag value` and `--flag=value`
// forms.
func TestFilterArgsExtraValueFlagBothForms(t *testing.T) {
	got := FilterArgs(
		[]string{"--url", "http://x", "--file=y.txt", "-addr", ":8081"},
		ValueFlag("--url"),
		ValueFlag("--file"),
	)
	want := []string{"-addr", ":8081"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("FilterArgs+ValueFlag:\n got:  %v\n want: %v", got, want)
	}
}

// TestFilterArgsUnrelatedPassesThrough verifies undeclared flags
// (including ones that LOOK like value flags) pass through. This is
// the contract for downstream flag.Parse — it's responsible for its
// own flags; FilterArgs only strips what it's been told.
func TestFilterArgsUnrelatedPassesThrough(t *testing.T) {
	got := FilterArgs([]string{"--unknown", "value", "--also-unknown=v"})
	want := []string{"--unknown", "value", "--also-unknown=v"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("FilterArgs unrelated:\n got:  %v\n want: %v", got, want)
	}
}

// TestFilterArgsEmpty verifies the no-arg case.
func TestFilterArgsEmpty(t *testing.T) {
	got := FilterArgs(nil)
	if len(got) != 0 {
		t.Errorf("FilterArgs(nil) = %v, want empty", got)
	}
}

// TestFilterArgsBuiltInWithExtras verifies the built-in set composes
// with caller extras — both layers strip in the same pass.
func TestFilterArgsBuiltInWithExtras(t *testing.T) {
	got := FilterArgs(
		[]string{
			"--tui",
			"--serve",
			"--doc", "md",
			"--url", "http://x",
			"--non-interactive",
			"-addr", ":8081",
		},
		BoolFlag("--serve"),
		ValueFlag("--url"),
	)
	want := []string{"-addr", ":8081"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("FilterArgs composed:\n got:  %v\n want: %v", got, want)
	}
}

// withArgs swaps os.Args for the duration of fn — matching the
// demokit_test.go pattern for tests that read the process-global.
func withArgs(t *testing.T, args []string, fn func()) {
	t.Helper()
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = args
	fn()
}

// TestIsTUI verifies the helper detects --tui at any position and
// returns false otherwise.
func TestIsTUI(t *testing.T) {
	withArgs(t, []string{"prog", "--tui"}, func() {
		if !IsTUI() {
			t.Error("IsTUI() = false, want true with --tui present")
		}
	})
	withArgs(t, []string{"prog", "--non-interactive", "--tui", "-addr", ":8081"}, func() {
		if !IsTUI() {
			t.Error("IsTUI() should detect --tui mid-list")
		}
	})
	withArgs(t, []string{"prog", "-addr", ":8081"}, func() {
		if IsTUI() {
			t.Error("IsTUI() = true, want false with no --tui")
		}
	})
}

// TestIsNonInteractive verifies the same shape for --non-interactive.
func TestIsNonInteractive(t *testing.T) {
	withArgs(t, []string{"prog", "--non-interactive"}, func() {
		if !IsNonInteractive() {
			t.Error("IsNonInteractive() = false, want true with --non-interactive present")
		}
	})
	withArgs(t, []string{"prog", "-addr", ":8081"}, func() {
		if IsNonInteractive() {
			t.Error("IsNonInteractive() = true, want false with no --non-interactive")
		}
	})
}
