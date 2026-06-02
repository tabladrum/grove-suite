package coverage

// Package coverage implements the coverage-of-changed-symbols policy gate.
// In MVP-L2 this is intentionally minimal: it enforces an optional minimum
// aggregate coverage threshold and recognizes the `// relay:no-test-required`
// escape-hatch comment on changed files.
//
// Future work (MVP-L3+): correlate against Grove `tests` edges to compute
// per-changed-symbol coverage rather than aggregate %.

import (
	"context"
	"fmt"
	"strings"

	"github.com/provasign/provasign/internal/cert"
	"github.com/provasign/provasign/internal/core"
)

// EscapeHatchMarker is the comment that disables the coverage gate for a
// specific change. Any added line (`+...`) in the diff containing this
// marker exempts the entire ChangeSet.
const EscapeHatchMarker = "relay:no-test-required"

// Gate enforces a minimum aggregate coverage from a BuildResult.
type Gate struct {
	// Build supplies the most recent BuildResult; the engine sets it
	// before policy evaluation so the gate can read it without re-running.
	Build func() *cert.BuildResult
}

// Name implements policy.Gate.
func (g *Gate) Name() string { return "coverage" }

// Evaluate implements policy.Gate.
func (g *Gate) Evaluate(_ context.Context, cs *core.ChangeSet, opts map[string]any) core.PolicyResult {
	res := core.PolicyResult{Gate: g.Name(), Verdict: core.VerdictAllow}

	if g.Build == nil || g.Build() == nil {
		// No Build result available — fail closed to avoid false positives.
		res.Verdict = core.VerdictReviewRequired
		res.Message = "coverage gate: no Build result available"
		return res
	}
	s := g.Build()

	if s.Skipped {
		res.Message = "coverage gate: skipped (" + s.SkipReason + ")"
		return res
	}

	if hasEscapeHatch(cs.Diff) {
		res.Message = fmt.Sprintf("coverage gate: skipped (%s)", EscapeHatchMarker)
		return res
	}

	min := floatOption(opts, "min_coverage", 80)
	if s.Tests.CoveragePct < min {
		res.Verdict = core.VerdictDeny
		res.Message = fmt.Sprintf(
			"coverage %.1f%% below minimum %.1f%% — add tests or annotate with `// %s`",
			s.Tests.CoveragePct, min, EscapeHatchMarker,
		)
		return res
	}
	res.Message = fmt.Sprintf("coverage %.1f%% (min %.1f%%)", s.Tests.CoveragePct, min)
	return res
}

// hasEscapeHatch reports whether any *added* diff line contains the marker.
// Removed lines are ignored so a deletion cannot accidentally enable bypass.
func hasEscapeHatch(diff string) bool {
	for _, line := range strings.Split(diff, "\n") {
		if !strings.HasPrefix(line, "+") || strings.HasPrefix(line, "+++ ") {
			continue
		}
		if strings.Contains(line, EscapeHatchMarker) {
			return true
		}
	}
	return false
}

func floatOption(opts map[string]any, key string, def float64) float64 {
	if opts == nil {
		return def
	}
	v, ok := opts[key]
	if !ok {
		return def
	}
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int64:
		return float64(x)
	default:
		return def
	}
}
