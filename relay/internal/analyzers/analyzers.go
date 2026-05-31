// Package analyzers defines the contract every Stage-2 static-analysis
// adapter implements. Concrete adapters live in subpackages
// (semgrep, gitleaks, govulncheck) and are wired into the aggregator
// (cert/stage2) by the engine.
package analyzers

import (
	"context"

	"github.com/tabladrum/grove-suite/relay/internal/cert"
	"github.com/tabladrum/grove-suite/relay/internal/core"
)

// Analyzer is one external static-analysis tool wrapped behind a stable
// interface. Implementations must be safe to invoke concurrently and must
// honor ctx cancellation.
type Analyzer interface {
	// Name is the stable identifier used in AnalyzerRun.Name and as the
	// `Analyzer` field on every emitted Finding.
	Name() string

	// Available reports whether this analyzer can run on the host. The
	// aggregator records unavailable analyzers as skipped rather than
	// treating absence as failure. Implementations typically `exec.LookPath`
	// the underlying binary.
	Available() bool

	// Run executes the analyzer against the worktree at dir (which already
	// has cs.Diff applied) and returns normalized findings. The ChangeSet is
	// also passed so diff-aware analyzers (e.g. inline-secrets) can focus on
	// added lines without re-deriving them from the worktree.
	// Returning an error indicates the analyzer itself failed to execute; a
	// clean run with zero findings returns (nil, nil).
	Run(ctx context.Context, cs *core.ChangeSet, dir string) ([]cert.Finding, error)
}
