// Package outbox replays certificates from relay's local engine store into
// a separate git repository (the "intent store"). It exists so a solo
// developer or single-machine team can keep a permanent, append-only audit
// log of every admitted certificate without standing up Postgres.
//
// Each push appends one file per new certificate under
// `<intent-store>/certificates/<cert-id>.json` and commits the batch with
// a "Cert-Cursor:" trailer so the next push knows where to resume. A
// per-store file lock prevents concurrent pushes from racing.
package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/tabladrum/grove-suite/relay/internal/core"
	"github.com/tabladrum/grove-suite/relay/internal/enginestore"
)

// CursorFile records the ID of the last successfully pushed certificate.
// It lives at <intent-store>/.relay-outbox-cursor and is the source of
// truth for resume — git history is read-only audit.
const CursorFile = ".relay-outbox-cursor"

// LockFile guards Push against concurrent invocations on the same intent
// store. Lives at <intent-store>/.relay-outbox.lock.
const LockFile = ".relay-outbox.lock"

// CertDir is the subdirectory under the intent store where per-cert
// snapshot files are written.
const CertDir = "certificates"

// CertLister is the subset of enginestore.Store that Pusher needs.
// Defining it as an interface keeps the package easy to test with a fake.
type CertLister interface {
	ListCertificates(ctx context.Context, sinceID string, limit int) ([]*core.Certificate, error)
}

// Snapshot is the JSON document written per certificate. It mirrors the
// signed certificate verbatim and tags it with the effective config hash
// for traceability across distinct relay configurations.
type Snapshot struct {
	Certificate         *core.Certificate `json:"certificate"`
	EffectiveConfigHash string            `json:"effective_config_hash"`
}

// PushResult summarizes a single Push invocation.
type PushResult struct {
	Pushed     int    `json:"pushed"`
	LastCertID string `json:"last_cert_id,omitempty"`
	Commit     string `json:"commit,omitempty"`
	Note       string `json:"note,omitempty"`
}

// Pusher writes new certificates from store into intent-store on each Push.
type Pusher struct {
	Store       CertLister
	IntentStore string
	// GitCmd is the git binary. Defaults to "git".
	GitCmd string
	// Author identifies the committer for outbox snapshots.
	// Defaults to "relay-outbox <relay@local>".
	AuthorName  string
	AuthorEmail string
	// BatchLimit caps how many certs a single Push commits. Zero means no
	// limit.
	BatchLimit int

	mu sync.Mutex // serializes Push within a single process
}

// New returns a Pusher with sensible defaults.
func New(store CertLister, intentStore string) *Pusher {
	return &Pusher{
		Store:       store,
		IntentStore: intentStore,
		GitCmd:      "git",
		AuthorName:  "relay-outbox",
		AuthorEmail: "relay@local",
	}
}

// Push appends snapshots for every certificate created after the stored
// cursor, commits them, and advances the cursor. It is safe to call on an
// up-to-date store (returns a zero-count PushResult).
func (p *Pusher) Push(ctx context.Context) (PushResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.ensureIntentStore(); err != nil {
		return PushResult{}, err
	}
	release, err := p.acquireFileLock()
	if err != nil {
		return PushResult{}, err
	}
	defer release()

	cursor, err := p.readCursor()
	if err != nil {
		return PushResult{}, err
	}
	certs, err := p.Store.ListCertificates(ctx, cursor, p.BatchLimit)
	if err != nil {
		return PushResult{}, fmt.Errorf("outbox: list certs: %w", err)
	}
	if len(certs) == 0 {
		return PushResult{Note: "up to date"}, nil
	}

	certDir := filepath.Join(p.IntentStore, CertDir)
	if err := os.MkdirAll(certDir, 0o755); err != nil {
		return PushResult{}, err
	}
	for _, c := range certs {
		snap := Snapshot{Certificate: c, EffectiveConfigHash: c.EffectiveConfigHash}
		body, err := json.MarshalIndent(snap, "", "  ")
		if err != nil {
			return PushResult{}, err
		}
		path := filepath.Join(certDir, c.ID+".json")
		if err := os.WriteFile(path, body, 0o644); err != nil {
			return PushResult{}, err
		}
	}
	last := certs[len(certs)-1]
	if err := p.writeCursor(last.ID); err != nil {
		return PushResult{}, err
	}

	if err := p.runGit(ctx, "add", "-A"); err != nil {
		return PushResult{}, err
	}
	msg := fmt.Sprintf("outbox: push %d certificate(s)\n\nCert-Cursor: %s\n", len(certs), last.ID)
	if err := p.runGit(ctx, "-c", "user.name="+p.AuthorName, "-c", "user.email="+p.AuthorEmail,
		"commit", "--allow-empty", "-m", msg); err != nil {
		return PushResult{}, err
	}
	sha, err := p.captureGit(ctx, "rev-parse", "HEAD")
	if err != nil {
		return PushResult{}, err
	}
	return PushResult{
		Pushed:     len(certs),
		LastCertID: last.ID,
		Commit:     strings.TrimSpace(sha),
	}, nil
}

// ensureIntentStore creates the intent store directory and initializes a
// git repo there if necessary.
func (p *Pusher) ensureIntentStore() error {
	if strings.TrimSpace(p.IntentStore) == "" {
		return errors.New("outbox: empty IntentStore")
	}
	if err := os.MkdirAll(p.IntentStore, 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(p.IntentStore, ".git")); err == nil {
		return nil
	}
	cmd := exec.Command(p.gitBin(), "init", "-q", "-b", "main", p.IntentStore)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("outbox: git init: %v: %s", err, out)
	}
	return nil
}

// acquireFileLock takes an exclusive lock for the duration of Push. The
// lock is implemented as O_CREATE|O_EXCL on LockFile — simple and good
// enough for the single-machine outbox.
func (p *Pusher) acquireFileLock() (func(), error) {
	path := filepath.Join(p.IntentStore, LockFile)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("outbox: another push in progress (%s): %w", path, err)
	}
	_ = f.Close()
	return func() { _ = os.Remove(path) }, nil
}

func (p *Pusher) readCursor() (string, error) {
	body, err := os.ReadFile(filepath.Join(p.IntentStore, CursorFile))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(body)), nil
}

func (p *Pusher) writeCursor(id string) error {
	return os.WriteFile(filepath.Join(p.IntentStore, CursorFile), []byte(id+"\n"), 0o644)
}

func (p *Pusher) gitBin() string {
	if p.GitCmd != "" {
		return p.GitCmd
	}
	return "git"
}

func (p *Pusher) runGit(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, p.gitBin(), args...)
	cmd.Dir = p.IntentStore
	cmd.Stdout = io.Discard
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("outbox: git %s: %w: %s", strings.Join(args, " "), err, stderr.String())
	}
	return nil
}

func (p *Pusher) captureGit(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, p.gitBin(), args...)
	cmd.Dir = p.IntentStore
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// Compile-time check: *enginestore.Store must implement CertLister.
var _ CertLister = (*enginestore.Store)(nil)
