package deps

import (
	"context"
	"testing"

	"github.com/provasign/provasign/internal/cert"
	"github.com/provasign/provasign/internal/core"
)

func TestGate_AllowsWhenNoVulns(t *testing.T) {
	g := &Gate{Analysis: func() *cert.AnalysisResult { return &cert.AnalysisResult{} }}
	r := g.Evaluate(context.Background(), &core.ChangeSet{}, nil)
	if r.Verdict != core.VerdictAllow {
		t.Fatalf("verdict=%s want allow", r.Verdict)
	}
}

func TestGate_CriticalDenies(t *testing.T) {
	g := &Gate{Analysis: func() *cert.AnalysisResult {
		return &cert.AnalysisResult{Findings: []cert.Finding{
			{Analyzer: "govulncheck", Category: cert.CategoryVuln, RuleID: "GO-1", Severity: cert.SeverityCritical},
		}}
	}}
	r := g.Evaluate(context.Background(), &core.ChangeSet{}, nil)
	if r.Verdict != core.VerdictDeny {
		t.Fatalf("verdict=%s want deny", r.Verdict)
	}
}

func TestGate_HighIsReviewRequiredByDefault(t *testing.T) {
	g := &Gate{Analysis: func() *cert.AnalysisResult {
		return &cert.AnalysisResult{Findings: []cert.Finding{
			{Analyzer: "govulncheck", Category: cert.CategoryVuln, RuleID: "GO-2", Severity: cert.SeverityHigh},
		}}
	}}
	r := g.Evaluate(context.Background(), &core.ChangeSet{}, nil)
	if r.Verdict != core.VerdictReviewRequired {
		t.Fatalf("verdict=%s want review_required", r.Verdict)
	}
}

func TestGate_HighDeniesWhenConfigured(t *testing.T) {
	g := &Gate{Analysis: func() *cert.AnalysisResult {
		return &cert.AnalysisResult{Findings: []cert.Finding{
			{Analyzer: "govulncheck", Category: cert.CategoryVuln, RuleID: "GO-2", Severity: cert.SeverityHigh},
		}}
	}}
	r := g.Evaluate(context.Background(), &core.ChangeSet{}, map[string]any{"deny_high": true})
	if r.Verdict != core.VerdictDeny {
		t.Fatalf("verdict=%s want deny", r.Verdict)
	}
}

func TestGate_Stage2NilFailsClosed(t *testing.T) {
	g := &Gate{Analysis: func() *cert.AnalysisResult { return nil }}
	r := g.Evaluate(context.Background(), &core.ChangeSet{}, nil)
	if r.Verdict != core.VerdictReviewRequired {
		t.Fatalf("verdict=%s want review_required", r.Verdict)
	}
}
