package stage1

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tabladrum/grove-suite/relay/internal/core"
	"github.com/tabladrum/grove-suite/relay/internal/runner/gotest"
)

func ensureGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not in PATH")
	}
}

func ensureGo(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not in PATH")
	}
}

// initRepo creates a git repo with a single passing Go test and returns its
// path plus the initial commit SHA.
func initRepo(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	mustRun(t, dir, "git", "init", "-q", "-b", "main")
	mustRun(t, dir, "git", "config", "user.email", "test@example.com")
	mustRun(t, dir, "git", "config", "user.name", "Test")

	writeFile(t, dir, "go.mod", "module example.com/m\n\ngo 1.21\n")
	writeFile(t, dir, "lib.go", "package m\n\nfunc Add(a, b int) int { return a + b }\n")
	writeFile(t, dir, "lib_test.go", `package m

import "testing"

func TestAdd(t *testing.T) {
	if Add(1, 2) != 3 {
		t.Fatal("bad")
	}
}
`)
	mustRun(t, dir, "git", "add", ".")
	mustRun(t, dir, "git", "commit", "-q", "-m", "init")
	sha := strings.TrimSpace(capture(t, dir, "git", "rev-parse", "HEAD"))
	return dir, sha
}

func TestStage1_PassingDiff(t *testing.T) {
	ensureGit(t)
	ensureGo(t)
	repo, sha := initRepo(t)

	// Empty diff: just runs build+tests on HEAD.
	cs := &core.ChangeSet{RepoRoot: repo, BaseSHA: sha, Diff: ""}
	s := New(gotest.New())
	res, err := s.Run(context.Background(), cs)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if !res.Ok() {
		t.Errorf("expected ok, got build=%v tests=%+v output=%s", res.BuildOk, res.Tests, res.BuildOutput)
	}
}

func TestStage1_BreakingDiff(t *testing.T) {
	ensureGit(t)
	ensureGo(t)
	repo, sha := initRepo(t)

	// Generate a real diff by modifying lib.go then `git diff`.
	writeFile(t, repo, "lib.go", "package m\n\nfunc Add(a, b int) int { return a - b }\n")
	diff := capture(t, repo, "git", "diff")
	mustRun(t, repo, "git", "checkout", "--", "lib.go")

	cs := &core.ChangeSet{RepoRoot: repo, BaseSHA: sha, Diff: diff}
	s := New(gotest.New())
	res, err := s.Run(context.Background(), cs)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if !res.BuildOk {
		t.Fatalf("build should succeed for this diff: %s", res.BuildOutput)
	}
	if res.Tests.Failed == 0 {
		t.Errorf("expected test failure, got tests=%+v", res.Tests)
	}
	if res.Ok() {
		t.Error("Stage1Result.Ok() should be false")
	}
}

func TestStage1_NilChangeSet(t *testing.T) {
	s := New(gotest.New())
	if _, err := s.Run(context.Background(), nil); err == nil {
		t.Error("expected error for nil cs")
	}
}

func TestStage1_MissingRepo(t *testing.T) {
	s := New(gotest.New())
	cs := &core.ChangeSet{RepoRoot: filepath.Join(t.TempDir(), "nope")}
	if _, err := s.Run(context.Background(), cs); err == nil {
		t.Error("expected error for missing repo")
	}
}

func TestStage1_BadDiff(t *testing.T) {
	ensureGit(t)
	ensureGo(t)
	repo, sha := initRepo(t)
	cs := &core.ChangeSet{RepoRoot: repo, BaseSHA: sha, Diff: "not a diff at all\n"}
	s := New(gotest.New())
	_, err := s.Run(context.Background(), cs)
	if err == nil {
		t.Error("expected apply diff error")
	}
}

func TestFingerprintDiff_Stable(t *testing.T) {
	a := FingerprintDiff("hello")
	b := FingerprintDiff("hello")
	c := FingerprintDiff("world")
	if a != b {
		t.Error("not stable")
	}
	if a == c {
		t.Error("different inputs collided")
	}
	if len(a) != 16 {
		t.Errorf("expected 16 hex chars, got %d", len(a))
	}
}

func TestStage1_BuildErrorSkipsTests(t *testing.T) {
	ensureGit(t)
	ensureGo(t)
	repo, sha := initRepo(t)

	writeFile(t, repo, "lib.go", "package m\n\nfunc Add(a, b int) int { return\n")
	diff := capture(t, repo, "git", "diff")
	mustRun(t, repo, "git", "checkout", "--", "lib.go")

	cs := &core.ChangeSet{RepoRoot: repo, BaseSHA: sha, Diff: diff}
	s := New(gotest.New())
	res, err := s.Run(context.Background(), cs)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if res.BuildOk {
		t.Error("build should fail")
	}
	if res.Tests.Passed != 0 || res.Tests.Failed != 0 || res.Tests.Runner != "" {
		t.Errorf("tests should not run when build fails, got %+v", res.Tests)
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustRun(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, buf.String())
	}
}

func capture(t *testing.T, dir, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%s %v: %v", name, args, err)
	}
	return string(out)
}
