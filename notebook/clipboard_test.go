package notebook

import (
	"os"
	"strings"
	"testing"
)

func TestFileClipboardWritesPayloadToTmpDir(t *testing.T) {
	dir := t.TempDir()
	clip := FileClipboard(dir)

	got, ok := clip("hello\nworld")
	if !ok {
		t.Fatalf("FileClipboard returned ok=false; expected success in writable tmp dir")
	}
	if !strings.HasPrefix(got, dir) {
		t.Errorf("returned path = %q, want a path under %q", got, dir)
	}
	data, err := os.ReadFile(got)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", got, err)
	}
	if string(data) != "hello\nworld" {
		t.Errorf("file contents = %q, want %q", string(data), "hello\nworld")
	}
}

func TestFileClipboardFailsOnUnwritableDir(t *testing.T) {
	// Pass a path that doesn't exist and can't be created — the
	// nested-into-nonexistent-parent case CreateTemp rejects.
	clip := FileClipboard("/nonexistent/path/that/will/not/exist")
	got, ok := clip("x")
	if ok {
		_ = os.Remove(got) // shouldn't have created anything, but be tidy
		t.Errorf("FileClipboard with unwritable dir returned ok=true; want false")
	}
}

func TestFileClipboardEmptyDirUsesOsTempDir(t *testing.T) {
	clip := FileClipboard("")
	got, ok := clip("payload")
	if !ok {
		t.Fatalf("FileClipboard(\"\") failed; expected to use os.TempDir() and succeed")
	}
	defer os.Remove(got)
	if !strings.HasPrefix(got, os.TempDir()) {
		t.Errorf("path = %q, want one under os.TempDir() = %q", got, os.TempDir())
	}
}
