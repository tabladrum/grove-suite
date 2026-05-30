package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/tabladrum/grove-suite/relay/internal/core"
)

// ── RepoStore ─────────────────────────────────────────────────────────────────

type RepoStore struct{ db *sql.DB }

func NewRepoStore(db *sql.DB) *RepoStore { return &RepoStore{db: db} }

func (s *RepoStore) Create(r *core.Repo) error {
	_, err := s.db.Exec(`
		INSERT INTO repos (id, name, url, default_branch, created_at)
		VALUES ($1, $2, $3, $4, $5)`,
		r.ID, r.Name, r.URL, r.DefaultBranch, r.CreatedAt)
	return err
}

func (s *RepoStore) Get(id string) (*core.Repo, error) {
	row := s.db.QueryRow(`SELECT id, name, url, default_branch, created_at FROM repos WHERE id = $1`, id)
	return scanRepo(row)
}

func (s *RepoStore) GetByName(name string) (*core.Repo, error) {
	row := s.db.QueryRow(`SELECT id, name, url, default_branch, created_at FROM repos WHERE name = $1`, name)
	return scanRepo(row)
}

func (s *RepoStore) List() ([]*core.Repo, error) {
	rows, err := s.db.Query(`SELECT id, name, url, default_branch, created_at FROM repos ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*core.Repo
	for rows.Next() {
		r := &core.Repo{}
		if err := rows.Scan(&r.ID, &r.Name, &r.URL, &r.DefaultBranch, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *RepoStore) Delete(id string) error {
	_, err := s.db.Exec(`DELETE FROM repos WHERE id = $1`, id)
	return err
}

func scanRepo(row *sql.Row) (*core.Repo, error) {
	r := &core.Repo{}
	err := row.Scan(&r.ID, &r.Name, &r.URL, &r.DefaultBranch, &r.CreatedAt)
	if err != nil {
		return nil, err
	}
	return r, nil
}

// ── ProjectStore ──────────────────────────────────────────────────────────────

type ProjectStore struct{ db *sql.DB }

func NewProjectStore(db *sql.DB) *ProjectStore { return &ProjectStore{db: db} }

func (s *ProjectStore) Create(p *core.Project) error {
	_, err := s.db.Exec(`
		INSERT INTO projects (id, name, repo_id, source_path, gs_threshold, auto_approve, owner, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		p.ID, p.Name, p.RepoID, p.SourcePath, p.GSThreshold, p.AutoApprove,
		nullString(p.Owner), p.CreatedAt, p.UpdatedAt)
	return err
}

func (s *ProjectStore) Get(id string) (*core.Project, error) {
	row := s.db.QueryRow(projectSelect+` WHERE id = $1`, id)
	return scanProject(row)
}

func (s *ProjectStore) GetByName(name string) (*core.Project, error) {
	row := s.db.QueryRow(projectSelect+` WHERE name = $1`, name)
	return scanProject(row)
}

func (s *ProjectStore) List(repoID string) ([]*core.Project, error) {
	q := projectSelect
	args := []any{}
	if repoID != "" {
		q += ` WHERE repo_id = $1`
		args = append(args, repoID)
	}
	q += ` ORDER BY name`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*core.Project
	for rows.Next() {
		p, err := scanProjectRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *ProjectStore) Delete(id string) error {
	_, err := s.db.Exec(`DELETE FROM projects WHERE id = $1`, id)
	return err
}

const projectSelect = `SELECT id, name, repo_id, source_path, gs_threshold, auto_approve, owner, created_at, updated_at FROM projects`

func scanProject(row *sql.Row) (*core.Project, error) {
	p := &core.Project{}
	var owner sql.NullString
	err := row.Scan(&p.ID, &p.Name, &p.RepoID, &p.SourcePath,
		&p.GSThreshold, &p.AutoApprove, &owner, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	p.Owner = owner.String
	return p, nil
}

func scanProjectRow(row rowScanner) (*core.Project, error) {
	p := &core.Project{}
	var owner sql.NullString
	err := row.Scan(&p.ID, &p.Name, &p.RepoID, &p.SourcePath,
		&p.GSThreshold, &p.AutoApprove, &owner, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	p.Owner = owner.String
	return p, nil
}

// ── IntegrationStore ──────────────────────────────────────────────────────────

type IntegrationStore struct{ db *sql.DB }

func NewIntegrationStore(db *sql.DB) *IntegrationStore { return &IntegrationStore{db: db} }

func (s *IntegrationStore) Add(pi *core.ProjectIntegration) error {
	cfg, err := json.Marshal(pi.Config)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	_, err = s.db.Exec(`
		INSERT INTO project_integrations (id, project_id, type, external_id, config, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (project_id, type, external_id) DO UPDATE SET config = EXCLUDED.config`,
		pi.ID, pi.ProjectID, pi.Type, pi.ExternalID, cfg, pi.CreatedAt)
	return err
}

func (s *IntegrationStore) FindByTypeAndExternalID(intType, externalID string) ([]*core.ProjectIntegration, error) {
	rows, err := s.db.Query(`
		SELECT id, project_id, type, external_id, config, created_at
		FROM project_integrations
		WHERE type = $1 AND external_id = $2`, intType, externalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanIntegrations(rows)
}

func (s *IntegrationStore) FindByProject(projectID string) ([]*core.ProjectIntegration, error) {
	rows, err := s.db.Query(`
		SELECT id, project_id, type, external_id, config, created_at
		FROM project_integrations WHERE project_id = $1`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanIntegrations(rows)
}

func (s *IntegrationStore) Delete(id string) error {
	_, err := s.db.Exec(`DELETE FROM project_integrations WHERE id = $1`, id)
	return err
}

func scanIntegrations(rows *sql.Rows) ([]*core.ProjectIntegration, error) {
	var out []*core.ProjectIntegration
	for rows.Next() {
		pi := &core.ProjectIntegration{}
		var cfgRaw []byte
		if err := rows.Scan(&pi.ID, &pi.ProjectID, &pi.Type, &pi.ExternalID, &cfgRaw, &pi.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(cfgRaw, &pi.Config); err != nil {
			return nil, fmt.Errorf("unmarshal integration config: %w", err)
		}
		out = append(out, pi)
	}
	return out, rows.Err()
}

// ── IntegrationStatus for connectivity checks ─────────────────────────────────

type IntegrationSummary struct {
	ProjectID  string
	Type       string
	ExternalID string
}

func (s *IntegrationStore) AllTypes(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`SELECT DISTINCT type FROM project_integrations ORDER BY type`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ProjectWithRepo joins project + repo for convenience in handlers.
type ProjectWithRepo struct {
	*core.Project
	RepoName          string
	RepoURL           string
	RepoDefaultBranch string
}

func (s *ProjectStore) GetWithRepo(id string) (*ProjectWithRepo, error) {
	row := s.db.QueryRow(`
		SELECT p.id, p.name, p.repo_id, p.source_path, p.gs_threshold, p.auto_approve, p.owner, p.created_at, p.updated_at,
		       r.name, r.url, r.default_branch
		FROM projects p JOIN repos r ON r.id = p.repo_id
		WHERE p.id = $1`, id)

	p := &core.Project{}
	pr := &ProjectWithRepo{Project: p}
	var owner sql.NullString
	err := row.Scan(
		&p.ID, &p.Name, &p.RepoID, &p.SourcePath, &p.GSThreshold, &p.AutoApprove,
		&owner, &p.CreatedAt, &p.UpdatedAt,
		&pr.RepoName, &pr.RepoURL, &pr.RepoDefaultBranch,
	)
	if err != nil {
		return nil, err
	}
	p.Owner = owner.String
	return pr, nil
}

// FindByRepoAndPath is used during Grove indexing to find the right project.
func (s *ProjectStore) FindByRepoAndPath(repoID, path string) (*core.Project, error) {
	row := s.db.QueryRow(projectSelect+` WHERE repo_id = $1 AND source_path = $2`, repoID, path)
	return scanProject(row)
}

// AssignProject updates an unrouted intent's project_id and transitions it.
func (s *IntentStore) AssignProject(id, projectID string, updatedAt time.Time) error {
	_, err := s.db.Exec(`UPDATE intents SET project_id = $1, updated_at = $2 WHERE id = $3`,
		projectID, updatedAt, id)
	return err
}
