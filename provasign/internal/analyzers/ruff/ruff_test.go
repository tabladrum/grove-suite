package ruff

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
	if n := New().Name(); n != "ruff" {
		t.Errorf("name=%q want ruff", n)
	}
}

func TestRun_NotInstalled(t *testing.T) {
	t.Setenv("PATH", "")
	t.Setenv("HOME", t.TempDir())
	if New().Available() {
		t.Skip("ruff resolved unexpectedly")
	}
	out, err := New().Run(context.Background(), &core.ChangeSet{}, t.TempDir())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out != nil {
		t.Errorf("expected nil, got %d findings", len(out))
	}
}

func writeFakeBin(t *testing.T, name, stdout string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake-binary tests use sh; skip on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	escaped := strings.ReplaceAll(stdout, "'", `'\''`)
	body := "#!/bin/sh\nprintf '%s' '" + escaped + "'\nexit 1\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func prependPath(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HOME", t.TempDir())
}

func TestRun_ParsesJSON(t *testing.T) {
	stdout := `[
  {"code":"E501","message":"line too long","filename":"/work/a.py","location":{"row":4,"column":80}},
  {"code":"W292","message":"no newline at EOF","filename":"/work/b.py","location":{"row":10,"column":1}},
  {"code":"S101","message":"assert used","filename":"/work/c.py","location":{"row":1,"column":1}}
]`
	binDir := writeFakeBin(t, "ruff", stdout)
	prependPath(t, binDir)

	out, err := New().Run(context.Background(), &core.ChangeSet{}, t.TempDir())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(out) != 3 {
		t.Fatalf("findings=%d want 3", len(out))
	}
	if out[0].RuleID != "E501" || out[0].Severity != "high" {
		t.Errorf("E501 mapping wrong: %+v", out[0])
	}
	if out[1].Severity != "medium" {
		t.Errorf("W severity=%q want medium", out[1].Severity)
	}
	if out[2].Severity != "high" {
		t.Errorf("S severity=%q want high", out[2].Severity)
	}
}

func TestRun_EmptyAndBadJSON(t *testing.T) {
	binDir := writeFakeBin(t, "ruff", "")
	prependPath(t, binDir)
	out, err := New().Run(context.Background(), &core.ChangeSet{}, t.TempDir())
	if err != nil || out != nil {
		t.Fatalf("empty stdout: out=%v err=%v", out, err)
	}

	binDir2 := writeFakeBin(t, "ruff", "garbage")
	prependPath(t, binDir2)
	if _, err := New().Run(context.Background(), &core.ChangeSet{}, t.TempDir()); err == nil {
		t.Fatal("expected error on bad json")
	}
}

func TestMapSeverity(t *testing.T) {
	cases := map[string]string{"E1": "high", "F2": "high", "S1": "high", "W1": "medium", "I1": "info", "": "info"}
	for in, want := range cases {
		if got := string(mapSeverity(in)); got != want {
			t.Errorf("mapSeverity(%q)=%q want %q", in, got, want)
		}
	}
}
