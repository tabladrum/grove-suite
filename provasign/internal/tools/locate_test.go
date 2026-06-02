package tools

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLocate_PrefersToolsRoot(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PROVASIGN_TOOLS_ROOT", root)

	bin := "fakefake-" + t.Name()
	dst := filepath.Join(root, "bin", bin)
	if runtime.GOOS == "windows" {
		dst += ".exe"
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := Locate(bin)
	if got != dst {
		t.Errorf("Locate(%q) = %q, want %q", bin, got, dst)
	}
}

func TestLocate_MissingReturnsEmpty(t *testing.T) {
	t.Setenv("PROVASIGN_TOOLS_ROOT", t.TempDir())
	if got := Locate("certainly-not-a-real-tool-name-xyz123"); got != "" {
		t.Errorf("expected empty for missing tool, got %q", got)
	}
}

func TestAvailable(t *testing.T) {
	t.Setenv("PROVASIGN_TOOLS_ROOT", t.TempDir())
	if Available("certainly-not-a-real-tool-name-xyz123") {
		t.Error("expected unavailable")
	}
}
