package cli

// MVP-L3 closing integration test: Stage 2 standalone static analysis.
//
// Verifies:
//   - A diff containing a leaked AWS access key is denied by the secrets
//     gate, with NextAction guidance. (Uses the always-available
//     inline-secrets analyzer, so the test is hermetic and does not
//     require gitleaks/semgrep/govulncheck binaries.)
//   - A clean diff still admits.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestE2E_Stage2SecretsDenyThenCleanAdmits(t *testing.T) {
	repo := initGoRepo(t)
	chdir(t, repo)

	if code := RunEngine([]string{"init"}); code != 0 {
		t.Fatalf("init exit=%d", code)
	}

	// 1. Diff that adds a literal-looking AWS Access Key ID.
	leakSrc := `package m

const awsKey = "AKIAIOSFODNN7EXAMPLE"

// Add returns the sum of two ints.
func Add(a, b int) int { return a + b }
`
	writeRepoFile(t, repo, "lib.go", leakSrc)
	leakDiff := captureIn(t, repo, "git", "diff")
	mustRunIn(t, repo, "git", "checkout", "--", "lib.go")
	leakPath := filepath.Join(repo, "leak.diff")
	if err := os.WriteFile(leakPath, []byte(leakDiff), 0o644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		code := RunEngine([]string{
			"submit",
			"--diff", leakPath,
			"--intent", "intent-leak",
			"--brief", "accidentally added a key",
		})
		if code != 2 {
			t.Fatalf("leaking submit should deny (exit 2), got %d", code)
		}
	})
	if !strings.Contains(out, "verdict: DENY") {
		t.Fatalf("expected DENY, got:\n%s", out)
	}
	if !strings.Contains(out, "[secrets]") {
		t.Fatalf("expected [secrets] gate in output, got:\n%s", out)
	}
	if !strings.Contains(strings.ToLower(out), "rotate") {
		t.Fatalf("expected rotation guidance, got:\n%s", out)
	}

	// 2. Clean docstring-only diff is admitted (Stage 1 passes, Stage 2
	//    has zero secrets findings).
	cleanSrc := "package m\n\n// Add returns the sum of two ints.\nfunc Add(a, b int) int { return a + b }\n"
	writeRepoFile(t, repo, "lib.go", cleanSrc)
	cleanDiff := captureIn(t, repo, "git", "diff")
	mustRunIn(t, repo, "git", "checkout", "--", "lib.go")
	cleanPath := filepath.Join(repo, "clean.diff")
	if err := os.WriteFile(cleanPath, []byte(cleanDiff), 0o644); err != nil {
		t.Fatal(err)
	}

	out2 := captureStdout(t, func() {
		code := RunEngine([]string{
			"submit",
			"--diff", cleanPath,
			"--intent", "intent-clean",
			"--brief", "add docstring",
		})
		if code != 0 {
			t.Fatalf("clean submit should admit (exit 0), got %d\n%s", code, "")
		}
	})
	if !strings.Contains(out2, "admitted: ") {
		t.Fatalf("expected admitted SHA, got:\n%s", out2)
	}
	if !strings.Contains(out2, "[secrets]") {
		t.Fatalf("expected [secrets] gate to have run, got:\n%s", out2)
	}
}

func TestE2E_Stage2FileclassDeniesPemFile(t *testing.T) {
	repo := initGoRepo(t)
	chdir(t, repo)
	if code := RunEngine([]string{"init"}); code != 0 {
		t.Fatalf("init exit=%d", code)
	}

	// Add a file with a forbidden .pem extension.
	writeRepoFile(t, repo, "ssh_host_key.pem", "-----BEGIN OPENSSH PRIVATE KEY-----\nfake\n-----END OPENSSH PRIVATE KEY-----\n")
	pemDiff := captureIn(t, repo, "git", "add", "-N", "ssh_host_key.pem")
	_ = pemDiff
	// Now actually capture the diff with the staged-new file as added lines.
	diff := captureIn(t, repo, "git", "diff", "HEAD")
	mustRunIn(t, repo, "git", "reset", "HEAD", "ssh_host_key.pem")
	if err := os.Remove(filepath.Join(repo, "ssh_host_key.pem")); err != nil {
		t.Fatal(err)
	}
	diffPath := filepath.Join(repo, "pem.diff")
	if err := os.WriteFile(diffPath, []byte(diff), 0o644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		code := RunEngine([]string{
			"submit",
			"--diff", diffPath,
			"--intent", "intent-pem",
			"--brief", "add ssh key",
		})
		if code != 2 {
			t.Fatalf("pem submit should deny (exit 2), got %d", code)
		}
	})
	if !strings.Contains(out, "verdict: DENY") {
		t.Fatalf("expected DENY, got:\n%s", out)
	}
	// Either fileclass OR secrets (inline-secrets catches the PEM header) will fire;
	// both are valid blocks. Verify at least one fired with a forbidden marker.
	if !strings.Contains(out, "[fileclass]") && !strings.Contains(out, "[secrets]") {
		t.Fatalf("expected fileclass or secrets gate to deny, got:\n%s", out)
	}
}
