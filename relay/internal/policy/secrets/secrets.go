// Package secrets implements the Stage-2 secrets policy gate. It denies
// admission when any Stage-2 finding has Category=secret. The gate is
// always-on (no opts required) because leaked credentials are a hard
// failure with no legitimate bypass.
package secrets

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/tabladrum/grove-suite/relay/internal/cert"
	"github.com/tabladrum/grove-suite/relay/internal/core"
)

// Gate denies on any Category=secret finding produced by Stage 2.
type Gate struct {
	// Stage2 supplies the most recent Stage2Result; the engine sets it
	// before policy evaluation.
	Stage2 func() *cert.Stage2Result
}

// Name implements policy.Gate.
func (g *Gate) Name() string { return "secrets" }

// Evaluate implements policy.Gate.
func (g *Gate) Evaluate(_ context.Context, _ *core.ChangeSet, _ map[string]any) core.PolicyResult {
	res := core.PolicyResult{Gate: g.Name(), Verdict: core.VerdictAllow}

	if g.Stage2 == nil || g.Stage2() == nil {
		res.Verdict = core.VerdictReviewRequired
		res.Message = "secrets gate: no Stage2 result available"
		return res
	}
	s := g.Stage2()
	if s.Skipped {
		res.Verdict = core.VerdictReviewRequired
		res.Message = "secrets gate: Stage 2 skipped (" + s.SkipReason + ")"
		res.NextAction = "install a secrets scanner (`relay tools install`) and re-submit"
		return res
	}

	secretsFound := s.FindingsByCategory()[cert.CategorySecret]
	if len(secretsFound) == 0 {
		res.Message = "no secrets detected"
		return res
	}

	sort.SliceStable(secretsFound, func(i, j int) bool {
		if secretsFound[i].Path != secretsFound[j].Path {
			return secretsFound[i].Path < secretsFound[j].Path
		}
		return secretsFound[i].Line < secretsFound[j].Line
	})

	var b strings.Builder
	fmt.Fprintf(&b, "%d secret(s) detected:", len(secretsFound))
	limit := 5
	for i, f := range secretsFound {
		if i >= limit {
			fmt.Fprintf(&b, " (+%d more)", len(secretsFound)-limit)
			break
		}
		fmt.Fprintf(&b, " [%s] %s:%d %s;", f.Analyzer, f.Path, f.Line, f.RuleID)
	}
	res.Verdict = core.VerdictDeny
	res.Message = b.String()
	res.NextAction = "remove the secret from the diff, rotate the credential at its provider, and re-submit"
	res.Details = map[string]any{"count": len(secretsFound)}
	return res
}
