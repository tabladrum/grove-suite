package cli

// MVP-L2 closing integration test:
//   - A ChangeSet with a deliberate test break is rejected with actionable
//     diagnostics.
//   - The same intent re-expressed as a passing diff is admitted.

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initGoRepo creates a git repo containing a tiny Go module with one passing
// test, returns the repo path.
func initGoRepo(t *testing.T) string {
	t.Helper()
	requireGitCLI(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not in PATH")
	}
	root := t.TempDir()
	mustRunIn(t, root, "git", "init", "--initial-branch=main", "-q")
	mustRunIn(t, root, "git", "config", "user.email", "test@example.com")
	mustRunIn(t, root, "git", "config", "user.name", "Test")
	mustRunIn(t, root, "git", "config", "commit.gpgsign", "false")

	writeRepoFile(t, root, "go.mod", "module example.com/m\n\ngo 1.21\n")
	writeRepoFile(t, root, "lib.go", "package m\n\nfunc Add(a, b int) int { return a + b }\n")
	writeRepoFile(t, root, "lib_test.go", `package m

import "testing"

func TestAdd(t *testing.T) {
	if Add(2, 3) != 5 {
		t.Fatal("bad")
	}
}
`)
	mustRunIn(t, root, "git", "add", ".")
	mustRunIn(t, root, "git", "commit", "-q", "-m", "seed")
	return root
}

func TestE2E_Stage1RejectsBrokenTestAdmitsPassingFix(t *testing.T) {
	repo := initGoRepo(t)
	chdir(t, repo)

	if code := RunEngine([]string{"init"}); code != 0 {
		t.Fatalf("init exit=%d", code)
	}

	// 1. Breaking diff: changes Add(a,b) to return a-b → existing TestAdd fails.
	writeRepoFile(t, repo, "lib.go", "package m\n\nfunc Add(a, b int) int { return a - b }\n")
	breakingDiff := captureIn(t, repo, "git", "diff")
	mustRunIn(t, repo, "git", "checkout", "--", "lib.go")
	breakingPath := filepath.Join(repo, "breaking.diff")
	if err := os.WriteFile(breakingPath, []byte(breakingDiff), 0o644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		code := RunEngine([]string{
			"submit",
			"--diff", breakingPath,
			"--intent", "intent-break",
			"--brief", "deliberately broken",
		})
		if code != 2 {
			t.Fatalf("breaking submit should deny (exit 2), got %d\noutput=%s", code, "")
		}
	})
	if !strings.Contains(out, "build") {
		t.Errorf("expected build in output, got:\n%s", out)
	}
	if !strings.Contains(out, "verdict: DENY") {
		t.Errorf("expected DENY, got:\n%s", out)
	}

	// 2. Passing diff: re-express the intent as a no-op edit (add a comment).
	// Build still passes, TestAdd still passes, Build admits.
	writeRepoFile(t, repo, "lib.go", "package m\n\n// Add returns the sum of two ints.\nfunc Add(a, b int) int { return a + b }\n")
	passingDiff := captureIn(t, repo, "git", "diff")
	mustRunIn(t, repo, "git", "checkout", "--", "lib.go")

	passingPath := filepath.Join(repo, "passing.diff")
	if err := os.WriteFile(passingPath, []byte(passingDiff), 0o644); err != nil {
		t.Fatal(err)
	}

	var out2 string
	out2 = captureStdout(t, func() {
		code := RunEngine([]string{
			"submit",
			"--diff", passingPath,
			"--intent", "intent-pass",
			"--brief", "add Add docstring",
		})
		if code != 0 {
			t.Fatalf("passing submit should admit (exit 0), got %d", code)
		}
	})
	if !strings.Contains(out2, "admitted: ") {
		t.Errorf("expected admitted SHA, got:\n%s", out2)
	}
	if !strings.Contains(out2, "[build]") {
		t.Errorf("expected build policy in output, got:\n%s", out2)
	}
}

func writeRepoFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustRunIn(t *testing.T, dir, name string, args ...string) {
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

func captureIn(t *testing.T, dir, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%s %v: %v", name, args, err)
	}
	return string(out)
}
