package core

import (
	"encoding/json"
	"time"
)

type IntentStatus string

const (
	StatusDraft      IntentStatus = "draft"
	StatusUnrouted   IntentStatus = "unrouted"   // no project resolved; awaiting human assignment
	StatusValidating IntentStatus = "validating"
	StatusNeedsInfo  IntentStatus = "needs_info"
	StatusQueued     IntentStatus = "queued"
	StatusRejected   IntentStatus = "rejected"
)

type IntentSource string

const (
	SourceNative         IntentSource = "native"
	SourceJira           IntentSource = "jira"
	SourceGitHubIssue    IntentSource = "github_issue"
	SourceGitHubProjects IntentSource = "github_projects"
	SourceLinear         IntentSource = "linear"
)

type Intent struct {
	ID            string
	ProjectID     string       // FK → Project; empty while unrouted
	Description   string
	Domain        string       // optional tag for filtering within a project
	Source        IntentSource
	SourceRef     string       // "AUTH-123", "acme/backend#42"
	Status        IntentStatus
	GSScore       float64
	ICRSymbols    int
	Author        string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	ApprovedAt    *time.Time
	ApprovedBy    string
	RejectedAt    *time.Time
	RejectedBy    string
	RejectionNote string
}

type IntentEvent struct {
	ID        int64
	IntentID  string
	Type      string
	Detail    string
	Actor     string
	CreatedAt time.Time
}

// ── Project entities ──────────────────────────────────────────────────────────

type Repo struct {
	ID            string
	Name          string
	URL           string
	DefaultBranch string
	CreatedAt     time.Time
}

type Project struct {
	ID          string
	Name        string
	RepoID      string
	SourcePath  string
	GSThreshold float64
	AutoApprove bool
	Owner       string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// IntegrationConfig is the shared config stored in project_integrations.config JSONB.
// Fields are union of all source-system requirements; unused fields are omitted on serialise.
type IntegrationConfig struct {
	// Jira
	TriggerStatus     string `json:"trigger_status,omitempty"`
	RelayProjectField string `json:"relay_project_field,omitempty"` // Jira custom field name
	ComponentFilter   string `json:"component_filter,omitempty"`

	// GitHub Issues
	LabelTrigger            string `json:"label_trigger,omitempty"`
	RelayProjectLabelPrefix string `json:"relay_project_label_prefix,omitempty"` // e.g. "relay-project:"
	ComponentLabel          string `json:"component_label,omitempty"`

	// GitHub Projects
	ProjectNumber int `json:"project_number,omitempty"`

	// Common
	AutoApprove bool `json:"auto_approve,omitempty"`
}

func (c IntegrationConfig) MarshalDB() ([]byte, error)   { return json.Marshal(c) }
func (c *IntegrationConfig) UnmarshalDB(b []byte) error  { return json.Unmarshal(b, c) }

type ProjectIntegration struct {
	ID         string
	ProjectID  string
	Type       string            // "jira" | "github_issues" | "github_projects" | "linear"
	ExternalID string            // Jira board key, "owner/repo", etc.
	Config     IntegrationConfig
	CreatedAt  time.Time
}

// ── Connector interface ───────────────────────────────────────────────────────

type Connector interface {
	Name() string
	FetchTicket(ref string) (*TicketData, error)
	PostComment(ref, message string) error
	TransitionTicket(ref, toStatus string) error
}

// TicketData is normalised across all source systems.
type TicketData struct {
	Ref              string
	Title            string
	Description      string
	Priority         string
	Labels           []string
	Author           string
	RelayProjectHint string // explicit relay-project value extracted by the connector
	Component        string // component field for A-routing fallback
}
