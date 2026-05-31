package githook

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestInstall_FreshRepo(t *testing.T) {
	repo := initRepo(t)
	path, err := Install(repo, false)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	want := filepath.Join(repo, ".git", "hooks", "pre-push")
	if path != want {
		t.Errorf("path=%q want %q", path, want)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), ManagedMarker) {
		t.Error("body missing ManagedMarker")
	}
	if !strings.Contains(string(body), "relay hook run") {
		t.Error("body missing relay hook run invocation")
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(path)
		if info.Mode().Perm()&0o100 == 0 {
			t.Errorf("hook not executable: %v", info.Mode())
		}
	}
}

func TestInstall_IdempotentOnManaged(t *testing.T) {
	repo := initRepo(t)
	if _, err := Install(repo, false); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(repo, false); err != nil {
		t.Errorf("second Install on managed hook should succeed, got %v", err)
	}
}

func TestInstall_RefusesForeign(t *testing.T) {
	repo := initRepo(t)
	hooks := filepath.Join(repo, ".git", "hooks")
	if err := os.MkdirAll(hooks, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hooks, "pre-push"), []byte("#!/bin/sh\necho hi\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(repo, false); err == nil {
		t.Fatal("expected error overwriting foreign hook")
	}
	if _, err := Install(repo, true); err != nil {
		t.Errorf("--force should overwrite foreign hook, got %v", err)
	}
	managed, err := IsManaged(filepath.Join(hooks, "pre-push"))
	if err != nil || !managed {
		t.Errorf("hook not managed after force install: managed=%v err=%v", managed, err)
	}
}

func TestUninstall_RemovesManagedOnly(t *testing.T) {
	repo := initRepo(t)
	if _, err := Install(repo, false); err != nil {
		t.Fatal(err)
	}
	removed, err := Uninstall(repo)
	if err != nil || !removed {
		t.Fatalf("expected removed=true err=nil, got %v %v", removed, err)
	}
	// Second uninstall: nothing to remove.
	removed, err = Uninstall(repo)
	if err != nil || removed {
		t.Errorf("second uninstall should be no-op, got removed=%v err=%v", removed, err)
	}
}

func TestUninstall_LeavesForeignAlone(t *testing.T) {
	repo := initRepo(t)
	hooks := filepath.Join(repo, ".git", "hooks")
	if err := os.MkdirAll(hooks, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(hooks, "pre-push")
	body := []byte("#!/bin/sh\necho foreign\n")
	if err := os.WriteFile(path, body, 0o755); err != nil {
		t.Fatal(err)
	}
	removed, err := Uninstall(repo)
	if err != nil || removed {
		t.Errorf("uninstall foreign: removed=%v err=%v", removed, err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != string(body) {
		t.Error("foreign hook was modified")
	}
}

func TestIsManaged_Missing(t *testing.T) {
	m, err := IsManaged(filepath.Join(t.TempDir(), "nope"))
	if err != nil || m {
		t.Errorf("missing: managed=%v err=%v", m, err)
	}
}

func TestInstall_EmptyRepoRoot(t *testing.T) {
	if _, err := Install("", false); err == nil {
		t.Error("expected error for empty repoRoot")
	}
}

func TestInstall_NoGitDir(t *testing.T) {
	if _, err := Install(t.TempDir(), false); err == nil {
		t.Error("expected error when .git is missing")
	}
}
