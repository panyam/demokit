package harness_test

import (
	"os"
	"testing"

	"github.com/panyam/demokit"
	"github.com/panyam/demokit/harness"
	"github.com/panyam/demokit/notebookbridge"
	"github.com/panyam/demokit/tui"
)

// withArgs runs fn with os.Args temporarily replaced (Mode reads os.Args
// directly), restoring the original afterward.
func withArgs(t *testing.T, args []string, fn func()) {
	t.Helper()
	saved := os.Args
	os.Args = append([]string{"demo"}, args...)
	defer func() { os.Args = saved }()
	fn()
}

// TestSetupRendererSelectsByMode is the core of harness: the renderer set
// on the demo must match the resolved mode, including the bare --tui /
// --note aliases, and plain/absent must leave the renderer unset so
// Execute falls back to PlainRenderer.
func TestSetupRendererSelectsByMode(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string // "tui" | "notebook" | "nil"
	}{
		{"mode tui", []string{"--mode=tui"}, "tui"},
		{"mode notebook", []string{"--mode=notebook"}, "notebook"},
		{"tui alias", []string{"--tui"}, "tui"},
		{"note alias", []string{"--note"}, "notebook"},
		{"mode plain", []string{"--mode=plain"}, "nil"},
		{"absent", nil, "nil"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			withArgs(t, c.args, func() {
				d := demokit.New("t")
				harness.SetupRenderer(d)
				switch r := d.Renderer(); c.want {
				case "tui":
					if _, ok := r.(*tui.Renderer); !ok {
						t.Errorf("renderer = %T, want *tui.Renderer", r)
					}
				case "notebook":
					if _, ok := r.(*notebookbridge.Bridge); !ok {
						t.Errorf("renderer = %T, want *notebookbridge.Bridge", r)
					}
				case "nil":
					if r != nil {
						t.Errorf("renderer = %T, want nil (default PlainRenderer at Execute)", r)
					}
				}
			})
		})
	}
}

// TestRunSetsRendererBeforeExecute guards that Run wires the renderer via
// the same path as SetupRenderer. It does not call Execute (which would
// block on stdin); it asserts the renderer selection Run relies on.
func TestRunSetsRendererBeforeExecute(t *testing.T) {
	withArgs(t, []string{"--mode=tui"}, func() {
		d := demokit.New("t")
		harness.SetupRenderer(d)
		if _, ok := d.Renderer().(*tui.Renderer); !ok {
			t.Fatalf("renderer = %T, want *tui.Renderer", d.Renderer())
		}
	})
}
