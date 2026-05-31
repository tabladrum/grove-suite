package cert

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"time"
)

// FindingsSchemaVersion is the version stamped on every emitted Finding.
// Bump this when the schema changes; consumers (policy gates, future
// Pre-Flight Autopilot, downstream dashboards) key off this value.
const FindingsSchemaVersion = "relay.findings/v1"

// Severity is the normalized severity level across analyzers.
type Severity string

const (
	// SeverityInfo is informational; never blocks.
	SeverityInfo Severity = "info"
	// SeverityLow is a low-impact issue; surfaced but non-blocking by default.
	SeverityLow Severity = "low"
	// SeverityMedium is a moderate issue; gates may treat as review_required.
	SeverityMedium Severity = "medium"
	// SeverityHigh is a serious issue; gates typically deny.
	SeverityHigh Severity = "high"
	// SeverityCritical is the maximum severity; always blocks.
	SeverityCritical Severity = "critical"
)

// Category classifies a finding into one of the well-known buckets that
// policy gates dispatch on.
type Category string

const (
	// CategorySecret is a leaked credential, API key, or other secret.
	CategorySecret Category = "secret"
	// CategorySAST is a static-analysis security finding (e.g. semgrep).
	CategorySAST Category = "sast"
	// CategoryVuln is a vulnerable dependency (e.g. govulncheck).
	CategoryVuln Category = "vuln"
	// CategoryFileClass is a forbidden file-type/path classification hit.
	CategoryFileClass Category = "fileclass"
)

// Finding is one normalized result from one analyzer. The schema is stable
// and versioned via FindingsSchemaVersion. Multiple analyzers may produce
// findings that point at the same location; the engine deduplicates by
// Fingerprint when computing the certificate.
type Finding struct {
	Schema      string         `json:"schema"`   // always FindingsSchemaVersion
	Analyzer    string         `json:"analyzer"` // "semgrep", "gitleaks", "govulncheck", ...
	Category    Category       `json:"category"`
	RuleID      string         `json:"rule_id"` // analyzer-specific rule identifier
	Severity    Severity       `json:"severity"`
	Path        string         `json:"path,omitempty"`        // file path relative to repo root
	Line        int            `json:"line,omitempty"`        // 1-based
	Column      int            `json:"column,omitempty"`      // 1-based
	Message     string         `json:"message"`               // short human-readable summary
	NextAction  string         `json:"next_action,omitempty"` // Pre-Flight Autopilot hint
	Fingerprint string         `json:"fingerprint"`           // deterministic dedup key
	Extra       map[string]any `json:"extra,omitempty"`       // analyzer-specific data
}

// ComputeFingerprint returns a stable 16-hex-char fingerprint based on the
// (analyzer, ruleID, path, line, message) tuple. Two findings with the same
// fingerprint are considered the same logical issue and may be deduplicated.
func ComputeFingerprint(analyzer, ruleID, path string, line int, message string) string {
	h := sha256.New()
	h.Write([]byte(analyzer))
	h.Write([]byte{0})
	h.Write([]byte(ruleID))
	h.Write([]byte{0})
	h.Write([]byte(path))
	h.Write([]byte{0})
	h.Write([]byte(strconv.Itoa(line)))
	h.Write([]byte{0})
	h.Write([]byte(message))
	sum := h.Sum(nil)
	return hex.EncodeToString(sum[:8])
}

// AnalyzerRun records the outcome of running a single analyzer.
type AnalyzerRun struct {
	Name        string        `json:"name"`
	Available   bool          `json:"available"` // false when the binary was missing
	SkipReason  string        `json:"skip_reason,omitempty"`
	Duration    time.Duration `json:"duration_ns"`
	ErrorMsg    string        `json:"error,omitempty"` // populated when the run failed mid-execution
	NumFindings int           `json:"num_findings"`
}

// Stage2Result aggregates the output of all analyzers that ran in Stage 2.
type Stage2Result struct {
	StartedAt time.Time     `json:"started_at"`
	Duration  time.Duration `json:"duration_ns"`
	Runs      []AnalyzerRun `json:"runs"`
	Findings  []Finding     `json:"findings"`
	// Skipped is set when no analyzer was configured/available at all.
	Skipped    bool   `json:"skipped,omitempty"`
	SkipReason string `json:"skip_reason,omitempty"`
}

// FindingsByCategory groups findings into a map for fast lookup by gates.
func (s Stage2Result) FindingsByCategory() map[Category][]Finding {
	out := map[Category][]Finding{}
	for _, f := range s.Findings {
		out[f.Category] = append(out[f.Category], f)
	}
	return out
}

// HighestSeverity returns the most severe finding's severity, or empty
// string when there are no findings.
func (s Stage2Result) HighestSeverity() Severity {
	rank := map[Severity]int{
		SeverityInfo: 1, SeverityLow: 2, SeverityMedium: 3,
		SeverityHigh: 4, SeverityCritical: 5,
	}
	var best Severity
	bestRank := 0
	for _, f := range s.Findings {
		if r := rank[f.Severity]; r > bestRank {
			best = f.Severity
			bestRank = r
		}
	}
	return best
}

// SortFindings returns a new slice sorted deterministically (by analyzer,
// path, line, ruleID, fingerprint). Used wherever stable ordering matters
// (certificate persistence, test assertions).
func SortFindings(in []Finding) []Finding {
	out := append([]Finding(nil), in...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Analyzer != b.Analyzer {
			return a.Analyzer < b.Analyzer
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.RuleID != b.RuleID {
			return a.RuleID < b.RuleID
		}
		return a.Fingerprint < b.Fingerprint
	})
	return out
}
