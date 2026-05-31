// Package multilang dispatches Stage 1 testing to the first language
// runner that detects the target directory. Order matters: callers
// pass runners in priority order. A nil result with Runner="multilang"
// and zero counts indicates "no detector matched" — Stage 1 treats
// that as a structural skip.
package multilang

import (
	"context"
	"time"

	"github.com/tabladrum/grove-suite/relay/internal/cert"
)

// LangRunner is the contract for one detectable language runner.
type LangRunner interface {
	Name() string
	Detect(dir string) bool
	Run(ctx context.Context, dir string) (cert.TestRun, error)
}

// Runner dispatches to the first matching LangRunner.
type Runner struct {
	Runners []LangRunner
}

// New builds a Runner from a priority-ordered list.
func New(runners ...LangRunner) *Runner { return &Runner{Runners: runners} }

// Run finds the first LangRunner whose Detect returns true and delegates.
// If nothing matches, returns an empty TestRun with Runner="multilang"
// — the caller (stage1.Stage1) interprets that as "no applicable runner".
func (r *Runner) Run(ctx context.Context, dir string) (cert.TestRun, error) {
	for _, lr := range r.Runners {
		if lr.Detect(dir) {
			return lr.Run(ctx, dir)
		}
	}
	return cert.TestRun{Runner: "multilang", StartedAt: time.Now().UTC()}, nil
}

// MatchedName returns the Name() of the LangRunner whose Detect matches
// dir, or empty string if none. Useful for logging and tests.
func (r *Runner) MatchedName(dir string) string {
	for _, lr := range r.Runners {
		if lr.Detect(dir) {
			return lr.Name()
		}
	}
	return ""
}
