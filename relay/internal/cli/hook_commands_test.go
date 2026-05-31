package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunHook_InstallUninstall(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if rc := runHookWith([]string{"install", "--repo", repo}, &out, &errb); rc != 0 {
		t.Fatalf("install rc=%d stderr=%s", rc, errb.String())
	}
	if !strings.Contains(out.String(), "installed:") {
		t.Errorf("install stdout: %q", out.String())
	}
	out.Reset()
	errb.Reset()
	if rc := runHookWith([]string{"uninstall", "--repo", repo}, &out, &errb); rc != 0 {
		t.Fatalf("uninstall rc=%d stderr=%s", rc, errb.String())
	}
	if !strings.Contains(out.String(), "removed") {
		t.Errorf("uninstall stdout: %q", out.String())
	}
}

func TestRunHook_NoArgs(t *testing.T) {
	var out, errb bytes.Buffer
	if rc := runHookWith(nil, &out, &errb); rc == 0 {
		t.Error("expected non-zero rc for missing subcommand")
	}
}

func TestRunHook_Unknown(t *testing.T) {
	var out, errb bytes.Buffer
	if rc := runHookWith([]string{"weird"}, &out, &errb); rc == 0 {
		t.Error("expected non-zero rc for unknown subcommand")
	}
}

func TestRunOutbox_MissingIntentStore(t *testing.T) {
	var out, errb bytes.Buffer
	if rc := runOutboxWith([]string{"push"}, &out, &errb); rc == 0 {
		t.Error("expected non-zero rc when --intent-store is missing")
	}
}

func TestRunOutbox_NoArgs(t *testing.T) {
	var out, errb bytes.Buffer
	if rc := runOutboxWith(nil, &out, &errb); rc == 0 {
		t.Error("expected non-zero rc for no args")
	}
}
