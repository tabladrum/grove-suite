package eslint

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/provasign/provasign/internal/core"
)

func TestName(t *testing.T) {
	if n := New().Name(); n != "eslint" {
		t.Errorf("name=%q want eslint", n)
	}
}

func TestRun_NotInstalled(t *testing.T) {
	// Force PATH to empty and HOME to a temp dir so Locate returns "".
	t.Setenv("PATH", "")
	t.Setenv("HOME", t.TempDir())
	a := New()
	if a.Available() {
		t.Skip("eslint resolved unexpectedly; cannot test missing-binary path")
	}
	out, err := a.Run(context.Background(), &core.ChangeSet{}, t.TempDir())
	if err != nil {
		t.Fatalf("Run unexpected error: %v", err)
	}
	if out != nil {
		t.Errorf("expected nil findings when binary missing, got %d", len(out))
	}
}

// writeFakeBin writes an executable shell script that prints the given
// stdout payload and returns exit code 1 (matching eslint's "found issues"
// behavior). Returns the directory containing the script.
func writeFakeBin(t *testing.T, name, stdout string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake-binary tests use sh scripts; skip on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	// Use printf so the script has no dependency on /bin/cat. The payload
	// is single-quoted; embedded single quotes are escaped.
	escaped := strings.ReplaceAll(stdout, "'", `'\''`)
	body := "#!/bin/sh\nprintf '%s' '" + escaped + "'\nexit 1\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// prependPath puts dir at the front of PATH and sets HOME to a temp dir so
// tools.Locate falls through to exec.LookPath.
func prependPath(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HOME", t.TempDir())
}

func TestRun_ParsesJSON(t *testing.T) {
	stdout := `[
  {"filePath":"/work/src/a.js","messages":[
    {"ruleId":"no-unused-vars","severity":2,"message":"unused","line":3,"column":5},
    {"ruleId":"semi","severity":1,"message":"missing semi","line":7,"column":1}
  ]}
]`
	binDir := writeFakeBin(t, "eslint", stdout)
	prependPath(t, binDir)

	work := t.TempDir()
	out, err := New().Run(context.Background(), &core.ChangeSet{}, work)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("findings=%d want 2", len(out))
	}
	if out[0].RuleID != "no-unused-vars" || out[0].Line != 3 {
		t.Errorf("first finding wrong: %+v", out[0])
	}
	if out[0].Severity != "high" {
		t.Errorf("severity=%q want high", out[0].Severity)
	}
	if out[1].Severity != "medium" {
		t.Errorf("severity=%q want medium", out[1].Severity)
	}
}

func TestRun_EmptyStdout(t *testing.T) {
	binDir := writeFakeBin(t, "eslint", "")
	prependPath(t, binDir)

	out, err := New().Run(context.Background(), &core.ChangeSet{}, t.TempDir())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out != nil {
		t.Errorf("expected nil findings for empty stdout, got %d", len(out))
	}
}

func TestRun_BadJSON(t *testing.T) {
	binDir := writeFakeBin(t, "eslint", "not-json")
	prependPath(t, binDir)

	_, err := New().Run(context.Background(), &core.ChangeSet{}, t.TempDir())
	if err == nil {
		t.Fatal("expected error on bad JSON")
	}
}

func TestMapSeverity(t *testing.T) {
	cases := map[int]string{0: "info", 1: "medium", 2: "high", 9: "info"}
	for in, want := range cases {
		if got := string(mapSeverity(in)); got != want {
			t.Errorf("mapSeverity(%d)=%q want %q", in, got, want)
		}
	}
}
