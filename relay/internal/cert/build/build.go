package build

// Package stage1 orchestrates the build + test stage of certification.
// It applies a ChangeSet's diff into an ephemeral git worktree, runs the
// project build, executes a language runner, and returns a BuildResult.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/tabladrum/grove-suite/relay/internal/cert"
	"github.com/tabladrum/grove-suite/relay/internal/core"
)

// Runner runs a project's test suite in dir and returns a TestRun.
// Implemented by language-specific runners (gotest, npmtest, pytest, ...).
type Runner interface {
	Run(ctx context.Context, dir string) (cert.TestRun, error)
}

// Builder runs the project's build in dir. The default implementation runs
// `go build ./...`; alternative builders can be plugged in per stack.
type Builder interface {
	Build(ctx context.Context, dir string) (output string, err error)
}

// GoBuilder runs `go build ./...`.
type GoBuilder struct{}

// Build implements Builder.
func (GoBuilder) Build(ctx context.Context, dir string) (string, error) {
	cmd := exec.CommandContext(ctx, "go", "build", "./...")
	cmd.Dir = dir
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

// Build wires a Builder + Runner against an isolated copy of the repo.
type Build struct {
	Builder Builder
	Runner  Runner
	// Detector, when set, decides whether the worktree has an applicable
	// language runner. It returns (matched-name, true) when something is
	// detected, or ("", false) to short-circuit Build as Skipped. When
	// nil, Build falls back to "does go.mod exist?" for backward
	// compatibility with single-runner gotest wiring.
	Detector func(dir string) (string, bool)
	// MaxOutputBytes truncates BuildOutput. Defaults to 64 KiB if zero.
	MaxOutputBytes int
}

// New returns a Build wired with GoBuilder + the given runner.
func New(runner Runner) *Build {
	return &Build{
		Builder:        GoBuilder{},
		Runner:         runner,
		MaxOutputBytes: 64 << 10,
	}
}

// Run executes the stage against cs: clones cs.RepoRoot into an ephemeral
// worktree, applies cs.Diff there, runs build then tests, and returns the
// aggregated result. The worktree is removed before Run returns.
func (s *Build) Run(ctx context.Context, cs *core.ChangeSet) (cert.BuildResult, error) {
	if cs == nil {
		return cert.BuildResult{}, fmt.Errorf("build: nil changeset")
	}
	if cs.RepoRoot == "" {
		return cert.BuildResult{}, fmt.Errorf("build: empty RepoRoot")
	}
	if _, err := os.Stat(filepath.Join(cs.RepoRoot, ".git")); err != nil {
		return cert.BuildResult{}, fmt.Errorf("build: %s is not a git repo: %w", cs.RepoRoot, err)
	}

	wt, cleanup, err := addWorktree(ctx, cs)
	if err != nil {
		return cert.BuildResult{}, fmt.Errorf("build: add worktree: %w", err)
	}
	defer cleanup()

	if strings.TrimSpace(cs.Diff) != "" {
		if err := applyDiff(ctx, wt, cs.Diff); err != nil {
			return cert.BuildResult{}, fmt.Errorf("build: apply diff: %w", err)
		}
	}

	// Detection: skip cleanly when no language runner detects the worktree.
	// A Detector callback (set by multilang dispatcher consumers) is the
	// primary signal. Legacy go.mod fallback is kept for direct
	// build.New(gotest.New()) users.
	if s.Detector != nil {
		if name, ok := s.Detector(wt); !ok {
			reason := "no language detected; Stage 1 has no applicable runner"
			if name != "" {
				reason = "detector returned no match (" + name + ")"
			}
			return cert.BuildResult{
				Skipped:    true,
				SkipReason: reason,
				BuildOk:    true,
			}, nil
		}
	} else if _, err := os.Stat(filepath.Join(wt, "go.mod")); err != nil {
		return cert.BuildResult{
			Skipped:    true,
			SkipReason: "no go.mod found; Stage 1 has no applicable runner",
			BuildOk:    true,
		}, nil
	}

	res := cert.BuildResult{}

	if s.Builder != nil {
		bStart := time.Now()
		bOut, bErr := s.Builder.Build(ctx, wt)
		res.BuildDuration = time.Since(bStart)
		res.BuildOutput = truncate(bOut, s.MaxOutputBytes)
		res.BuildOk = bErr == nil
		if bErr != nil {
			// Skip tests when build is broken; nothing meaningful to run.
			return res, nil
		}
	} else {
		// No builder: the language runner is expected to handle build+test
		// in a single invocation (pytest, jest, mvn test, ...).
		res.BuildOk = true
	}

	tr, terr := s.Runner.Run(ctx, wt)
	if terr != nil {
		return res, fmt.Errorf("build: runner: %w", terr)
	}
	res.Tests = tr
	return res, nil
}

// addWorktree creates a temporary git worktree of cs.RepoRoot at cs.BaseSHA
// (or HEAD if BaseSHA is empty) and returns its path plus a cleanup func.
func addWorktree(ctx context.Context, cs *core.ChangeSet) (string, func(), error) {
	tmp, err := os.MkdirTemp("", "relay-build-*")
	if err != nil {
		return "", nil, err
	}
	wt := filepath.Join(tmp, "wt")

	base := strings.TrimSpace(cs.BaseSHA)
	if base == "" {
		base = "HEAD"
	}
	// Detached worktree to avoid colliding with branch names.
	out, err := runGit(ctx, cs.RepoRoot, "worktree", "add", "--detach", wt, base)
	if err != nil {
		_ = os.RemoveAll(tmp)
		return "", nil, fmt.Errorf("git worktree add: %w (%s)", err, out)
	}
	cleanup := func() {
		// Best-effort; ignore errors since we always remove the tmp dir too.
		_, _ = runGit(context.Background(), cs.RepoRoot, "worktree", "remove", "--force", wt)
		_ = os.RemoveAll(tmp)
	}
	return wt, cleanup, nil
}

// applyDiff applies a unified diff to the working tree at wt.
// Uses --index so renames and mode changes are honored.
func applyDiff(ctx context.Context, wt, diff string) error {
	cmd := exec.CommandContext(ctx, "git", "apply", "--index", "--whitespace=nowarn", "-")
	cmd.Dir = wt
	cmd.Stdin = strings.NewReader(diff)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %s", err, buf.String())
	}
	return nil
}

func runGit(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

// FingerprintDiff returns a short stable hash of a diff. Useful for naming
// ephemeral worktree branches when isolation is desired beyond the tempdir.
func FingerprintDiff(diff string) string {
	sum := sha256.Sum256([]byte(diff))
	return hex.EncodeToString(sum[:8])
}

func truncate(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "\n...(truncated)"
}
