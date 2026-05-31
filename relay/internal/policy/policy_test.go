package policy

import (
	"context"
	"strings"
	"testing"

	"github.com/tabladrum/grove-suite/relay/internal/core"
)

const sampleDiff = `--- a/foo.go
+++ b/foo.go
@@ -1,2 +1,3 @@
 package foo
+// new line
 // old
--- a/bar.go
+++ b/bar.go
@@ -1,1 +1,2 @@
 package bar
+func Baz() {}
`

func TestPathGate_AllowsCleanDiff(t *testing.T) {
	res := PathGate{}.Evaluate(context.Background(), &core.ChangeSet{Diff: sampleDiff}, map[string]any{
		"deny": []any{"vendor/", ".git/"},
	})
	if res.Verdict != core.VerdictAllow {
		t.Errorf("verdict: %s msg=%s", res.Verdict, res.Message)
	}
}

func TestPathGate_DeniesForbiddenPrefix(t *testing.T) {
	diff := "--- a/vendor/x.go\n+++ b/vendor/x.go\n@@\n+pkg vendor\n"
	res := PathGate{}.Evaluate(context.Background(), &core.ChangeSet{Diff: diff}, map[string]any{
		"deny": []any{"vendor/"},
	})
	if res.Verdict != core.VerdictDeny {
		t.Fatalf("expected deny, got %s", res.Verdict)
	}
	hits := res.Details["hits"].([]string)
	if len(hits) != 1 || !strings.Contains(hits[0], "vendor/x.go") {
		t.Errorf("hits: %+v", hits)
	}
}

func TestPathGate_HandlesDevNullAndStringSliceOption(t *testing.T) {
	diff := "--- a/secret.env\n+++ /dev/null\n"
	res := PathGate{}.Evaluate(context.Background(), &core.ChangeSet{Diff: diff}, map[string]any{
		"deny": []string{"never/"},
	})
	if res.Verdict != core.VerdictAllow {
		t.Errorf("dev/null target should not match: %+v", res)
	}
}

func TestSizeGate_AllowsSmallDiff(t *testing.T) {
	res := SizeGate{}.Evaluate(context.Background(), &core.ChangeSet{Diff: sampleDiff}, map[string]any{
		"max_files":       10,
		"max_added_lines": 100,
	})
	if res.Verdict != core.VerdictAllow {
		t.Errorf("verdict: %s msg=%s", res.Verdict, res.Message)
	}
}

func TestSizeGate_WarnsNearLimit(t *testing.T) {
	// 2 files of 2 added lines each = 2 files, 2 added lines.
	// Set max_files=2 → at limit (>= 80%) → warn.
	res := SizeGate{}.Evaluate(context.Background(), &core.ChangeSet{Diff: sampleDiff}, map[string]any{
		"max_files":       2,
		"max_added_lines": 1000,
	})
	if res.Verdict != core.VerdictWarn {
		t.Errorf("expected warn, got %s (msg=%s)", res.Verdict, res.Message)
	}
}

func TestSizeGate_DeniesTooManyFiles(t *testing.T) {
	res := SizeGate{}.Evaluate(context.Background(), &core.ChangeSet{Diff: sampleDiff}, map[string]any{
		"max_files":       1,
		"max_added_lines": 100,
	})
	if res.Verdict != core.VerdictDeny {
		t.Errorf("expected deny, got %s", res.Verdict)
	}
}

func TestSizeGate_DeniesTooManyAddedLines(t *testing.T) {
	res := SizeGate{}.Evaluate(context.Background(), &core.ChangeSet{Diff: sampleDiff}, map[string]any{
		"max_files":       10,
		"max_added_lines": 1,
	})
	if res.Verdict != core.VerdictDeny {
		t.Errorf("expected deny, got %s", res.Verdict)
	}
}

func TestSizeGate_DefaultLimits(t *testing.T) {
	// Empty options → uses defaults 100/5000.
	res := SizeGate{}.Evaluate(context.Background(), &core.ChangeSet{Diff: sampleDiff}, nil)
	if res.Verdict != core.VerdictAllow {
		t.Errorf("default limits should allow small diff: %s", res.Verdict)
	}
}

func TestRegistryAndEvaluateAll(t *testing.T) {
	reg := NewRegistry()
	reg.Register(PathGate{})
	reg.Register(SizeGate{})

	if got := reg.Names(); len(got) != 2 || got[0] != "path" || got[1] != "size" {
		t.Errorf("names: %v", got)
	}

	cfg := &core.RelayConfig{
		Policies: map[string]core.PolicyBlock{
			"path": {Enabled: true, Options: map[string]any{"deny": []any{".git/"}}},
			"size": {Enabled: true, Options: map[string]any{"max_files": 10, "max_added_lines": 100}},
		},
	}
	results := EvaluateAll(context.Background(), reg, &core.ChangeSet{Diff: sampleDiff}, cfg)
	if len(results) != 2 {
		t.Fatalf("results: %d", len(results))
	}
	if results[0].Gate != "path" || results[1].Gate != "size" {
		t.Errorf("order: %s, %s", results[0].Gate, results[1].Gate)
	}
	if !Allowed(results) {
		t.Errorf("expected allowed")
	}
}

func TestEvaluateAll_DisabledGateSkipped(t *testing.T) {
	reg := NewRegistry()
	reg.Register(PathGate{})
	cfg := &core.RelayConfig{
		Policies: map[string]core.PolicyBlock{
			"path": {Enabled: false},
		},
	}
	if got := EvaluateAll(context.Background(), reg, &core.ChangeSet{}, cfg); len(got) != 0 {
		t.Errorf("expected 0 results, got %d", len(got))
	}
}

func TestEvaluateAll_UnregisteredGateDeniesClosed(t *testing.T) {
	reg := NewRegistry()
	cfg := &core.RelayConfig{
		Policies: map[string]core.PolicyBlock{
			"mystery": {Enabled: true},
		},
	}
	got := EvaluateAll(context.Background(), reg, &core.ChangeSet{}, cfg)
	if len(got) != 1 || got[0].Verdict != core.VerdictDeny {
		t.Errorf("expected one deny: %+v", got)
	}
}

func TestAllowed_BlocksOnReviewRequired(t *testing.T) {
	rs := []core.PolicyResult{
		{Gate: "a", Verdict: core.VerdictAllow},
		{Gate: "b", Verdict: core.VerdictReviewRequired},
	}
	if Allowed(rs) {
		t.Errorf("review_required must block")
	}
}
