package admission

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tabladrum/grove-suite/relay/internal/core"
)

// requireGit skips the test when git is unavailable in PATH.
// (CI on all three OSes always has git, but local sandboxes might not.)
func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git not available: %v", err)
	}
}

func initRepo(t *testing.T) string {
	t.Helper()
	requireGit(t)
	root := t.TempDir()
	for _, args := range [][]string{
		{"init", "--initial-branch=main", "-q"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
		{"config", "commit.gpgsign", "false"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v (%s)", strings.Join(args, " "), err, out)
		}
	}
	// Seed commit so HEAD exists.
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("seed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"add", "README.md"},
		{"commit", "-m", "seed", "-q"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v (%s)", strings.Join(args, " "), err, out)
		}
	}
	return root
}

func TestGitAdmitter_CommitsOnRelayMain(t *testing.T) {
	repo := initRepo(t)
	a := NewGitAdmitter()
	cs := &core.ChangeSet{
		ID:          "cs-1",
		IntentID:    "intent-1",
		IntentBrief: "add hello file",
		RepoRoot:    repo,
		Author:      "test-agent",
		Diff: `--- /dev/null
+++ b/hello.txt
@@ -0,0 +1 @@
+hello world
`,
	}
	cert := &core.Certificate{
		ID:                  "cert-1",
		EffectiveConfigHash: "cfg-hash",
		SignedBy:            "local-ed25519:abc",
		Signature:           []byte{0x01, 0x02, 0x03},
		ICR:                 core.ICR{Hash: "icrhash"},
	}
	sha, err := a.Admit(context.Background(), cs, cert)
	if err != nil {
		t.Fatalf("admit: %v", err)
	}
	if len(sha) != 40 {
		t.Errorf("expected 40-char sha, got %q", sha)
	}

	// Verify relay-main exists and points at sha.
	branchSHA, err := captureGit(context.Background(), repo, "rev-parse", "relay-main")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(branchSHA) != sha {
		t.Errorf("branch sha mismatch: %s vs %s", branchSHA, sha)
	}

	// Verify file was committed.
	if _, err := os.Stat(filepath.Join(repo, "hello.txt")); err != nil {
		t.Errorf("hello.txt not present: %v", err)
	}

	// Verify trailers are in the commit message.
	msg, err := captureGit(context.Background(), repo, "log", "-1", "--format=%B", sha)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"add hello file",
		"Intent-ID: intent-1",
		"ChangeSet-ID: cs-1",
		"Certificate-ID: cert-1",
		"ICR-Hash: icrhash",
		"Effective-Config-Hash: cfg-hash",
		"Signed-By: local-ed25519:abc",
		"Signature: " + base64.StdEncoding.EncodeToString([]byte{0x01, 0x02, 0x03}),
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("commit msg missing %q\n--full--\n%s", want, msg)
		}
	}
}

func TestGitAdmitter_FailsOnMissingRepo(t *testing.T) {
	requireGit(t)
	a := NewGitAdmitter()
	cs := &core.ChangeSet{RepoRoot: t.TempDir(), Diff: "irrelevant"}
	_, err := a.Admit(context.Background(), cs, &core.Certificate{})
	if err == nil {
		t.Fatal("expected error for non-git directory")
	}
}

func TestGitAdmitter_FailsOnBadDiff(t *testing.T) {
	repo := initRepo(t)
	a := NewGitAdmitter()
	cs := &core.ChangeSet{RepoRoot: repo, Diff: "not a diff at all\n"}
	_, err := a.Admit(context.Background(), cs, &core.Certificate{})
	if err == nil {
		t.Fatal("expected git apply error")
	}
	var _ error = err // exercise the wrapped-error type minimally
	if !strings.Contains(err.Error(), "git apply") {
		t.Errorf("expected git apply error, got %v", err)
	}
}

func TestGitAdmitter_EmptyRepoRoot(t *testing.T) {
	a := NewGitAdmitter()
	_, err := a.Admit(context.Background(), &core.ChangeSet{}, &core.Certificate{})
	if err == nil || !strings.Contains(err.Error(), "RepoRoot") {
		t.Errorf("expected empty RepoRoot error, got %v", err)
	}
}

func TestBuildCommitMessage_Defaults(t *testing.T) {
	cs := &core.ChangeSet{ID: "cs-x"}
	cert := &core.Certificate{ID: "cert-x"}
	msg := buildCommitMessage(cs, cert)
	if !strings.HasPrefix(msg, "relay: admit cs-x\n") {
		t.Errorf("default subject: %q", msg)
	}
}

func TestNonEmpty(t *testing.T) {
	if nonEmpty("a", "b") != "a" || nonEmpty("", "b") != "b" {
		t.Errorf("nonEmpty broken")
	}
}

// Sanity: errors.Is still works on the wrapped Admit error.
func TestAdmit_ErrorWrappingPreservesIs(t *testing.T) {
	repo := initRepo(t)
	a := NewGitAdmitter()
	a.Branch = "" // exercise default branch path
	cs := &core.ChangeSet{RepoRoot: repo, Diff: "not a diff\n"}
	_, err := a.Admit(context.Background(), cs, &core.Certificate{})
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Errorf("did not expect os.ErrNotExist, but errors.Is returned true")
	}
}
