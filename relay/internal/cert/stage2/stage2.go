// Package stage2 runs all configured static-analysis adapters in parallel
// against an ephemeral worktree and aggregates their findings into a single
// Stage2Result. The result feeds the secrets/fileclass/deps policy gates.
package stage2

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tabladrum/grove-suite/relay/internal/analyzers"
	"github.com/tabladrum/grove-suite/relay/internal/cert"
	"github.com/tabladrum/grove-suite/relay/internal/core"
)

// Stage2 holds the configured analyzer adapters.
type Stage2 struct {
	Analyzers []analyzers.Analyzer
	// MaxParallel caps concurrent analyzer execution. Zero means unbounded
	// (one goroutine per analyzer).
	MaxParallel int
	// Scope, when ScopeNew, drops findings whose (Path, Line) is not in
	// the diff's added-line ranges. ScopeAll (default) reports everything.
	// Applied uniformly across every analyzer's findings.
	Scope core.ScanScope
	// SeverityFloors maps analyzer name → minimum severity to keep. Empty
	// means "no floor". Applied per-analyzer before scope filtering so a
	// noisy linter can be muted to high-only without affecting others.
	SeverityFloors map[string]cert.Severity
	// ScopeOverrides maps analyzer name → per-analyzer ScanScope that
	// overrides Stage2.Scope for that analyzer's findings only.
	ScopeOverrides map[string]core.ScanScope
}

// New constructs a Stage2 with the given analyzers.
func New(a ...analyzers.Analyzer) *Stage2 {
	return &Stage2{Analyzers: a}
}

// Run executes every available analyzer in parallel against cs's worktree
// and returns the aggregated result. Like Stage1, it creates its own
// ephemeral `git worktree` so analyzers see the post-diff state without
// touching the developer's checkout.
func (s *Stage2) Run(ctx context.Context, cs *core.ChangeSet) (cert.Stage2Result, error) {
	if cs == nil {
		return cert.Stage2Result{}, fmt.Errorf("stage2: nil changeset")
	}
	if cs.RepoRoot == "" {
		return cert.Stage2Result{}, fmt.Errorf("stage2: empty RepoRoot")
	}
	if _, err := os.Stat(filepath.Join(cs.RepoRoot, ".git")); err != nil {
		return cert.Stage2Result{}, fmt.Errorf("stage2: %s is not a git repo: %w", cs.RepoRoot, err)
	}
	if len(s.Analyzers) == 0 {
		return cert.Stage2Result{
			Skipped:    true,
			SkipReason: "no analyzers configured",
			StartedAt:  time.Now().UTC(),
		}, nil
	}

	wt, cleanup, err := addWorktree(ctx, cs)
	if err != nil {
		return cert.Stage2Result{}, fmt.Errorf("stage2: add worktree: %w", err)
	}
	defer cleanup()

	if strings.TrimSpace(cs.Diff) != "" {
		if err := applyDiff(ctx, wt, cs.Diff); err != nil {
			return cert.Stage2Result{}, fmt.Errorf("stage2: apply diff: %w", err)
		}
	}

	res := cert.Stage2Result{StartedAt: time.Now().UTC()}
	defer func() { res.Duration = time.Since(res.StartedAt) }()

	type analyzerOut struct {
		run      cert.AnalyzerRun
		findings []cert.Finding
	}

	results := make([]analyzerOut, len(s.Analyzers))
	var wg sync.WaitGroup
	var sem chan struct{}
	if s.MaxParallel > 0 {
		sem = make(chan struct{}, s.MaxParallel)
	}

	for i, a := range s.Analyzers {
		i, a := i, a
		wg.Add(1)
		go func() {
			defer wg.Done()
			if sem != nil {
				sem <- struct{}{}
				defer func() { <-sem }()
			}
			start := time.Now()
			out := analyzerOut{run: cert.AnalyzerRun{Name: a.Name(), Available: a.Available()}}
			if !out.run.Available {
				out.run.SkipReason = "binary not in PATH"
				out.run.Duration = time.Since(start)
				results[i] = out
				return
			}
			findings, err := a.Run(ctx, cs, wt)
			out.run.Duration = time.Since(start)
			if err != nil {
				out.run.ErrorMsg = err.Error()
				results[i] = out
				return
			}
			out.findings = findings
			out.run.NumFindings = len(findings)
			results[i] = out
		}()
	}
	wg.Wait()

	var added core.AddedLineMap
	if s.Scope == core.ScopeNew || len(s.ScopeOverrides) > 0 {
		added = core.ParseAddedLines(cs.Diff)
	}

	for i, r := range results {
		findings := r.findings
		if floor, ok := s.SeverityFloors[r.run.Name]; ok && floor != "" {
			findings = filterFloor(findings, floor)
		}
		scope := s.Scope
		if ov, ok := s.ScopeOverrides[r.run.Name]; ok && ov != "" {
			scope = ov
		}
		if scope == core.ScopeNew && added != nil {
			findings = filterNewCode(findings, added)
		}
		results[i].findings = findings
		results[i].run.NumFindings = len(findings)
	}

	for _, r := range results {
		res.Runs = append(res.Runs, r.run)
		res.Findings = append(res.Findings, r.findings...)
	}
	res.Findings = cert.SortFindings(res.Findings)
	res.Duration = time.Since(res.StartedAt)

	// If no analyzer was available, mark the whole stage as skipped so the
	// engine treats it as non-blocking (developer needs `relay tools install`).
	availableCount := 0
	for _, r := range res.Runs {
		if r.Available {
			availableCount++
		}
	}
	if availableCount == 0 {
		res.Skipped = true
		res.SkipReason = "no analyzer binaries available; run `relay tools install`"
	}
	return res, nil
}

func addWorktree(ctx context.Context, cs *core.ChangeSet) (string, func(), error) {
	tmp, err := os.MkdirTemp("", "relay-stage2-*")
	if err != nil {
		return "", nil, err
	}
	wt := filepath.Join(tmp, "wt")
	base := strings.TrimSpace(cs.BaseSHA)
	if base == "" {
		base = "HEAD"
	}
	out, err := runGit(ctx, cs.RepoRoot, "worktree", "add", "--detach", wt, base)
	if err != nil {
		_ = os.RemoveAll(tmp)
		return "", nil, fmt.Errorf("git worktree add: %w (%s)", err, out)
	}
	cleanup := func() {
		_, _ = runGit(context.Background(), cs.RepoRoot, "worktree", "remove", "--force", wt)
		_ = os.RemoveAll(tmp)
	}
	return wt, cleanup, nil
}

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
