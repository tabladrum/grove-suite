package core

import "time"

// ChangeSet is the unit of work an agent submits to Relay.
// One ChangeSet ≈ one diff against a single repo head, scoped to a single intent.
// Persisted in the engine store; not the same table as Phase-1 Intent intake.
type ChangeSet struct {
	ID          string // ULID/UUID generated at ingest
	IntentID    string // free-form reference; need not exist in intents table (laptop mode)
	IntentBrief string // short human-readable description shown in cert
	RepoRoot    string // absolute path to repo root at submit time
	BaseSHA     string // commit the diff was generated against
	Diff        string // unified diff payload
	Author      string // agent identifier or human user
	AgentModel  string // e.g. "claude-sonnet-4.5" — empty when human
	Status      ChangeSetStatus
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type ChangeSetStatus string

const (
	ChangeSetSubmitted ChangeSetStatus = "submitted"
	ChangeSetChecked   ChangeSetStatus = "checked"
	ChangeSetCertified ChangeSetStatus = "certified"
	ChangeSetAdmitted  ChangeSetStatus = "admitted"
	ChangeSetRejected  ChangeSetStatus = "rejected"
)

// PolicyVerdict is the per-gate decision.
type PolicyVerdict string

const (
	VerdictAllow          PolicyVerdict = "allow"
	VerdictWarn           PolicyVerdict = "warn"
	VerdictReviewRequired PolicyVerdict = "review_required"
	VerdictDeny           PolicyVerdict = "deny"
)

// PolicyResult is the structured outcome of one gate.
// Aggregated PolicyResults feed both `relay check` output and the certificate.
type PolicyResult struct {
	Gate       string // e.g. "path", "size", "secrets"
	Verdict    PolicyVerdict
	Message    string         // human-readable summary
	NextAction string         // optional Pre-Flight Autopilot hint
	Details    map[string]any // gate-specific evidence
}

// Allowed is true when a result does not block admission.
func (r PolicyResult) Allowed() bool {
	switch r.Verdict {
	case VerdictAllow, VerdictWarn:
		return true
	default:
		return false
	}
}

// ICR is the Intent Change Region returned by Grove plus a confidence score.
type ICR struct {
	Symbols    []string // qualified symbol IDs touched by the diff
	Files      []string // file paths touched
	Edges      int      // count of impact edges considered
	Confidence float64  // 0.0-1.0, derived from Grove edge-confidence weights
	Hash       string   // canonical sha256 of {Symbols, Files} for audit
}

// Certificate is the signed admission record.
// Stored once per admitted ChangeSet; trailers on the linear-main commit reference it.
type Certificate struct {
	ID                  string // ULID/UUID
	ChangeSetID         string
	IntentID            string
	AdmittedCommitSHA   string // commit on relay-main after admission
	BaseSHA             string // pre-rebase HEAD the agent based the work on
	ICR                 ICR
	Policies            []PolicyResult
	EffectiveConfigHash string // sha256 of canonical .relay/ snapshot
	PolicyVersion       string // Relay's policy schema version
	ToolchainImage      string // optional; empty in laptop mode
	SignedBy            string // public-key ID
	Signature           []byte // raw Ed25519 signature over CanonicalBytes
	CreatedAt           time.Time
}

// RelayConfig is the in-memory representation of merged .relay/ configuration.
// The canonical-JSON encoding of this struct (with deterministic field order) is
// what gets sha256-hashed into EffectiveConfigHash.
type RelayConfig struct {
	RelayVersion string                 `json:"relay_version"`
	Stack        string                 `json:"stack,omitempty"`
	Policies     map[string]PolicyBlock `json:"policies"`
	SourcePath   string                 `json:"-"` // discovered location, excluded from hash
}

// PolicyBlock holds the raw, gate-specific options. Each gate decides how to interpret.
type PolicyBlock struct {
	Enabled bool           `json:"enabled"`
	Options map[string]any `json:"options,omitempty"`
}
