package demokit

import (
	"flag"
	"os"
	"reflect"
	"testing"
)

// TestFilterArgsCoversRegisterFlags is the drift guard: every flag
// RegisterFlags declares must be stripped by FilterArgs, so a consumer
// running its own flag.Parse(FilterArgs(...)) never chokes on a demokit
// flag. Enumerating RegisterFlags (rather than a hand-copied list) means
// adding a flag there without teaching FilterArgs fails here.
func TestFilterArgsCoversRegisterFlags(t *testing.T) {
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	New("t").RegisterFlags(fs)
	fs.VisitAll(func(f *flag.Flag) {
		name := "--" + f.Name
		args := []string{name, "v", "keep"}
		if bf, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && bf.IsBoolFlag() {
			args = []string{name, "keep"}
		}
		if got := FilterArgs(args); !reflect.DeepEqual(got, []string{"keep"}) {
			t.Errorf("FilterArgs does not strip RegisterFlags flag %s: got %v", name, got)
		}
	})
}

// TestFilterArgsBuiltInStripsOnly verifies the built-in flags are
// stripped (with both spaced and = forms for value flags) and nothing
// else is touched. This is the load-bearing case — examples rely on
// these being inverted from demokit's own scanner.
func TestFilterArgsBuiltInStripsOnly(t *testing.T) {
	got := FilterArgs([]string{
		"--tui",
		"--non-interactive",
		"--doc", "md",
		"--from=trace.json",
		"--variant", "curl",
		"-addr", ":8081", // unrelated, must pass through
		"positional", // positional, must pass through
	})
	want := []string{"-addr", ":8081", "positional"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("FilterArgs built-in:\n got:  %v\n want: %v", got, want)
	}
}

// TestFilterArgsStripsVariantEqForm verifies --variant=<value> is
// stripped alongside the spaced form. Examples that layer their own
// flags rely on this so demokit's dispatcher flags never leak into
// their flag.Parse.
func TestFilterArgsStripsVariantEqForm(t *testing.T) {
	got := FilterArgs([]string{
		"--variant=python",
		"-addr", ":8081",
	})
	want := []string{"-addr", ":8081"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("FilterArgs --variant=...:\n got:  %v\n want: %v", got, want)
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

// TestModeRecognizesBothForms verifies --mode=value and --mode value
// resolve to the same string. The unset case returns "" so callers
// can treat "" / "plain" as equivalent.
func TestModeRecognizesBothForms(t *testing.T) {
	withArgs(t, []string{"prog", "--mode=notebook"}, func() {
		if got := Mode(); got != "notebook" {
			t.Errorf("--mode=notebook → Mode() = %q, want %q", got, "notebook")
		}
	})
	withArgs(t, []string{"prog", "--mode", "tui"}, func() {
		if got := Mode(); got != "tui" {
			t.Errorf("--mode tui → Mode() = %q, want %q", got, "tui")
		}
	})
	withArgs(t, []string{"prog", "-addr", ":8081"}, func() {
		if got := Mode(); got != "" {
			t.Errorf("no --mode → Mode() = %q, want \"\"", got)
		}
	})
}

// TestModeViaTUIAlias verifies the bare --tui flag (deprecated)
// resolves to Mode() == "tui" so examples that haven't migrated to
// --mode yet keep working.
func TestModeViaTUIAlias(t *testing.T) {
	withArgs(t, []string{"prog", "--tui"}, func() {
		if got := Mode(); got != "tui" {
			t.Errorf("--tui alias → Mode() = %q, want %q", got, "tui")
		}
	})
}

// TestIsTUIHonorsMode verifies IsTUI() also recognizes --mode=tui,
// not just the legacy bare --tui flag.
func TestIsTUIHonorsMode(t *testing.T) {
	withArgs(t, []string{"prog", "--mode=tui"}, func() {
		if !IsTUI() {
			t.Error("IsTUI() = false, want true with --mode=tui")
		}
	})
}

// TestNoteAliasResolvesToNotebook verifies --note is honored both
// by Mode() and IsNotebook(), mirroring the --tui alias contract.
func TestNoteAliasResolvesToNotebook(t *testing.T) {
	withArgs(t, []string{"prog", "--note"}, func() {
		if got := Mode(); got != "notebook" {
			t.Errorf("--note → Mode() = %q, want %q", got, "notebook")
		}
		if !IsNotebook() {
			t.Error("--note → IsNotebook() = false, want true")
		}
	})
}

// TestIsNotebookHonorsMode verifies IsNotebook() also recognizes
// --mode=notebook, not just the shorthand --note flag.
func TestIsNotebookHonorsMode(t *testing.T) {
	withArgs(t, []string{"prog", "--mode=notebook"}, func() {
		if !IsNotebook() {
			t.Error("IsNotebook() = false, want true with --mode=notebook")
		}
	})
}

// TestFilterArgsStripsNoteAlias verifies --note is stripped from
// arg lists handed to caller-side flag.Parse, the same way --tui is.
func TestFilterArgsStripsNoteAlias(t *testing.T) {
	got := FilterArgs([]string{"--note", "-addr", ":8081", "positional"})
	want := []string{"-addr", ":8081", "positional"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("FilterArgs --note strip:\n got:  %v\n want: %v", got, want)
	}
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
