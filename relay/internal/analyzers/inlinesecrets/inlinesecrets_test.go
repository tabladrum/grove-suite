package inlinesecrets

import (
	"context"
	"strings"
	"testing"

	"github.com/tabladrum/grove-suite/relay/internal/cert"
	"github.com/tabladrum/grove-suite/relay/internal/core"
)

func TestAvailable_AlwaysTrue(t *testing.T) {
	if !New().Available() {
		t.Fatal("inline-secrets must always be available")
	}
}

func TestRun_DetectsAWSKey(t *testing.T) {
	diff := `diff --git a/x.go b/x.go
--- a/x.go
+++ b/x.go
@@ -0,0 +1,3 @@
+package x
+
+const k = "AKIAIOSFODNN7EXAMPLE"
`
	fs, err := New().Run(context.Background(), &core.ChangeSet{Diff: diff}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) != 1 {
		t.Fatalf("want 1 finding, got %d: %+v", len(fs), fs)
	}
	f := fs[0]
	if f.Category != cert.CategorySecret {
		t.Errorf("category=%s", f.Category)
	}
	if f.Severity != cert.SeverityCritical {
		t.Errorf("severity=%s", f.Severity)
	}
	if f.Path != "x.go" {
		t.Errorf("path=%s", f.Path)
	}
	if !strings.Contains(strings.ToLower(f.NextAction), "rotate") {
		t.Errorf("next_action missing rotate guidance: %q", f.NextAction)
	}
}

func TestRun_IgnoresRemovedLines(t *testing.T) {
	// A key on a removed (`-`) line is being deleted; should NOT flag.
	diff := `--- a/x.go
+++ b/x.go
@@ -1,2 +1,1 @@
-const k = "AKIAIOSFODNN7EXAMPLE"
 package x
`
	fs, err := New().Run(context.Background(), &core.ChangeSet{Diff: diff}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) != 0 {
		t.Fatalf("expected 0 findings on removal, got %d", len(fs))
	}
}

func TestRun_DetectsPrivateKey(t *testing.T) {
	diff := `+++ b/key.txt
@@ -0,0 +1,1 @@
+-----BEGIN RSA PRIVATE KEY-----
`
	fs, _ := New().Run(context.Background(), &core.ChangeSet{Diff: diff}, "")
	if len(fs) != 1 || fs[0].RuleID != "private-key-pem" {
		t.Fatalf("want private-key-pem finding, got %+v", fs)
	}
}

func TestRun_NilChangeSet(t *testing.T) {
	fs, err := New().Run(context.Background(), nil, "")
	if err != nil || fs != nil {
		t.Fatalf("nil cs should be no-op, got fs=%v err=%v", fs, err)
	}
}
