// Package admission writes admitted ChangeSets onto the linear `relay-main`
// branch as signed commits with full provenance trailers.
package admission

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tabladrum/grove-suite/relay/internal/core"
)

// GitAdmitter applies a unified diff and produces a commit on `relay-main`.
// All git operations run with cwd = ChangeSet.RepoRoot. The committer identity
// defaults to "Relay <relay@local>" and may be overridden.
type GitAdmitter struct {
	Branch         string // default "relay-main"
	CommitterName  string // default "Relay"
	CommitterEmail string // default "relay@local"
}

// NewGitAdmitter returns a GitAdmitter with sensible defaults.
func NewGitAdmitter() *GitAdmitter {
	return &GitAdmitter{
		Branch:         "relay-main",
		CommitterName:  "Relay",
		CommitterEmail: "relay@local",
	}
}

// Admit checks out the target branch, applies the patch, commits with
// provenance trailers, and returns the new commit SHA.
//
// Pre-conditions:
//   - cs.RepoRoot is an existing git repo
//   - cs.Diff is a valid unified diff applicable from current branch HEAD
//
// Failure leaves the working tree in whatever state `git apply` produced;
// callers expecting a clean abort should wrap in a tempdir clone.
func (g *GitAdmitter) Admit(ctx context.Context, cs *core.ChangeSet, cert *core.Certificate) (string, error) {
	branch := g.Branch
	if branch == "" {
		branch = "relay-main"
	}
	repo := cs.RepoRoot
	if repo == "" {
		return "", fmt.Errorf("admit: empty RepoRoot")
	}
	if _, err := os.Stat(filepath.Join(repo, ".git")); err != nil {
		return "", fmt.Errorf("admit: %s is not a git repo: %w", repo, err)
	}

	// Ensure target branch exists; create from current HEAD if missing.
	if err := ensureBranch(ctx, repo, branch); err != nil {
		return "", err
	}
	if err := runGit(ctx, repo, nil, "checkout", branch); err != nil {
		return "", fmt.Errorf("checkout %s: %w", branch, err)
	}

	// Apply the patch directly to the index.
	if err := runGit(ctx, repo, []byte(cs.Diff), "apply", "--index", "--whitespace=nowarn", "-"); err != nil {
		return "", fmt.Errorf("git apply: %w", err)
	}

	msg := buildCommitMessage(cs, cert)
	env := []string{
		"GIT_AUTHOR_NAME=" + nonEmpty(cs.Author, g.CommitterName),
		"GIT_AUTHOR_EMAIL=" + g.CommitterEmail,
		"GIT_COMMITTER_NAME=" + g.CommitterName,
		"GIT_COMMITTER_EMAIL=" + g.CommitterEmail,
	}
	if err := runGitEnv(ctx, repo, env, nil, "commit", "-m", msg); err != nil {
		return "", fmt.Errorf("git commit: %w", err)
	}

	out, err := captureGit(ctx, repo, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("rev-parse: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// buildCommitMessage formats the linear-main commit message with provenance
// trailers. The trailers are stable, parseable key:value pairs so a verifier
// can reconstruct the certificate lookup key from any commit.
func buildCommitMessage(cs *core.ChangeSet, cert *core.Certificate) string {
	subject := cs.IntentBrief
	if subject == "" {
		subject = "relay: admit " + cs.ID
	}
	var b bytes.Buffer
	b.WriteString(subject)
	b.WriteString("\n\n")
	if cs.IntentID != "" {
		fmt.Fprintf(&b, "Intent-ID: %s\n", cs.IntentID)
	}
	fmt.Fprintf(&b, "ChangeSet-ID: %s\n", cs.ID)
	fmt.Fprintf(&b, "Certificate-ID: %s\n", cert.ID)
	fmt.Fprintf(&b, "ICR-Hash: %s\n", cert.ICR.Hash)
	fmt.Fprintf(&b, "Effective-Config-Hash: %s\n", cert.EffectiveConfigHash)
	fmt.Fprintf(&b, "Signed-By: %s\n", cert.SignedBy)
	fmt.Fprintf(&b, "Signature: %s\n", encodeSig(cert.Signature))
	return b.String()
}

func encodeSig(sig []byte) string {
	// Base64 is round-trippable; hex prefix is for human-scan friendliness.
	hexStr := hex.EncodeToString(sig)
	if len(hexStr) > 16 {
		hexStr = hexStr[:16] + "..."
	}
	return base64.StdEncoding.EncodeToString(sig) + " (" + hexStr + ")"
}

func ensureBranch(ctx context.Context, repo, branch string) error {
	// `git rev-parse --verify <branch>` exits 0 when branch exists.
	if err := runGit(ctx, repo, nil, "rev-parse", "--verify", branch); err == nil {
		return nil
	}
	// Create from current HEAD; if repo has zero commits this will fail and the
	// caller is expected to set up a base commit first.
	return runGit(ctx, repo, nil, "branch", branch)
}

func runGit(ctx context.Context, repo string, stdin []byte, args ...string) error {
	return runGitEnv(ctx, repo, nil, stdin, args...)
}

func runGitEnv(ctx context.Context, repo string, env []string, stdin []byte, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repo
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w (%s)", strings.Join(append([]string{"git"}, args...), " "), err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func captureGit(ctx context.Context, repo string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repo
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func nonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
