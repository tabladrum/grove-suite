// Package intent implements Auto-Intent Capture: the prompt that produced
// the code is captured as a YAML committed alongside the code. See
// docs/design.md §17 for the full flow.
//
// Storage model:
//   - Drafts live at .relay/.cache/intents/INT-{id}.draft.yaml (gitignored,
//     written during the agent session via relay_intent_open).
//   - Promoted intents live at .relay/intents/INT-{id}.yaml (committed; travel
//     with the repo). Promotion happens via relay_intent_close.
//
// Storage discipline is what makes the prompt durable: every AI-generated
// commit carries an Intent-ID trailer that points at the YAML in the repo,
// and the YAML records the verbatim user prompt, agent identity, model
// identity, and acceptance criteria. Reviewers see the prompt in the PR diff
// alongside the code; auditors can replay forever.
package intent

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// SchemaVersion is stamped on every Intent YAML. Bump when the schema changes
// in a way that prior consumers cannot parse.
const SchemaVersion = "relay.intent/v2"

// Status is one of the two terminal states of an Intent.
type Status string

const (
	// StatusDraft is a draft intent owned by the local daemon. Stored under
	// .relay/.cache/intents/. Gitignored. Not yet committed.
	StatusDraft Status = "draft"
	// StatusCommitted is a promoted intent. Stored under .relay/intents/.
	// Committed to the repo; referenced by Intent-ID trailer on the commit.
	StatusCommitted Status = "committed"
)

// OriginatedFrom records who/what produced the intent. Populated by the
// agent at relay_intent_open time; immutable after promotion.
type OriginatedFrom struct {
	Agent           string    `yaml:"agent,omitempty" json:"agent,omitempty"`
	Model           string    `yaml:"model,omitempty" json:"model,omitempty"`
	ConversationTS  time.Time `yaml:"conversation_ts,omitempty" json:"conversation_ts,omitempty"`
	PromptHash      string    `yaml:"prompt_hash,omitempty" json:"prompt_hash,omitempty"`
}

// Intent is the Auto-Intent Capture artifact. Stored as YAML in
// .relay/.cache/intents/ (draft) or .relay/intents/ (committed).
//
// The "Intent-Hash" trailer on the resulting commit is the SHA-256 of the
// canonical YAML serialization of this struct.
type Intent struct {
	Schema             string         `yaml:"schema" json:"schema"`
	ID                 string         `yaml:"id" json:"id"`
	Status             Status         `yaml:"status" json:"status"`
	Title              string         `yaml:"title" json:"title"`
	Description        string         `yaml:"description" json:"description"`
	OriginatedFrom     OriginatedFrom `yaml:"originated_from,omitempty" json:"originated_from,omitempty"`
	Domain             string         `yaml:"domain,omitempty" json:"domain,omitempty"`
	Capability         string         `yaml:"capability,omitempty" json:"capability,omitempty"`
	AllowedPaths       []string       `yaml:"allowed_paths,omitempty" json:"allowed_paths,omitempty"`
	ForbiddenPaths     []string       `yaml:"forbidden_paths,omitempty" json:"forbidden_paths,omitempty"`
	AcceptanceCriteria []string       `yaml:"acceptance_criteria,omitempty" json:"acceptance_criteria,omitempty"`
	AmbiguityPolicy    string         `yaml:"ambiguity_policy,omitempty" json:"ambiguity_policy,omitempty"`
	RiskLevel          string         `yaml:"risk_level,omitempty" json:"risk_level,omitempty"`
	CreatedAt          time.Time      `yaml:"created_at" json:"created_at"`
	CommittedAt        *time.Time     `yaml:"committed_at,omitempty" json:"committed_at,omitempty"`
	CommittedByAgent   string         `yaml:"committed_by_agent,omitempty" json:"committed_by_agent,omitempty"`
}

// Slugify converts a free-text title into a lowercase, kebab-case slug
// suitable for embedding in an intent ID.
func Slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		case r == ' ' || r == '_' || r == '-' || r == '/':
			if !prevDash && b.Len() > 0 {
				b.WriteRune('-')
				prevDash = true
			}
		}
	}
	out := strings.TrimRight(b.String(), "-")
	if len(out) > 60 {
		out = strings.TrimRight(out[:60], "-")
	}
	return out
}

// NewID returns an ID in the form INT-YYYY-MM-DD-<slug>. Uniqueness within
// a day is preserved by appending an index when collisions exist; the caller
// supplies its own collision-avoidance (see Service.draftPathExists).
func NewID(now time.Time, title string) string {
	slug := Slugify(title)
	if slug == "" {
		slug = "intent"
	}
	return fmt.Sprintf("INT-%s-%s", now.UTC().Format("2006-01-02"), slug)
}

