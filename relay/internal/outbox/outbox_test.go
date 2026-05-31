package outbox

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/tabladrum/grove-suite/relay/internal/core"
)

type fakeStore struct {
	certs []*core.Certificate
}

func (f *fakeStore) ListCertificates(_ context.Context, sinceID string, limit int) ([]*core.Certificate, error) {
	start := 0
	if sinceID != "" {
		for i, c := range f.certs {
			if c.ID == sinceID {
				start = i + 1
				break
			}
		}
	}
	out := f.certs[start:]
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	// Copy so callers can't mutate the backing slice.
	cp := make([]*core.Certificate, len(out))
	copy(cp, out)
	return cp, nil
}

func ensureGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not in PATH")
	}
}

func mkCert(id string, t time.Time) *core.Certificate {
	return &core.Certificate{
		ID:                  id,
		ChangeSetID:         "cs-" + id,
		EffectiveConfigHash: "deadbeef",
		CreatedAt:           t,
	}
}

func TestPush_FreshThenIncremental(t *testing.T) {
	ensureGit(t)
	now := time.Now()
	fs := &fakeStore{certs: []*core.Certificate{
		mkCert("c1", now),
		mkCert("c2", now.Add(time.Second)),
	}}
	dir := t.TempDir()
	p := New(fs, dir)

	res, err := p.Push(context.Background())
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if res.Pushed != 2 || res.LastCertID != "c2" || res.Commit == "" {
		t.Fatalf("unexpected first push result: %+v", res)
	}
	// Files exist + cursor updated.
	for _, id := range []string{"c1", "c2"} {
		path := filepath.Join(dir, CertDir, id+".json")
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("missing %s: %v", path, err)
		}
		var snap Snapshot
		if err := json.Unmarshal(body, &snap); err != nil {
			t.Fatalf("bad json %s: %v", path, err)
		}
		if snap.Certificate == nil || snap.Certificate.ID != id {
			t.Errorf("snapshot %s wrong: %+v", id, snap.Certificate)
		}
	}
	cur, _ := os.ReadFile(filepath.Join(dir, CursorFile))
	if string(cur) != "c2\n" {
		t.Errorf("cursor=%q want c2", string(cur))
	}

	// Second push with nothing new.
	res2, err := p.Push(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res2.Pushed != 0 {
		t.Errorf("second push should push nothing, got %d", res2.Pushed)
	}

	// Add a new cert and push again.
	fs.certs = append(fs.certs, mkCert("c3", now.Add(2*time.Second)))
	res3, err := p.Push(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res3.Pushed != 1 || res3.LastCertID != "c3" {
		t.Errorf("incremental push wrong: %+v", res3)
	}
}

func TestPush_EmptyStore(t *testing.T) {
	ensureGit(t)
	p := New(&fakeStore{}, t.TempDir())
	res, err := p.Push(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Pushed != 0 || res.Note == "" {
		t.Errorf("expected up-to-date result, got %+v", res)
	}
}

func TestPush_FileLockBlocksConcurrent(t *testing.T) {
	ensureGit(t)
	dir := t.TempDir()
	// Pre-create the lock file to simulate another in-flight push.
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, LockFile), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	p := New(&fakeStore{certs: []*core.Certificate{mkCert("c1", time.Now())}}, dir)
	if _, err := p.Push(context.Background()); err == nil {
		t.Fatal("expected push to fail when lock file exists")
	}
}

func TestPush_EmptyIntentStore(t *testing.T) {
	ensureGit(t)
	p := New(&fakeStore{}, "")
	if _, err := p.Push(context.Background()); err == nil {
		t.Fatal("expected error for empty IntentStore")
	}
}

func TestPush_RespectsBatchLimit(t *testing.T) {
	ensureGit(t)
	now := time.Now()
	fs := &fakeStore{certs: []*core.Certificate{
		mkCert("a", now),
		mkCert("b", now.Add(time.Second)),
		mkCert("c", now.Add(2*time.Second)),
	}}
	p := New(fs, t.TempDir())
	p.BatchLimit = 2
	res, err := p.Push(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Pushed != 2 || res.LastCertID != "b" {
		t.Errorf("batch limit ignored: %+v", res)
	}
}
