package secrets

import (
	"context"
	"strings"
	"testing"

	"github.com/provasign/provasign/internal/cert"
	"github.com/provasign/provasign/internal/core"
)

func TestGate_AllowsWhenNoSecrets(t *testing.T) {
	g := &Gate{Analysis: func() *cert.AnalysisResult {
		return &cert.AnalysisResult{}
	}}
	r := g.Evaluate(context.Background(), &core.ChangeSet{}, nil)
	if r.Verdict != core.VerdictAllow {
		t.Fatalf("verdict=%s want allow", r.Verdict)
	}
}

func TestGate_DeniesOnSecretFinding(t *testing.T) {
	g := &Gate{Analysis: func() *cert.AnalysisResult {
		return &cert.AnalysisResult{Findings: []cert.Finding{
			{Analyzer: "x", Category: cert.CategorySecret, RuleID: "aws", Path: "a.go", Line: 3, Severity: cert.SeverityCritical},
		}}
	}}
	r := g.Evaluate(context.Background(), &core.ChangeSet{}, nil)
	if r.Verdict != core.VerdictDeny {
		t.Fatalf("verdict=%s want deny", r.Verdict)
	}
	if !strings.Contains(r.Message, "secret(s)") {
		t.Errorf("message=%s", r.Message)
	}
	if r.NextAction == "" {
		t.Error("expected NextAction")
	}
}

func TestGate_ReviewRequiredWhenStage2Skipped(t *testing.T) {
	g := &Gate{Analysis: func() *cert.AnalysisResult {
		return &cert.AnalysisResult{Skipped: true, SkipReason: "no analyzers"}
	}}
	r := g.Evaluate(context.Background(), &core.ChangeSet{}, nil)
	if r.Verdict != core.VerdictReviewRequired {
		t.Fatalf("verdict=%s want review_required", r.Verdict)
	}
}

func TestGate_ReviewRequiredWhenStage2Nil(t *testing.T) {
	g := &Gate{Analysis: func() *cert.AnalysisResult { return nil }}
	r := g.Evaluate(context.Background(), &core.ChangeSet{}, nil)
	if r.Verdict != core.VerdictReviewRequired {
		t.Fatalf("verdict=%s want review_required", r.Verdict)
	}
}
