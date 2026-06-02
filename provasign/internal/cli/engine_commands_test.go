package cli

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// MVP-L1 closing integration test:
//   - `provasign init` scaffolds .provasign/
//   - 5 sequential `relay submit` calls each produce a signed commit on relay-main
//   - `provasign cert verify` succeeds for every resulting commit
//
// This is the milestone exit criterion from docs/mvp-roadmap.md.

func requireGitCLI(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not in PATH: %v", err)
	}
}

func initGitRepo(t *testing.T) string {
	t.Helper()
	requireGitCLI(t)
	root := t.TempDir()
	cmds := [][]string{
		{"init", "--initial-branch=main", "-q"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
		{"config", "commit.gpgsign", "false"},
	}
	for _, c := range cmds {
		cmd := exec.Command("git", c...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(c, " "), err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("seed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, c := range [][]string{{"add", "README.md"}, {"commit", "-m", "seed", "-q"}} {
		cmd := exec.Command("git", c...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(c, " "), err, out)
		}
	}
	return root
}

// chdir is a helper so RunEngine() can resolve "." correctly.
func chdir(t *testing.T, dir string) {
	t.Helper()
	prev, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
}

// captureStdout replaces os.Stdout for the duration of fn and returns whatever was written.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	done := make(chan struct{})
	var buf bytes.Buffer
	go func() {
		_, _ = buf.ReadFrom(r)
		close(done)
	}()
	fn()
	_ = w.Close()
	os.Stdout = old
	<-done
	return buf.String()
}

// writeDiffFile writes a unified diff that creates file `name` containing `body`
// at the repo root, and returns the diff file path.
func writeDiffFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	diff := fmt.Sprintf(`--- /dev/null
+++ b/%s
@@ -0,0 +1,1 @@
+%s
`, name, body)
	p := filepath.Join(dir, strings.ReplaceAll(name, "/", "_")+".diff")
	if err := os.WriteFile(p, []byte(diff), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// extractCommit pulls the `admitted: <sha>` line from cli output.
func extractCommit(t *testing.T, out string) string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "admitted: ") {
			return strings.TrimPrefix(line, "admitted: ")
		}
	}
	t.Fatalf("no admitted line in output:\n%s", out)
	return ""
}

func TestE2E_FiveSignedCommitsAndVerify(t *testing.T) {
	repo := initGitRepo(t)
	chdir(t, repo)

	// 1. provasign init
	if code := RunEngine([]string{"init"}); code != 0 {
		t.Fatalf("init exit=%d", code)
	}
	if _, err := os.Stat(filepath.Join(repo, ".provasign", "provasign.yaml")); err != nil {
		t.Fatalf("relay.yaml missing: %v", err)
	}

	// 2. Submit 5 changesets sequentially.
	var shas []string
	for i := 1; i <= 5; i++ {
		name := fmt.Sprintf("file%d.txt", i)
		diff := writeDiffFile(t, repo, name, fmt.Sprintf("content-%d", i))
		out := captureStdout(t, func() {
			code := RunEngine([]string{
				"submit",
				"--diff", diff,
				"--intent", fmt.Sprintf("intent-%d", i),
				"--brief", fmt.Sprintf("add %s", name),
				"--author", "test-agent",
			})
			if code != 0 {
				t.Fatalf("submit %d exit=%d output=%s", i, code, "")
			}
		})
		sha := extractCommit(t, out)
		if len(sha) != 40 {
			t.Errorf("bad sha for submit %d: %q", i, sha)
		}
		shas = append(shas, sha)
	}
	if len(shas) != 5 {
		t.Fatalf("expected 5 commits, got %d", len(shas))
	}

	// 3. Verify every certificate.
	for i, sha := range shas {
		out := captureStdout(t, func() {
			code := RunEngine([]string{"cert", "verify", sha})
			if code != 0 {
				t.Fatalf("verify %d exit=%d", i, code)
			}
		})
		if !strings.HasPrefix(out, "verify OK") {
			t.Errorf("verify %d output: %s", i, out)
		}
	}

	// 4. Linear history: relay-main has seed + 5 commits.
	logOut, err := exec.Command("git", "-C", repo, "log", "--format=%H", "relay-main").Output()
	if err != nil {
		t.Fatal(err)
	}
	logLines := strings.Split(strings.TrimSpace(string(logOut)), "\n")
	if len(logLines) != 6 { // 1 seed + 5 admitted
		t.Errorf("expected 6 commits on relay-main, got %d:\n%s", len(logLines), logOut)
	}

	// 5. Re-running init is idempotent (does not overwrite relay.yaml).
	if code := RunEngine([]string{"init"}); code != 0 {
		t.Errorf("re-init exit=%d", code)
	}
}

func TestE2E_CheckRejectsDeniedPath(t *testing.T) {
	repo := initGitRepo(t)
	chdir(t, repo)

	if code := RunEngine([]string{"init"}); code != 0 {
		t.Fatal("init failed")
	}
	diff := writeDiffFile(t, repo, "vendor/x.txt", "blocked")
	out := captureStdout(t, func() {
		code := RunEngine([]string{"check", "--diff", diff})
		if code != 2 {
			t.Errorf("expected exit 2 (deny), got %d", code)
		}
	})
	if !strings.Contains(out, "verdict: DENY") {
		t.Errorf("expected DENY verdict, got:\n%s", out)
	}
	if !strings.Contains(out, "[path]") {
		t.Errorf("expected path gate hit, got:\n%s", out)
	}
}

func TestE2E_CheckJSONOutput(t *testing.T) {
	repo := initGitRepo(t)
	chdir(t, repo)
	if code := RunEngine([]string{"init"}); code != 0 {
		t.Fatal("init failed")
	}
	diff := writeDiffFile(t, repo, "ok.txt", "hi")
	out := captureStdout(t, func() {
		_ = RunEngine([]string{"check", "--diff", diff, "--json"})
	})
	if !strings.Contains(out, `"Policies":`) && !strings.Contains(out, `"policies":`) {
		t.Errorf("expected json keys, got: %s", out)
	}
}

func TestRunEngine_UnknownCommand(t *testing.T) {
	code := RunEngine([]string{"bogus"})
	if code != 1 {
		t.Errorf("expected exit 1, got %d", code)
	}
}

func TestRunEngine_NoArgs(t *testing.T) {
	if code := RunEngine(nil); code != 1 {
		t.Errorf("expected exit 1, got %d", code)
	}
}

func TestCmdSubmit_MissingDiffFlag(t *testing.T) {
	repo := initGitRepo(t)
	chdir(t, repo)
	_ = RunEngine([]string{"init"})
	if code := RunEngine([]string{"submit"}); code != 1 {
		t.Errorf("expected exit 1, got %d", code)
	}
}

func TestCmdCert_RequiresSubcommand(t *testing.T) {
	if code := RunEngine([]string{"cert"}); code != 1 {
		t.Errorf("expected exit 1, got %d", code)
	}
	if code := RunEngine([]string{"cert", "verify"}); code != 1 {
		t.Errorf("expected exit 1 (missing sha), got %d", code)
	}
}
