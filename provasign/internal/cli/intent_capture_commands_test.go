package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// chdirCapture switches CWD for the test lifetime. Named to avoid the
// duplicate-symbol clash with chdir / chdirKeys in this package.
func chdirCapture(t *testing.T, dir string) {
	t.Helper()
	prev, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
}

// TestRunIntentCapture_LaptopModeRoutesListToCaptured covers the
// production-gap fix: `relay intent list` in laptop mode (no
// DATABASE_URL) returns captured intents rather than attempting a
// Postgres connection.
func TestRunIntentCapture_LaptopModeRoutesListToCaptured(t *testing.T) {
	dir := t.TempDir()
	chdirCapture(t, dir)
	// Force laptop mode: ensure no DATABASE_URL is set for this test.
	t.Setenv("DATABASE_URL", "")

	if err := os.MkdirAll(filepath.Join(dir, ".provasign", "intents"), 0o755); err != nil {
		t.Fatal(err)
	}

	handled, exit := RunIntentCapture([]string{"list"})
	if !handled {
		t.Fatal("expected list to be handled in laptop mode")
	}
	if exit != 0 {
		t.Errorf("exit = %d, want 0 (empty intent dir is valid)", exit)
	}
}

// TestRunIntentCapture_TeamModeFallsThroughList confirms that when
// DATABASE_URL is set, `intent list` is NOT short-circuited — the
// legacy Postgres-backed handler in cmdIntent gets a chance to run.
func TestRunIntentCapture_TeamModeFallsThroughList(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example.com/db")

	handled, _ := RunIntentCapture([]string{"list"})
	if handled {
		t.Error("expected list to fall through in team mode (DATABASE_URL set)")
	}
}

func TestIsLaptopMode(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	if !isLaptopMode() {
		t.Error("empty DATABASE_URL should be laptop mode")
	}
	t.Setenv("DATABASE_URL", "postgres://x")
	if isLaptopMode() {
		t.Error("DATABASE_URL set should NOT be laptop mode")
	}
}

// TestRunIntentCapture_KnownSubcommandsHandled spot-checks the simple
// open/update/close/list-captured/get-captured paths still report handled.
// We deliberately pass empty arg lists; these return non-zero exits but
// the routing assertion is what matters.
func TestRunIntentCapture_KnownSubcommandsHandled(t *testing.T) {
	dir := t.TempDir()
	chdirCapture(t, dir)
	for _, sub := range []string{"open", "update", "close", "list-captured", "get-captured"} {
		handled, _ := RunIntentCapture([]string{sub})
		if !handled {
			t.Errorf("RunIntentCapture(%q) should be handled=true", sub)
		}
	}
	handled, _ := RunIntentCapture([]string{"unknown-cmd"})
	if handled {
		t.Error("unknown subcommand should NOT be handled")
	}
}
