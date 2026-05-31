package gotest

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// writeMod scaffolds a tiny go module with one passing test.
func writeMod(t *testing.T, dir string, testBody string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/m\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "lib.go"), []byte("package m\n\nfunc Add(a, b int) int { return a + b }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "lib_test.go"), []byte(testBody), 0o644); err != nil {
		t.Fatal(err)
	}
}

func ensureGo(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not in PATH")
	}
}

func TestRunner_PassingSuite(t *testing.T) {
	ensureGo(t)
	dir := t.TempDir()
	writeMod(t, dir, `package m

import "testing"

func TestAdd(t *testing.T) {
	if Add(2, 3) != 5 {
		t.Fatal("bad add")
	}
}
`)
	r := New()
	run, err := r.Run(context.Background(), dir)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if run.Failed != 0 {
		t.Errorf("expected 0 failed, got %d (raw: %s)", run.Failed, run.RawOutput)
	}
	if run.Passed < 1 {
		t.Errorf("expected ≥1 passed, got %d", run.Passed)
	}
	if !run.Ok() {
		t.Error("expected Ok()")
	}
	if run.CoveragePct <= 0 {
		t.Errorf("expected coverage > 0, got %.2f", run.CoveragePct)
	}
}

func TestRunner_FailingSuite(t *testing.T) {
	ensureGo(t)
	dir := t.TempDir()
	writeMod(t, dir, `package m

import "testing"

func TestAddBroken(t *testing.T) {
	if Add(2, 3) == 5 {
		t.Fatal("deliberately broken")
	}
}
`)
	r := New()
	run, err := r.Run(context.Background(), dir)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if run.Failed == 0 {
		t.Errorf("expected at least 1 failure, got %d", run.Failed)
	}
	if run.Ok() {
		t.Error("Ok() should be false")
	}
}

func TestRunner_BuildError(t *testing.T) {
	ensureGo(t)
	dir := t.TempDir()
	// Broken go file: missing closing brace.
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/m\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.go"), []byte("package m\n\nfunc Bad() int { return\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := New()
	run, err := r.Run(context.Background(), dir)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if run.Failed == 0 {
		t.Error("expected failed > 0 on build error")
	}
}

func TestParseAggregateCoverage(t *testing.T) {
	out := `{"Output":"coverage: 80.0% of statements\n"}
{"Output":"coverage: 40.0% of statements\n"}`
	got := parseAggregateCoverage(out)
	if got != 60.0 {
		t.Errorf("expected 60.0, got %.2f", got)
	}
	if v := parseAggregateCoverage("no coverage here"); v != 0 {
		t.Errorf("expected 0, got %.2f", v)
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("hello", 0); got != "hello" {
		t.Errorf("max=0 should not truncate, got %q", got)
	}
	if got := truncate("abcdef", 3); got != "abc\n...(truncated)" {
		t.Errorf("unexpected %q", got)
	}
}
