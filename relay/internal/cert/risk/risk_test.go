package risk

import (
	"testing"

	"github.com/tabladrum/grove-suite/relay/internal/cert"
	"github.com/tabladrum/grove-suite/relay/internal/core"
)

func TestCompute_NoFiles_EmptyHeatmap(t *testing.T) {
	h := Compute(Inputs{})
	if h.ModelVersion != ModelVersion {
		t.Fatalf("model version: got %q want %q", h.ModelVersion, ModelVersion)
	}
	if len(h.Cells) != 0 {
		t.Fatalf("expected zero cells, got %d", len(h.Cells))
	}
	if h.Overall != 0 {
		t.Fatalf("overall: %v", h.Overall)
	}
}

func TestCompute_BlendsAllSignals(t *testing.T) {
	in := Inputs{
		ICR: core.ICR{
			Files:      []string{"a.go", "b.go", "a.go"}, // dup deduped
			Confidence: 0.8,                              // -> icr_unknown = 0.2
		},
		Build: &cert.BuildResult{Tests: cert.TestRun{CoveragePct: 60}}, // covDef = 0.4
		Analysis: &cert.AnalysisResult{Findings: []cert.Finding{
			{Path: "a.go", Severity: cert.Severity("critical")},
			{Path: "a.go", Severity: cert.Severity("low")},
			{Path: "b.go", Severity: cert.Severity("medium")},
		}},
	}
	h := Compute(in)
	if h.ModelVersion != ModelVersion {
		t.Fatalf("model version")
	}
	if len(h.Cells) != 2 {
		t.Fatalf("cells: got %d want 2", len(h.Cells))
	}
	// Hottest first.
	if h.Cells[0].File != "a.go" {
		t.Fatalf("expected hottest = a.go, got %s", h.Cells[0].File)
	}
	if h.Cells[0].Score <= h.Cells[1].Score {
		t.Fatalf("sort: %v vs %v", h.Cells[0].Score, h.Cells[1].Score)
	}
	if h.Overall != h.Cells[0].Score {
		t.Fatalf("overall != hottest cell: %v vs %v", h.Overall, h.Cells[0].Score)
	}
	// Components present.
	for _, key := range []string{"icr_unknown", "analysis_severity", "coverage_deficit", "touch_intensity"} {
		if _, ok := h.Cells[0].Components[key]; !ok {
			t.Fatalf("missing component %q", key)
		}
	}
	// a.go has a critical finding -> stage2_severity = 1.0.
	if h.Cells[0].Components["analysis_severity"] != 1.0 {
		t.Fatalf("a.go severity: %v want 1.0", h.Cells[0].Components["analysis_severity"])
	}
	// b.go has medium -> 0.5.
	if h.Cells[1].Components["analysis_severity"] != 0.5 {
		t.Fatalf("b.go severity: %v want 0.5", h.Cells[1].Components["analysis_severity"])
	}
}

func TestCompute_ScoreBoundedZeroToOne(t *testing.T) {
	files := make([]string, 50)
	for i := range files {
		files[i] = string(rune('a'+i)) + ".go"
	}
	in := Inputs{
		ICR:    core.ICR{Files: files, Confidence: 0},
		Build: &cert.BuildResult{Tests: cert.TestRun{CoveragePct: 0}},
		Analysis: &cert.AnalysisResult{},
	}
	h := Compute(in)
	for _, c := range h.Cells {
		if c.Score < 0 || c.Score > 1 {
			t.Fatalf("score out of [0,1]: %v on %s", c.Score, c.File)
		}
	}
	if h.Overall < 0 || h.Overall > 1 {
		t.Fatalf("overall out of [0,1]: %v", h.Overall)
	}
}

func TestStage2SeverityForFile_WorstWins(t *testing.T) {
	got := analysisSeverityForFile([]cert.Finding{
		{Severity: cert.Severity("low")},
		{Severity: cert.Severity("high")},
		{Severity: cert.Severity("medium")},
	})
	if got != 0.75 {
		t.Fatalf("got %v want 0.75", got)
	}
}

func TestStage2SeverityForFile_UnknownIsZero(t *testing.T) {
	got := analysisSeverityForFile([]cert.Finding{{Severity: cert.Severity("mysterious")}})
	if got != 0 {
		t.Fatalf("got %v want 0", got)
	}
}

func TestClamp01(t *testing.T) {
	if clamp01(-1) != 0 || clamp01(2) != 1 || clamp01(0.5) != 0.5 {
		t.Fatal("clamp01 broken")
	}
}
