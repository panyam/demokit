package demokit

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// Copy writes s to the system clipboard, trying terminal-side OSC 52
// first and falling back to OS-specific shell tools. Returns the
// strategy name that succeeded ("osc52", "pbcopy", "wl-copy", "xclip",
// "xsel", "clip") and ok=true, or ("", false) if every strategy failed
// (missing tools, OSC 52 disabled, hung daemons, etc.).
//
// Strategy order (first success wins):
//
//  1. OSC 52 escape sequence. Works over SSH and inside tmux (≥3.3 with
//     set-clipboard on) without a binary on the host. Some corporate
//     environments disable it.
//  2. pbcopy   (darwin)
//  3. wl-copy  (Wayland — typical on modern Linux laptops)
//  4. xclip    (X11)
//  5. xsel     (X11 fallback)
//  6. clip.exe (Windows; works in WSL where /mnt/c/Windows/System32 is
//     on PATH)
//
// Each shell-out is given a 2-second context deadline so a hung
// clipboard daemon never blocks demokit's caller. Missing tools (no
// exec.LookPath hit) are silent skips, not errors.
//
// Callers may swap the writer used by the OSC 52 strategy via the
// SetClipboardWriter package function — primarily for tests; default
// is os.Stderr (writing to stdout would be captured by demokit's step
// output redirect during Run).
func Copy(s string) (strategy string, ok bool) {
	if writeOSC52(s) {
		return "osc52", true
	}
	for _, c := range shellCopyCandidates() {
		if runShellCopy(c, s) {
			return c.name, true
		}
	}
	return "", false
}

// clipboardOut is the writer the OSC 52 strategy targets. Defaults to
// os.Stderr because stdout is captured during step execution; writing
// the escape sequence to stdout would loop into demokit's capture pipe
// and never reach the terminal.
var clipboardOut io.Writer = os.Stderr

// SetClipboardWriter overrides the writer used by the OSC 52 strategy.
// Pass nil to restore the default (os.Stderr). Tests use this to
// capture the emitted escape sequence without touching the real
// terminal.
func SetClipboardWriter(w io.Writer) {
	if w == nil {
		clipboardOut = os.Stderr
		return
	}
	clipboardOut = w
}

// shellCopyEnabled controls whether Copy attempts the shell-out
// fallback strategies. Tests set this to false to assert OSC 52 is the
// only path exercised. Default true.
var shellCopyEnabled = true

// EnableShellClipboardFallback toggles whether Copy attempts shell-out
// strategies (pbcopy / wl-copy / xclip / xsel / clip.exe) after OSC 52.
// Primarily for tests; production callers leave this on.
func EnableShellClipboardFallback(enabled bool) {
	shellCopyEnabled = enabled
}

// writeOSC52 emits the OSC 52 clipboard-write escape sequence to
// clipboardOut. Returns true if the write succeeded — note that
// success here means "the bytes left the process," not "the terminal
// honored the request." There is no in-band acknowledgement from the
// terminal for OSC 52; demokit treats a successful Write as good
// enough and presents the strategy as "osc52" to the user.
//
// Format: ESC ]52;c;<base64(payload)> BEL
func writeOSC52(s string) bool {
	enc := base64.StdEncoding.EncodeToString([]byte(s))
	seq := "\x1b]52;c;" + enc + "\x07"
	_, err := fmt.Fprint(clipboardOut, seq)
	return err == nil
}

// shellCopyCandidate describes one shell-fallback clipboard strategy.
type shellCopyCandidate struct {
	name string   // strategy label returned from Copy on success
	cmd  string   // binary to look up via exec.LookPath
	args []string // arguments to pass to the binary
}

// shellCopyCandidates returns the platform-appropriate shell fallback
// strategies in priority order. Filtered to candidates whose binary
// is actually present on PATH so a Copy call only spends a context
// timeout on tools that exist.
func shellCopyCandidates() []shellCopyCandidate {
	if !shellCopyEnabled {
		return nil
	}
	var all []shellCopyCandidate
	switch runtime.GOOS {
	case "darwin":
		all = []shellCopyCandidate{
			{name: "pbcopy", cmd: "pbcopy"},
		}
	case "linux", "freebsd", "openbsd", "netbsd":
		all = []shellCopyCandidate{
			{name: "wl-copy", cmd: "wl-copy"},
			{name: "xclip", cmd: "xclip", args: []string{"-selection", "clipboard"}},
			{name: "xsel", cmd: "xsel", args: []string{"--clipboard", "--input"}},
		}
	case "windows":
		all = []shellCopyCandidate{
			{name: "clip", cmd: "clip"},
		}
	default:
		return nil
	}
	// On Linux under WSL, clip.exe is reachable and is the only path
	// to the host Windows clipboard. Try it after the native tools.
	if runtime.GOOS == "linux" && isWSL() {
		all = append(all, shellCopyCandidate{name: "clip", cmd: "clip.exe"})
	}

	out := all[:0]
	for _, c := range all {
		if _, err := exec.LookPath(c.cmd); err == nil {
			out = append(out, c)
		}
	}
	return out
}

// runShellCopy invokes the given candidate with a 2-second deadline.
// Returns true if the command exited 0. False covers every failure
// shape (LookPath miss already filtered, timeout, non-zero exit, IO
// error). A failure is silent — the caller proceeds to the next
// candidate.
func runShellCopy(c shellCopyCandidate, s string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.cmd, c.args...)
	cmd.Stdin = strings.NewReader(s)
	// Discard stdout/stderr — the tool's output isn't useful here and
	// would otherwise leak into the caller's terminal.
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run() == nil
}

// isWSL returns true when running inside Windows Subsystem for Linux.
// Used to opportunistically try clip.exe alongside the native Linux
// clipboard tools. Cheap to compute; called at most once per Copy
// failure path.
func isWSL() bool {
	if os.Getenv("WSL_DISTRO_NAME") != "" || os.Getenv("WSL_INTEROP") != "" {
		return true
	}
	if b, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		return strings.Contains(strings.ToLower(string(b)), "microsoft")
	}
	return false
}
