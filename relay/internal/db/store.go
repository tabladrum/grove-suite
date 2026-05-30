package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/tabladrum/grove-suite/relay/internal/core"
)

type IntentStore struct {
	db *sql.DB
}

func NewIntentStore(db *sql.DB) *IntentStore {
	return &IntentStore{db: db}
}

func (s *IntentStore) DB() *sql.DB { return s.db }

func (s *IntentStore) Create(intent *core.Intent) error {
	_, err := s.db.Exec(`
		INSERT INTO intents (id, project_id, description, domain, source, source_ref, status,
		                     gs_score, icr_symbols, author, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		intent.ID, nullString(intent.ProjectID), intent.Description, intent.Domain,
		intent.Source, nullString(intent.SourceRef),
		intent.Status, intent.GSScore, intent.ICRSymbols, intent.Author,
		intent.CreatedAt, intent.UpdatedAt,
	)
	return err
}

func (s *IntentStore) Get(id string) (*core.Intent, error) {
	row := s.db.QueryRow(`
		SELECT id, project_id, description, domain, source, source_ref, status, gs_score, icr_symbols,
		       author, approved_by, approved_at, rejected_by, rejected_at, rejection_note,
		       created_at, updated_at
		FROM intents WHERE id = $1`, id)
	return scanIntent(row)
}

type ListFilter struct {
	ProjectID string
	Status    string
	Domain    string
	Source    string
	Limit     int
	Offset    int
}

const intentSelect = `SELECT id, project_id, description, domain, source, source_ref, status, gs_score, icr_symbols,
       author, approved_by, approved_at, rejected_by, rejected_at, rejection_note,
       created_at, updated_at FROM intents`

func (s *IntentStore) List(f ListFilter) ([]*core.Intent, error) {
	q := intentSelect + ` WHERE 1=1`
	args := []any{}
	n := 1
	if f.ProjectID != "" {
		q += fmt.Sprintf(" AND project_id = $%d", n)
		args = append(args, f.ProjectID)
		n++
	}
	if f.Status != "" {
		q += fmt.Sprintf(" AND status = $%d", n)
		args = append(args, f.Status)
		n++
	}
	if f.Domain != "" {
		q += fmt.Sprintf(" AND domain = $%d", n)
		args = append(args, f.Domain)
		n++
	}
	if f.Source != "" {
		q += fmt.Sprintf(" AND source = $%d", n)
		args = append(args, f.Source)
		n++
	}
	q += " ORDER BY created_at DESC"
	if f.Limit > 0 {
		q += fmt.Sprintf(" LIMIT $%d", n)
		args = append(args, f.Limit)
		n++
	}
	if f.Offset > 0 {
		q += fmt.Sprintf(" OFFSET $%d", n)
		args = append(args, f.Offset)
	}

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*core.Intent
	for rows.Next() {
		i, err := scanIntentRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func (s *IntentStore) ListUnrouted(limit int) ([]*core.Intent, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(intentSelect+` WHERE status = 'unrouted' ORDER BY created_at ASC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*core.Intent
	for rows.Next() {
		i, err := scanIntentRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func (s *IntentStore) UpdateStatus(id string, status core.IntentStatus, updatedAt time.Time) error {
	_, err := s.db.Exec(`UPDATE intents SET status = $1, updated_at = $2 WHERE id = $3`,
		status, updatedAt, id)
	return err
}

func (s *IntentStore) UpdateGS(id string, score float64, icrSymbols int) error {
	_, err := s.db.Exec(`UPDATE intents SET gs_score = $1, icr_symbols = $2, updated_at = NOW() WHERE id = $3`,
		score, icrSymbols, id)
	return err
}

func (s *IntentStore) Approve(id, approvedBy string, at time.Time) error {
	_, err := s.db.Exec(`
		UPDATE intents SET status = $1, approved_by = $2, approved_at = $3, updated_at = $3
		WHERE id = $4`, core.StatusQueued, approvedBy, at, id)
	return err
}

func (s *IntentStore) Reject(id, rejectedBy, note string, at time.Time) error {
	_, err := s.db.Exec(`
		UPDATE intents SET status = $1, rejected_by = $2, rejected_at = $3, rejection_note = $4, updated_at = $3
		WHERE id = $5`, core.StatusRejected, rejectedBy, at, note, id)
	return err
}

func (s *IntentStore) AppendEvent(ev *core.IntentEvent) error {
	_, err := s.db.Exec(`
		INSERT INTO intent_events (intent_id, type, detail, actor, created_at)
		VALUES ($1, $2, $3, $4, $5)`,
		ev.IntentID, ev.Type, nullString(ev.Detail), nullString(ev.Actor), ev.CreatedAt,
	)
	return err
}

func (s *IntentStore) Events(intentID string) ([]*core.IntentEvent, error) {
	rows, err := s.db.Query(`
		SELECT id, intent_id, type, detail, actor, created_at
		FROM intent_events WHERE intent_id = $1 ORDER BY created_at ASC`, intentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*core.IntentEvent
	for rows.Next() {
		ev := &core.IntentEvent{}
		var detail, actor sql.NullString
		if err := rows.Scan(&ev.ID, &ev.IntentID, &ev.Type, &detail, &actor, &ev.CreatedAt); err != nil {
			return nil, err
		}
		ev.Detail = detail.String
		ev.Actor = actor.String
		out = append(out, ev)
	}
	return out, rows.Err()
}

func (s *IntentStore) CountByStatus() (map[string]int, error) {
	rows, err := s.db.Query(`SELECT status, COUNT(*) FROM intents GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := map[string]int{}
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		counts[status] = count
	}
	return counts, rows.Err()
}

func nullString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}

func scanIntent(row *sql.Row) (*core.Intent, error) {
	i := &core.Intent{}
	var projectID, sourceRef, approvedBy, rejectedBy, rejectionNote sql.NullString
	var approvedAt, rejectedAt sql.NullTime
	err := row.Scan(
		&i.ID, &projectID, &i.Description, &i.Domain, &i.Source, &sourceRef, &i.Status,
		&i.GSScore, &i.ICRSymbols, &i.Author,
		&approvedBy, &approvedAt, &rejectedBy, &rejectedAt, &rejectionNote,
		&i.CreatedAt, &i.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	i.ProjectID = projectID.String
	i.SourceRef = sourceRef.String
	i.ApprovedBy = approvedBy.String
	i.RejectedBy = rejectedBy.String
	i.RejectionNote = rejectionNote.String
	if approvedAt.Valid {
		i.ApprovedAt = &approvedAt.Time
	}
	if rejectedAt.Valid {
		i.RejectedAt = &rejectedAt.Time
	}
	return i, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanIntentRow(row rowScanner) (*core.Intent, error) {
	i := &core.Intent{}
	var projectID, sourceRef, approvedBy, rejectedBy, rejectionNote sql.NullString
	var approvedAt, rejectedAt sql.NullTime
	err := row.Scan(
		&i.ID, &projectID, &i.Description, &i.Domain, &i.Source, &sourceRef, &i.Status,
		&i.GSScore, &i.ICRSymbols, &i.Author,
		&approvedBy, &approvedAt, &rejectedBy, &rejectedAt, &rejectionNote,
		&i.CreatedAt, &i.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	i.ProjectID = projectID.String
	i.SourceRef = sourceRef.String
	i.ApprovedBy = approvedBy.String
	i.RejectedBy = rejectedBy.String
	i.RejectionNote = rejectionNote.String
	if approvedAt.Valid {
		i.ApprovedAt = &approvedAt.Time
	}
	if rejectedAt.Valid {
		i.RejectedAt = &rejectedAt.Time
	}
	return i, nil
}
