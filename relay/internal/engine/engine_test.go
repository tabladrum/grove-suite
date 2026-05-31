package engine
package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/tabladrum/grove-suite/relay/internal/core"
	"github.com/tabladrum/grove-suite/relay/internal/enginestore"
	"github.com/tabladrum/grove-suite/relay/internal/policy"
	"github.com/tabladrum/grove-suite/relay/internal/signer"
)

type fakeAdmitter struct {
	sha string
	err error
}

func (f *fakeAdmitter) Admit(_ context.Context, _ *core.ChangeSet, _ *core.Certificate) (string, error) {
	return f.sha, f.err
}

func newTestEngine(t *testing.T, admit Admitter) *Engine {
	t.Helper()
	store, err := enginestore.Open(":memory:")
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	reg := policy.NewRegistry()
	reg.Register(policy.PathGate{})
	reg.Register(policy.SizeGate{})

	sgn, err := signer.LoadOrCreateLocal(t.TempDir())
	if err != nil {
		t.Fatalf("signer: %v", err)
	}

	return &Engine{
		Store:    store,
		Policies: reg,
		ICR:      NoopICRProvider{},
		Signer:   sgn,
		Admit:    admit,
		Config: &core.RelayConfig{
			Policies: map[string]core.PolicyBlock{
				"path": {Enabled: true, Options: map[string]any{"deny": []any{".git/"}}},
				"size": {Enabled: true, Options: map[string]any{"max_files": 10, "max_added_lines": 100}},
			},
		},
	}
}

func sampleChangeSet() *core.ChangeSet {
	return &core.ChangeSet{
		ID:          "cs-test-1",
		IntentID:    "intent-test",
		IntentBrief: "add foo",
		RepoRoot:    "/tmp/repo",
		BaseSHA:     "0000",
		Diff:        "--- a/foo.go\n+++ b/foo.go\n@@ -1 +1,2 @@\n package foo\n+// new\n",
	}
}

func TestCheck_AllowsCleanChangeSet(t *testing.T) {
	e := newTestEngine(t, &fakeAdmitter{sha: "sha1"})
	res, err := e.Check(context.Background(), sampleChangeSet())
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if !res.Allowed || len(res.Policies) != 2 {
		t.Errorf("unexpected: allowed=%v policies=%+v", res.Allowed, res.Policies)
	}
}

func TestCheck_RequiresConfig(t *testing.T) {
	e := newTestEngine(t, &fakeAdmitter{})
	e.Config = nil
	if _, err := e.Check(context.Background(), sampleChangeSet()); err == nil {
		t.Errorf("expected error for nil config")
	}
}

func TestCertify_ProducesSignedCertificate(t *testing.T) {
	e := newTestEngine(t, &fakeAdmitter{sha: "sha1"})
	res, err := e.Certify(context.Background(), sampleChangeSet())
	if err != nil {
		t.Fatalf("certify: %v", err)
	}
	if res.Certificate == nil {
		t.Fatal("expected certificate")
	}
	if len(res.Certificate.Signature) == 0 {
		t.Error("certificate not signed")
	}
	// Verify the signature against the signer's pubkey.
	if err := signer.Verify(e.Signer.PublicKey(), res.Certificate, res.Certificate.Signature); err != nil {
		t.Errorf("verify: %v", err)
	}
	if res.Certificate.EffectiveConfigHash == "" {
		t.Error("expected config hash")
	}
}

func TestCertify_SkipsCertWhenGateDenies(t *testing.T) {
	e := newTestEngine(t, &fakeAdmitter{sha: "sha1"})
	cs := sampleChangeSet()
	cs.Diff = "--- a/.git/HEAD\n+++ b/.git/HEAD\n@@\n+nope\n"
	res, err := e.Certify(context.Background(), cs)
	if err != nil {
		t.Fatalf("certify: %v", err)
	}
	if res.Allowed {
		t.Error("expected denied")
	}
	if res.Certificate != nil {
		t.Error("expected no certificate when denied")
	}
}

func TestSubmit_EndToEndWithFakeAdmitter(t *testing.T) {
	admit := &fakeAdmitter{sha: "commit-abc"}
	e := newTestEngine(t, admit)
	cs := sampleChangeSet()
	cs.ID = "" // let engine assign
	res, err := e.Submit(context.Background(), cs)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if res.CommitSHA != "commit-abc" {
		t.Errorf("commit sha: %s", res.CommitSHA)
	}
	if res.Certificate.AdmittedCommitSHA != "commit-abc" {
		t.Errorf("cert sha: %s", res.Certificate.AdmittedCommitSHA)
	}
	// Round-trip via store.
	got, err := e.Store.GetCertificateByCommit(context.Background(), "commit-abc")
	if err != nil {
		t.Fatalf("get cert: %v", err)
	}
	if got.ID != res.Certificate.ID {
		t.Errorf("cert id mismatch")
	}
	gotCS, _ := e.Store.GetChangeSet(context.Background(), cs.ID)
	if gotCS.Status != core.ChangeSetAdmitted {
		t.Errorf("status: %s", gotCS.Status)
	}
}

func TestSubmit_AdmissionFailureMarksRejected(t *testing.T) {
	e := newTestEngine(t, &fakeAdmitter{err: errors.New("boom")})
	cs := sampleChangeSet()
	res, _ := e.Submit(context.Background(), cs)
	if res == nil {
		t.Fatal("expected partial result")
	}
	gotCS, err := e.Store.GetChangeSet(context.Background(), cs.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if gotCS.Status != core.ChangeSetRejected {
		t.Errorf("status: %s", gotCS.Status)
	}
}

func TestSubmit_DeniedChangesetMarksRejected(t *testing.T) {
	e := newTestEngine(t, &fakeAdmitter{sha: "sha1"})
	cs := sampleChangeSet()
	cs.Diff = "--- a/.git/HEAD\n+++ b/.git/HEAD\n@@\n+nope\n"
	res, err := e.Submit(context.Background(), cs)
	if err != nil {
		t.Fatal(err)
	}
	if res.Allowed {
		t.Errorf("expected denied")
	}
	gotCS, _ := e.Store.GetChangeSet(context.Background(), cs.ID)
	if gotCS.Status != core.ChangeSetRejected {
		t.Errorf("status: %s", gotCS.Status)
	}
}

func TestComputeICRHash_DeterministicAndOrderIndependent(t *testing.T) {
	a := ComputeICRHash([]string{"pkg.B", "pkg.A"}, []string{"y.go", "x.go"})
	b := ComputeICRHash([]string{"pkg.A", "pkg.B"}, []string{"x.go", "y.go"})
	if a != b {
		t.Errorf("hash should be order-independent: %s vs %s", a, b)
	}
	c := ComputeICRHash([]string{"pkg.C"}, []string{"x.go"})
	if a == c {
		t.Error("different inputs should hash differently")
	}
}

func TestNoopICRProvider_PopulatesFiles(t *testing.T) {
	icr, err := NoopICRProvider{}.ComputeICR(context.Background(),
		&core.ChangeSet{Diff: "--- a/x.go\n+++ b/x.go\n@@\n+pkg x\n"})
	if err != nil {
		t.Fatal(err)
	}
	if len(icr.Files) != 1 || icr.Files[0] != "x.go" {
		t.Errorf("files: %+v", icr.Files)
	}
	if icr.Confidence != 0 {
		t.Errorf("confidence should be 0 for noop: %v", icr.Confidence)
	}
}