// Hash returns the SHA-256 hex digest of the canonical YAML serialization.
// Used to populate the Intent-Hash commit trailer at admission time and to
// detect tampering on audit replay.
func (i *Intent) Hash() (string, error) {
	out, err := yaml.Marshal(i)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(out)
	return hex.EncodeToString(sum[:]), nil
}

// Marshal returns the canonical YAML serialization with a leading comment.
func (i *Intent) Marshal() ([]byte, error) {
	out, err := yaml.Marshal(i)
	if err != nil {
		return nil, err
	}
	header := "# Auto-Intent Capture artifact. The user's prompt that produced this code.\n" +
		"# See docs/design.md §17. Do not hand-edit unless you know what you're doing.\n"
	return append([]byte(header), out...), nil
}

// Unmarshal parses a YAML payload into an Intent.
func Unmarshal(data []byte) (*Intent, error) {
	var i Intent
	if err := yaml.Unmarshal(data, &i); err != nil {
		return nil, err
	}
	if i.Schema == "" {
		return nil, errors.New("intent: missing schema")
	}
	return &i, nil
}

// Service is the on-disk store of intents. It owns the .relay/intents/ and
// .relay/.cache/intents/ directories for a given repo root.
//
// All methods are safe for concurrent use from the daemon's perspective: the
// daemon serializes write access via its Unix-socket request loop. The CLI
// and MCP transports proxy to the daemon on laptop, preserving this invariant.
type Service struct {
	RepoRoot string
	Now      func() time.Time // injectable clock; defaults to time.Now
}

// New returns a Service rooted at the given repo. The repo's .relay/intents/
// and .relay/.cache/intents/ directories are created lazily on first write.
func New(repoRoot string) *Service {
	return &Service{RepoRoot: repoRoot}
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().UTC()
}

func (s *Service) committedDir() string {
	return filepath.Join(s.RepoRoot, ".relay", "intents")
}

func (s *Service) draftDir() string {
	return filepath.Join(s.RepoRoot, ".relay", ".cache", "intents")
}

// Open drafts a new intent from the user's prompt. Caller supplies title and
// description; originatedFrom is populated from MCP request metadata.
//
// The returned Intent is also written to .relay/.cache/intents/INT-{id}.draft.yaml
// (gitignored). Idempotency: calling Open twice with the same title within a
// single day appends -2, -3, ... to the slug.
func (s *Service) Open(title, description string, originatedFrom OriginatedFrom) (*Intent, string, error) {
	if strings.TrimSpace(title) == "" {
		return nil, "", errors.New("intent: title is required")
	}
	if strings.TrimSpace(description) == "" {
		return nil, "", errors.New("intent: description is required")
	}
	if err := os.MkdirAll(s.draftDir(), 0o755); err != nil {
		return nil, "", fmt.Errorf("mkdir draft dir: %w", err)
	}
	now := s.now()
	id := s.uniqueID(now, title)

	// Stamp the prompt hash for tamper detection.
	if originatedFrom.PromptHash == "" {
		sum := sha256.Sum256([]byte(description))
		originatedFrom.PromptHash = hex.EncodeToString(sum[:])
	}

	i := &Intent{
		Schema:         SchemaVersion,
		ID:             id,
		Status:         StatusDraft,
		Title:          strings.TrimSpace(title),
		Description:    description,
		OriginatedFrom: originatedFrom,
		CreatedAt:      now,
	}
	path := s.draftPath(id)
	data, err := i.Marshal()
	if err != nil {
		return nil, "", err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return nil, "", fmt.Errorf("write draft: %w", err)
	}
	return i, path, nil
}

// Update merges patch fields onto an existing draft. Only the draft (not the
// committed intent) is mutable; promoting an intent commits a snapshot.
//
// Allowed fields in patch: title, description, allowed_paths, forbidden_paths,
// acceptance_criteria, domain, capability, ambiguity_policy, risk_level.
func (s *Service) Update(id string, patch map[string]any) (*Intent, error) {
	i, err := s.LoadDraft(id)
	if err != nil {
		return nil, err
	}
	if v, ok := patch["title"].(string); ok && v != "" {
		i.Title = v
	}
	if v, ok := patch["description"].(string); ok && v != "" {
		i.Description = v
	}
	if v, ok := patch["domain"].(string); ok {
		i.Domain = v
	}
	if v, ok := patch["capability"].(string); ok {
		i.Capability = v
	}
	if v, ok := patch["ambiguity_policy"].(string); ok {
		i.AmbiguityPolicy = v
	}
	if v, ok := patch["risk_level"].(string); ok {
		i.RiskLevel = v
	}
	if v, ok := patch["allowed_paths"].([]any); ok {
		i.AllowedPaths = toStringSlice(v)
	}
	if v, ok := patch["forbidden_paths"].([]any); ok {
		i.ForbiddenPaths = toStringSlice(v)
	}
	if v, ok := patch["acceptance_criteria"].([]any); ok {
		i.AcceptanceCriteria = toStringSlice(v)
	}
	data, err := i.Marshal()
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(s.draftPath(id), data, 0o644); err != nil {
		return nil, fmt.Errorf("write draft: %w", err)
	}
	return i, nil
}

