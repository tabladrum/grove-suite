package enginestore
// Package enginestore is the laptop-mode (SQLite, pure-Go) persistence layer
// for the certified-merge engine. Schema is intentionally portable so the
// team-mode Postgres adapter (MVP-T1) can reuse it without migration.
package enginestore

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/tabladrum/grove-suite/relay/internal/core"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// ErrNotFound is returned when a lookup by ID misses.
var ErrNotFound = errors.New("enginestore: not found")

// Store is the persistence handle.
type Store struct {
	db *sql.DB
}

// Open opens (or creates) a SQLite database file at path and applies migrations.
// Use ":memory:" for ephemeral test stores.
func Open(path string) (*Store, error) {
	dsn := path
	if path != ":memory:" {
		// modernc.org/sqlite uses standard sqlite URI options; enable FK + WAL.
		dsn = path + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	if err := runMigrations(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrations: %w", err)
	}
	return &Store{db: db}, nil
}

// Close releases the underlying database connection.
func (s *Store) Close() error { return s.db.Close() }

func runMigrations(db *sql.DB) error {
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		data, err := migrationFS.ReadFile("migrations/" + e.Name())
		if err != nil {
			return fmt.Errorf("read %s: %w", e.Name(), err)
		}
		// Split on ';' is unsafe in general; modernc.org/sqlite accepts
		// multi-statement Exec, so we pass the entire file as one call.
		if _, err := db.Exec(string(data)); err != nil {
			return fmt.Errorf("exec %s: %w", e.Name(), err)
		}
	}
	return nil
}

const rfc3339Nano = time.RFC3339Nano

// InsertChangeSet persists a new ChangeSet. Sets CreatedAt/UpdatedAt if zero.
func (s *Store) InsertChangeSet(ctx context.Context, cs *core.ChangeSet) error {
	now := time.Now().UTC()
	if cs.CreatedAt.IsZero() {
		cs.CreatedAt = now
	}
	if cs.UpdatedAt.IsZero() {
		cs.UpdatedAt = now
	}
	if cs.Status == "" {
		cs.Status = core.ChangeSetSubmitted
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO changesets (id, intent_id, intent_brief, repo_root, base_sha, diff, author, agent_model, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		cs.ID, cs.IntentID, cs.IntentBrief, cs.RepoRoot, cs.BaseSHA, cs.Diff,
		cs.Author, cs.AgentModel, string(cs.Status),
		cs.CreatedAt.Format(rfc3339Nano), cs.UpdatedAt.Format(rfc3339Nano),
	)
	if err != nil {
		return fmt.Errorf("insert changeset: %w", err)
	}
	return nil
}

// UpdateChangeSetStatus transitions a changeset to a new status.
func (s *Store) UpdateChangeSetStatus(ctx context.Context, id string, status core.ChangeSetStatus) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE changesets SET status = ?, updated_at = ? WHERE id = ?
	`, string(status), time.Now().UTC().Format(rfc3339Nano), id)
	if err != nil {
		return fmt.Errorf("update changeset: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// GetChangeSet returns the changeset with the given ID.
func (s *Store) GetChangeSet(ctx context.Context, id string) (*core.ChangeSet, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, intent_id, intent_brief, repo_root, base_sha, diff, author, agent_model, status, created_at, updated_at
		FROM changesets WHERE id = ?
	`, id)
	cs := &core.ChangeSet{}
	var status, createdAt, updatedAt string
	err := row.Scan(&cs.ID, &cs.IntentID, &cs.IntentBrief, &cs.RepoRoot, &cs.BaseSHA, &cs.Diff,
		&cs.Author, &cs.AgentModel, &status, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan changeset: %w", err)
	}
	cs.Status = core.ChangeSetStatus(status)
	cs.CreatedAt, _ = time.Parse(rfc3339Nano, createdAt)
	cs.UpdatedAt, _ = time.Parse(rfc3339Nano, updatedAt)
	return cs, nil
}

// InsertCertificate persists a signed certificate.
func (s *Store) InsertCertificate(ctx context.Context, cert *core.Certificate) error {
	if cert.CreatedAt.IsZero() {
		cert.CreatedAt = time.Now().UTC()
	}
	icrJSON, err := json.Marshal(cert.ICR)
	if err != nil {
		return fmt.Errorf("marshal icr: %w", err)
	}
	polJSON, err := json.Marshal(cert.Policies)
	if err != nil {
		return fmt.Errorf("marshal policies: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO certificates
		  (id, changeset_id, intent_id, admitted_commit_sha, base_sha, icr_json, policies_json,
		   effective_config_hash, policy_version, toolchain_image, signed_by, signature, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		cert.ID, cert.ChangeSetID, cert.IntentID, cert.AdmittedCommitSHA, cert.BaseSHA,
		string(icrJSON), string(polJSON),
		cert.EffectiveConfigHash, cert.PolicyVersion, cert.ToolchainImage,
		cert.SignedBy, cert.Signature, cert.CreatedAt.Format(rfc3339Nano),
	)
	if err != nil {
		return fmt.Errorf("insert certificate: %w", err)
	}
	return nil
}

// GetCertificate returns a certificate by ID.
func (s *Store) GetCertificate(ctx context.Context, id string) (*core.Certificate, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, changeset_id, intent_id, admitted_commit_sha, base_sha, icr_json, policies_json,
		       effective_config_hash, policy_version, toolchain_image, signed_by, signature, created_at
		FROM certificates WHERE id = ?
	`, id)
	return scanCertificate(row)
}

// GetCertificateByCommit returns the certificate that admitted the given commit SHA.
func (s *Store) GetCertificateByCommit(ctx context.Context, sha string) (*core.Certificate, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, changeset_id, intent_id, admitted_commit_sha, base_sha, icr_json, policies_json,
		       effective_config_hash, policy_version, toolchain_image, signed_by, signature, created_at
		FROM certificates WHERE admitted_commit_sha = ?
	`, sha)
	return scanCertificate(row)
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanCertificate(row rowScanner) (*core.Certificate, error) {
	cert := &core.Certificate{}
	var icrJSON, polJSON, createdAt string
	err := row.Scan(&cert.ID, &cert.ChangeSetID, &cert.IntentID, &cert.AdmittedCommitSHA, &cert.BaseSHA,
		&icrJSON, &polJSON, &cert.EffectiveConfigHash, &cert.PolicyVersion, &cert.ToolchainImage,
		&cert.SignedBy, &cert.Signature, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan certificate: %w", err)
	}
	if err := json.Unmarshal([]byte(icrJSON), &cert.ICR); err != nil {
		return nil, fmt.Errorf("unmarshal icr: %w", err)
	}
	if err := json.Unmarshal([]byte(polJSON), &cert.Policies); err != nil {
		return nil, fmt.Errorf("unmarshal policies: %w", err)
	}
	cert.CreatedAt, _ = time.Parse(rfc3339Nano, createdAt)
	return cert, nil
}
