package fileclass

import (
	"context"
	"testing"

	"github.com/provasign/provasign/internal/core"
)

const diffSample = `diff --git a/x.go b/x.go
--- a/x.go
+++ b/x.go
@@ -0,0 +1,1 @@
+package x
diff --git a/key.pem b/key.pem
--- /dev/null
+++ b/key.pem
@@ -0,0 +1,1 @@
+-----BEGIN-----
`

func TestGate_DeniesPemByExt(t *testing.T) {
	g := &Gate{}
	r := g.Evaluate(context.Background(), &core.ChangeSet{Diff: diffSample}, map[string]any{
		"deny_exts": []any{".pem"},
	})
	if r.Verdict != core.VerdictDeny {
		t.Fatalf("verdict=%s want deny", r.Verdict)
	}
	if r.NextAction == "" {
		t.Error("expected NextAction")
	}
}

func TestGate_AllowsWhenNoMatches(t *testing.T) {
	g := &Gate{}
	r := g.Evaluate(context.Background(), &core.ChangeSet{Diff: diffSample}, map[string]any{
		"deny_exts": []any{".bin"},
	})
	if r.Verdict != core.VerdictAllow {
		t.Fatalf("verdict=%s want allow", r.Verdict)
	}
}

func TestGate_NoRulesIsAllow(t *testing.T) {
	g := &Gate{}
	r := g.Evaluate(context.Background(), &core.ChangeSet{Diff: diffSample}, nil)
	if r.Verdict != core.VerdictAllow {
		t.Fatalf("verdict=%s want allow", r.Verdict)
	}
}

func TestGate_GlobMatch(t *testing.T) {
	diff := `+++ b/secrets/id_rsa
@@ -0,0 +1,1 @@
+x
`
	g := &Gate{}
	r := g.Evaluate(context.Background(), &core.ChangeSet{Diff: diff}, map[string]any{
		"deny_globs": []any{"secrets/id_rsa"},
	})
	if r.Verdict != core.VerdictDeny {
		t.Fatalf("verdict=%s want deny", r.Verdict)
	}
}