// Close promotes a draft to a committed intent. The draft file is removed and
// the committed file is written under .relay/intents/. Returns the committed
// path, the Intent-Hash, and the suggested commit-trailer block.
func (s *Service) Close(id string, agentIdentity string) (string, string, string, error) {
	i, err := s.LoadDraft(id)
	if err != nil {
		return "", "", "", err
	}
	if err := os.MkdirAll(s.committedDir(), 0o755); err != nil {
		return "", "", "", fmt.Errorf("mkdir committed dir: %w", err)
	}
	now := s.now()
	i.Status = StatusCommitted
	i.CommittedAt = &now
	i.CommittedByAgent = agentIdentity
	data, err := i.Marshal()
	if err != nil {
		return "", "", "", err
	}
	committedPath := s.committedPath(id)
	if err := os.WriteFile(committedPath, data, 0o644); err != nil {
		return "", "", "", fmt.Errorf("write committed: %w", err)
	}
	hash, err := i.Hash()
	if err != nil {
		return "", "", "", err
	}
	// Remove the draft once the committed copy is durably written.
	_ = os.Remove(s.draftPath(id))
	trailer := fmt.Sprintf("Intent-ID: %s\nIntent-Hash: sha256:%s\n", id, hash)
	return committedPath, hash, trailer, nil
}

// LoadDraft reads a draft intent from .relay/.cache/intents/.
func (s *Service) LoadDraft(id string) (*Intent, error) {
	data, err := os.ReadFile(s.draftPath(id))
	if err != nil {
		return nil, fmt.Errorf("load draft %s: %w", id, err)
	}
	return Unmarshal(data)
}

// LoadCommitted reads a promoted intent from .relay/intents/.
func (s *Service) LoadCommitted(id string) (*Intent, error) {
	data, err := os.ReadFile(s.committedPath(id))
	if err != nil {
		return nil, fmt.Errorf("load committed %s: %w", id, err)
	}
	return Unmarshal(data)
}

// Entry is a lightweight summary of an intent, returned by List.
type Entry struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Status    Status    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	FilePath  string    `json:"file_path"`
}

// List returns all drafts (Status=="draft"), all committed intents
// (Status=="committed"), or both ("all"). Sorted by CreatedAt descending.
func (s *Service) List(filter string) ([]Entry, error) {
	var entries []Entry
	if filter == "" {
		filter = "all"
	}
	if filter == "draft" || filter == "all" {
		es, err := s.scan(s.draftDir(), StatusDraft, ".draft.yaml")
		if err == nil {
			entries = append(entries, es...)
		}
	}
	if filter == "committed" || filter == "all" {
		es, err := s.scan(s.committedDir(), StatusCommitted, ".yaml")
		if err == nil {
			entries = append(entries, es...)
		}
	}
	sort.Slice(entries, func(a, b int) bool {
		return entries[a].CreatedAt.After(entries[b].CreatedAt)
	})
	return entries, nil
}

func (s *Service) scan(dir string, status Status, suffix string) ([]Entry, error) {
	infos, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var out []Entry
	for _, info := range infos {
		if info.IsDir() || !strings.HasSuffix(info.Name(), suffix) {
			continue
		}
		full := filepath.Join(dir, info.Name())
		data, err := os.ReadFile(full)
		if err != nil {
			continue
		}
		i, err := Unmarshal(data)
		if err != nil {
			continue
		}
		out = append(out, Entry{
			ID:        i.ID,
			Title:     i.Title,
			Status:    status,
			CreatedAt: i.CreatedAt,
			FilePath:  full,
		})
	}
	return out, nil
}

// idSuffixRe matches a trailing -<digit>+ that came from a uniqueID retry.
var idSuffixRe = regexp.MustCompile(`-\d+$`)

func (s *Service) uniqueID(now time.Time, title string) string {
	base := NewID(now, title)
	id := base
	for i := 2; s.draftPathExists(id) || s.committedPathExists(id); i++ {
		id = fmt.Sprintf("%s-%d", base, i)
		if i > 99 {
			break // give up rather than spin
		}
	}
	return id
}

func (s *Service) draftPath(id string) string {
	return filepath.Join(s.draftDir(), id+".draft.yaml")
}

func (s *Service) committedPath(id string) string {
	return filepath.Join(s.committedDir(), id+".yaml")
}

func (s *Service) draftPathExists(id string) bool {
	_, err := os.Stat(s.draftPath(id))
	return err == nil
}

func (s *Service) committedPathExists(id string) bool {
	_, err := os.Stat(s.committedPath(id))
	return err == nil
}

func toStringSlice(in []any) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
