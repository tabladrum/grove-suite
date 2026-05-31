package enginestore

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/tabladrum/grove-suite/relay/internal/core"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "engine.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestOpen_MemoryAndMigrations(t *testing.T) {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open memory: %v", err)
	}
	defer s.Close()
	// Re-running migrations should be idempotent.
	if err := runMigrations(s.db); err != nil {
		t.Fatalf("re-run migrations: %v", err)
	}
}

func TestChangeSetRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	cs := &core.ChangeSet{
		ID:          "cs-1",
		IntentID:    "intent-abc",
		IntentBrief: "fix login bug",
		RepoRoot:    "/tmp/repo",
		BaseSHA:     "abc123",
		Diff:        "--- a\n+++ b\n",
		Author:      "claude",
		AgentModel:  "claude-sonnet-4.5",
	}
	if err := s.InsertChangeSet(ctx, cs); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if cs.Status != core.ChangeSetSubmitted {
		t.Errorf("default status: %s", cs.Status)
	}
	if cs.CreatedAt.IsZero() || cs.UpdatedAt.IsZero() {
		t.Error("expected timestamps populated")
	}
	got, err := s.GetChangeSet(ctx, "cs-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.IntentID != cs.IntentID || got.Diff != cs.Diff || got.AgentModel != cs.AgentModel {
		t.Errorf("roundtrip mismatch: %+v", got)
	}
}

func TestUpdateChangeSetStatus(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	cs := &core.ChangeSet{ID: "cs-2", IntentID: "i", RepoRoot: "/r", BaseSHA: "b", Diff: "d"}
	if err := s.InsertChangeSet(ctx, cs); err != nil {
		t.Fatalf("insert: %v", err)
	}
	initial := cs.UpdatedAt
	time.Sleep(2 * time.Millisecond)
	if err := s.UpdateChangeSetStatus(ctx, "cs-2", core.ChangeSetCertified); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err := s.GetChangeSet(ctx, "cs-2")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != core.ChangeSetCertified {
		t.Errorf("status: got %s", got.Status)
	}
	if !got.UpdatedAt.After(initial) {
		t.Error("updated_at should advance")
	}
}

func TestUpdateChangeSetStatus_NotFound(t *testing.T) {
	s := newTestStore(t)
	err := s.UpdateChangeSetStatus(context.Background(), "missing", core.ChangeSetCertified)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestGetChangeSet_NotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetChangeSet(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestCertificateRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	cs := &core.ChangeSet{ID: "cs-3", IntentID: "i", RepoRoot: "/r", BaseSHA: "b", Diff: "d"}
	if err := s.InsertChangeSet(ctx, cs); err != nil {
		t.Fatalf("insert cs: %v", err)
	}
	cert := &core.Certificate{
		ID:                "cert-1",
		ChangeSetID:       "cs-3",
		IntentID:          "i",
		AdmittedCommitSHA: "deadbeef",
		BaseSHA:           "b",
		ICR: core.ICR{
			Symbols:    []string{"pkg.Foo", "pkg.Bar"},
			Files:      []string{"a.go", "b.go"},
			Edges:      4,
			Confidence: 0.87,
			Hash:       "icrhash",
		},
		Policies: []core.PolicyResult{
			{Gate: "path", Verdict: core.VerdictAllow, Message: "ok"},
			{Gate: "size", Verdict: core.VerdictWarn, Message: "near limit"},
		},
		EffectiveConfigHash: "cfghash",
		PolicyVersion:       "v1",
		SignedBy:            "local-ed25519:abc",
		Signature:           []byte{0x01, 0x02, 0x03, 0x04},
	}
	if err := s.InsertCertificate(ctx, cert); err != nil {
		t.Fatalf("insert cert: %v", err)
	}
	got, err := s.GetCertificate(ctx, "cert-1")
	if err != nil {
		t.Fatalf("get cert: %v", err)
	}
	if got.AdmittedCommitSHA != "deadbeef" || len(got.ICR.Symbols) != 2 || len(got.Policies) != 2 {
		t.Errorf("roundtrip mismatch: %+v", got)
	}
	if string(got.Signature) != string(cert.Signature) {
		t.Errorf("signature mismatch")
	}
	byCommit, err := s.GetCertificateByCommit(ctx, "deadbeef")
	if err != nil {
		t.Fatalf("by commit: %v", err)
	}
	if byCommit.ID != "cert-1" {
		t.Errorf("by commit lookup wrong id: %s", byCommit.ID)
	}
}

func TestGetCertificate_NotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetCertificate(context.Background(), "nope")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
	_, err = s.GetCertificateByCommit(context.Background(), "nope")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("by commit: want ErrNotFound, got %v", err)
	}
}
