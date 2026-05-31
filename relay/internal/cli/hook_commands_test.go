package cli

import (
	"bytes"
	"os"
	"os/exec"
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

func TestRunHook_PreCommitInstallUninstall(t *testing.T) {
	repo := newGitRepo(t)
	var out, errb bytes.Buffer
	if rc := runHookWith([]string{"install", "--pre-commit", "--repo", repo}, &out, &errb); rc != 0 {
		t.Fatalf("install --pre-commit rc=%d stderr=%s", rc, errb.String())
	}
	if _, err := os.Stat(filepath.Join(repo, ".git", "hooks", "pre-commit")); err != nil {
		t.Errorf("pre-commit not written: %v", err)
	}
	out.Reset()
	errb.Reset()
	if rc := runHookWith([]string{"uninstall", "--pre-commit", "--repo", repo}, &out, &errb); rc != 0 {
		t.Fatalf("uninstall --pre-commit rc=%d stderr=%s", rc, errb.String())
	}
}

func TestRunHook_InstallAll(t *testing.T) {
	repo := newGitRepo(t)
	var out, errb bytes.Buffer
	if rc := runHookWith([]string{"install-all", "--repo", repo}, &out, &errb); rc != 0 {
		t.Fatalf("install-all rc=%d stderr=%s", rc, errb.String())
	}
	for _, name := range []string{"pre-push", "pre-commit"} {
		if _, err := os.Stat(filepath.Join(repo, ".git", "hooks", name)); err != nil {
			t.Errorf("missing %s: %v", name, err)
		}
	}
}

func TestRunHook_PreCommitNoStaged(t *testing.T) {
	// A git repo with nothing staged should pass the pre-commit gate.
	repo := newGitRepo(t)
	// Need to be a real git repo so `git diff --cached` works.
	cmds := [][]string{
		{"git", "init", "-q"},
		{"git", "config", "user.email", "t@t"},
		{"git", "config", "user.name", "t"},
	}
	for _, c := range cmds {
		cmd := exec.Command(c[0], c[1:]...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Skipf("git not usable: %v %s", err, out)
		}
	}
	rc := cmdHookPreCommit(repo, &bytes.Buffer{}, &bytes.Buffer{})
	if rc != 0 {
		t.Errorf("no-staged pre-commit should be rc=0, got %d", rc)
	}
}

// newGitRepo creates a temp dir with just enough .git skeleton for the hook
// installers to consider it a git repo.
func newGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git", "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}
