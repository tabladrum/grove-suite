package coverage

import (
	"context"
	"strings"
	"testing"

	"github.com/tabladrum/grove-suite/relay/internal/cert"
	"github.com/tabladrum/grove-suite/relay/internal/core"
)

func TestGate_AllowAboveThreshold(t *testing.T) {
	s := &cert.BuildResult{BuildOk: true, Tests: cert.TestRun{Passed: 1, CoveragePct: 80}}
	g := &Gate{Build: func() *cert.BuildResult { return s }}
	res := g.Evaluate(context.Background(), &core.ChangeSet{Diff: "+++ b/foo.go\n"}, map[string]any{"min_coverage": 75.0})
	if res.Verdict != core.VerdictAllow {
		t.Errorf("verdict=%s msg=%s", res.Verdict, res.Message)
	}
}

func TestGate_DenyBelowThreshold(t *testing.T) {
	s := &cert.BuildResult{Tests: cert.TestRun{Passed: 1, CoveragePct: 40}}
	g := &Gate{Build: func() *cert.BuildResult { return s }}
	res := g.Evaluate(context.Background(), &core.ChangeSet{Diff: "+++ b/foo.go\n+a\n"}, map[string]any{"min_coverage": 80.0})
	if res.Verdict != core.VerdictDeny {
		t.Errorf("expected deny, got %s", res.Verdict)
	}
	if !strings.Contains(res.Message, "below minimum") {
		t.Errorf("expected actionable message, got %q", res.Message)
	}
}

func TestGate_EscapeHatchBypasses(t *testing.T) {
	s := &cert.BuildResult{Tests: cert.TestRun{Passed: 1, CoveragePct: 0}}
	g := &Gate{Build: func() *cert.BuildResult { return s }}
	diff := "+++ b/foo.go\n+// " + EscapeHatchMarker + " — generated code\n"
	res := g.Evaluate(context.Background(), &core.ChangeSet{Diff: diff}, map[string]any{"min_coverage": 100.0})
	if res.Verdict != core.VerdictAllow {
		t.Errorf("expected allow, got %s (%s)", res.Verdict, res.Message)
	}
}

func TestGate_RemovedLineHatchIgnored(t *testing.T) {
	s := &cert.BuildResult{Tests: cert.TestRun{Passed: 1, CoveragePct: 0}}
	g := &Gate{Build: func() *cert.BuildResult { return s }}
	// Marker only on a removed line: bypass must NOT apply.
	diff := "+++ b/foo.go\n-// " + EscapeHatchMarker + "\n+a\n"
	res := g.Evaluate(context.Background(), &core.ChangeSet{Diff: diff}, map[string]any{"min_coverage": 50.0})
	if res.Verdict != core.VerdictDeny {
		t.Errorf("expected deny, got %s", res.Verdict)
	}
}

func TestGate_NoStage1FailsClosed(t *testing.T) {
	g := &Gate{Build: func() *cert.BuildResult { return nil }}
	res := g.Evaluate(context.Background(), &core.ChangeSet{}, nil)
	if res.Verdict != core.VerdictReviewRequired {
		t.Errorf("expected review_required, got %s", res.Verdict)
	}
}

func TestGate_NilHolderFailsClosed(t *testing.T) {
	g := &Gate{}
	res := g.Evaluate(context.Background(), &core.ChangeSet{}, nil)
	if res.Verdict != core.VerdictReviewRequired {
		t.Errorf("expected review_required, got %s", res.Verdict)
	}
}

func TestGate_DefaultThresholdZeroAllows(t *testing.T) {
	s := &cert.BuildResult{Tests: cert.TestRun{Passed: 1, CoveragePct: 0}}
	g := &Gate{Build: func() *cert.BuildResult { return s }}
	res := g.Evaluate(context.Background(), &core.ChangeSet{Diff: "+++ b/foo.go\n"}, nil)
	if res.Verdict != core.VerdictAllow {
		t.Errorf("expected allow with no threshold, got %s", res.Verdict)
	}
}

func TestFloatOption(t *testing.T) {
	if v := floatOption(nil, "x", 7); v != 7 {
		t.Error("default for nil opts")
	}
	if v := floatOption(map[string]any{"x": 1.5}, "x", 0); v != 1.5 {
		t.Error("float64")
	}
	if v := floatOption(map[string]any{"x": 2}, "x", 0); v != 2 {
		t.Error("int")
	}
	if v := floatOption(map[string]any{"x": int64(3)}, "x", 0); v != 3 {
		t.Error("int64")
	}
	if v := floatOption(map[string]any{"x": "nope"}, "x", 9); v != 9 {
		t.Error("default for bad type")
	}
}

func TestName(t *testing.T) {
	g := &Gate{}
	if g.Name() != "coverage" {
		t.Errorf("name = %q", g.Name())
	}
}
