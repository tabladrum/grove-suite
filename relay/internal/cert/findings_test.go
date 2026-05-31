package cert

import (
	"testing"
)

func TestComputeFingerprint_Deterministic(t *testing.T) {
	a := ComputeFingerprint("semgrep", "rule.x", "a.go", 10, "msg")
	b := ComputeFingerprint("semgrep", "rule.x", "a.go", 10, "msg")
	if a != b {
		t.Fatalf("non-deterministic: %s != %s", a, b)
	}
	if len(a) != 16 {
		t.Fatalf("want 16 hex chars, got %d", len(a))
	}
	c := ComputeFingerprint("semgrep", "rule.x", "a.go", 11, "msg")
	if a == c {
		t.Fatal("line should affect fingerprint")
	}
}

func TestStage2Result_FindingsByCategoryAndHighest(t *testing.T) {
	s := Stage2Result{
		Findings: []Finding{
			{Category: CategorySecret, Severity: SeverityCritical},
			{Category: CategoryVuln, Severity: SeverityHigh},
			{Category: CategorySAST, Severity: SeverityMedium},
		},
	}
	by := s.FindingsByCategory()
	if len(by[CategorySecret]) != 1 || len(by[CategoryVuln]) != 1 || len(by[CategorySAST]) != 1 {
		t.Fatalf("bucket counts wrong: %+v", by)
	}
	if got := s.HighestSeverity(); got != SeverityCritical {
		t.Fatalf("highest = %s, want critical", got)
	}
}

func TestSortFindings_Stable(t *testing.T) {
	in := []Finding{
		{Analyzer: "semgrep", Path: "b.go", Line: 2, RuleID: "r", Fingerprint: "ff"},
		{Analyzer: "gitleaks", Path: "a.go", Line: 1, RuleID: "r", Fingerprint: "aa"},
		{Analyzer: "semgrep", Path: "b.go", Line: 1, RuleID: "r", Fingerprint: "11"},
	}
	out := SortFindings(in)
	if out[0].Analyzer != "gitleaks" {
		t.Fatalf("expected gitleaks first, got %s", out[0].Analyzer)
	}
	if out[1].Line != 1 || out[2].Line != 2 {
		t.Fatalf("expected sort by line, got %+v", out)
	}
}
