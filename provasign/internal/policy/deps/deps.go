// Package deps implements the Stage-2 dependency-vulnerability policy gate.
// It maps Stage-2 findings with Category=vuln to verdicts:
//
//	Critical → Deny
//	High     → ReviewRequired (configurable: deny_high=true → Deny)
//	Medium   → ReviewRequired
//	Low/Info → Allow (surface-only)
package deps

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/provasign/provasign/internal/cert"
	"github.com/provasign/provasign/internal/core"
)

// Gate enforces vulnerability-severity thresholds from Stage 2.
type Gate struct {
	// Analysis supplies the most recent AnalysisResult.
	Analysis func() *cert.AnalysisResult
}

// Name implements policy.Gate.
func (g *Gate) Name() string { return "deps" }

// Evaluate implements policy.Gate.
func (g *Gate) Evaluate(_ context.Context, _ *core.ChangeSet, opts map[string]any) core.PolicyResult {
	res := core.PolicyResult{Gate: g.Name(), Verdict: core.VerdictAllow}

	if g.Analysis == nil || g.Analysis() == nil {
		res.Verdict = core.VerdictReviewRequired
		res.Message = "deps gate: no Analysis result available"
		return res
	}
	s := g.Analysis()
	if s.Skipped {
		res.Message = "deps gate: Stage 2 skipped (" + s.SkipReason + ")"
		return res
	}

	denyHigh := boolOption(opts, "deny_high", false)

	vulns := s.FindingsByCategory()[cert.CategoryVuln]
	if len(vulns) == 0 {
		res.Message = "no vulnerable dependencies detected"
		return res
	}
	sort.SliceStable(vulns, func(i, j int) bool {
		if vulns[i].RuleID != vulns[j].RuleID {
			return vulns[i].RuleID < vulns[j].RuleID
		}
		return vulns[i].Path < vulns[j].Path
	})

	var critical, high, medium, lowish int
	for _, f := range vulns {
		switch f.Severity {
		case cert.SeverityCritical:
			critical++
		case cert.SeverityHigh:
			high++
		case cert.SeverityMedium:
			medium++
		default:
			lowish++
		}
	}

	switch {
	case critical > 0:
		res.Verdict = core.VerdictDeny
	case high > 0 && denyHigh:
		res.Verdict = core.VerdictDeny
	case high > 0 || medium > 0:
		res.Verdict = core.VerdictReviewRequired
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d vulnerable dep(s)", len(vulns))
	if critical > 0 {
		fmt.Fprintf(&b, " critical=%d", critical)
	}
	if high > 0 {
		fmt.Fprintf(&b, " high=%d", high)
	}
	if medium > 0 {
		fmt.Fprintf(&b, " medium=%d", medium)
	}
	if lowish > 0 {
		fmt.Fprintf(&b, " low=%d", lowish)
	}
	limit := 3
	b.WriteString(":")
	for i, f := range vulns {
		if i >= limit {
			fmt.Fprintf(&b, " (+%d more)", len(vulns)-limit)
			break
		}
		fmt.Fprintf(&b, " %s;", f.RuleID)
	}
	res.Message = b.String()
	if res.Verdict != core.VerdictAllow {
		res.NextAction = "upgrade or replace the vulnerable dependency; for unavoidable cases, request review"
	}
	res.Details = map[string]any{
		"count":    len(vulns),
		"critical": critical,
		"high":     high,
		"medium":   medium,
	}
	return res
}

func boolOption(opts map[string]any, key string, def bool) bool {
	if opts == nil {
		return def
	}
	v, ok := opts[key]
	if !ok {
		return def
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return def
}
