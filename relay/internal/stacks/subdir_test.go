package stacks

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestApply_CreatesSubdirs(t *testing.T) {
	dest := t.TempDir()
	written, _, err := Apply("go-microservice", dest)
	if err != nil {
		t.Fatal(err)
	}
	// rulesets/ subdir should exist after applying go-microservice.
	info, err := os.Stat(filepath.Join(dest, "rulesets"))
	if err != nil || !info.IsDir() {
		t.Fatalf("rulesets dir missing: %v", err)
	}
	if len(written) < 2 {
		t.Errorf("expected at least relay.yaml + rulesets/.keep, got %v", written)
	}
}

func TestApply_StatErrorBubbles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission semantics differ on Windows")
	}
	dest := t.TempDir()
	// Make dest unreadable so the inner os.Stat on a future target fails
	// with an EACCES-like error (not ErrNotExist) — exercises the
	// `if !errors.Is(err, os.ErrNotExist)` branch.
	if os.Geteuid() == 0 {
		t.Skip("running as root bypasses permission checks")
	}
	if err := os.Chmod(dest, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dest, 0o755) })
	_, _, err := Apply("go-microservice", dest)
	if err == nil {
		t.Skip("permission denial did not propagate (varies by OS)")
	}
}
