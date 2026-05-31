// Package inlinesecrets is a built-in, zero-dependency secrets scanner that
// runs over the *diff* of a ChangeSet (not the worktree). It is intentionally
// narrow but reliable: it catches obvious leaked credentials patterns that
// agents accidentally commit (AWS keys, GitHub tokens, generic high-entropy
// `secret = "..."` literals). It always reports Available=true so the
// secrets gate has at least one signal even when gitleaks is not installed.
//
// This complements (does NOT replace) the gitleaks adapter; both can run.
package inlinesecrets

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/tabladrum/grove-suite/relay/internal/analyzers"
	"github.com/tabladrum/grove-suite/relay/internal/cert"
	"github.com/tabladrum/grove-suite/relay/internal/core"
)

func init() {
	analyzers.DefaultRegistry.Register("inline-secrets", func(_ string, _ core.AnalyzerConfig) (analyzers.Analyzer, error) {
		return New(), nil
	})
}

// Analyzer scans a unified diff for added lines that match known secret
// patterns. Stateless: the diff is read from the ChangeSet passed to Run.
type Analyzer struct{}

// New returns an inline-secrets analyzer.
func New() *Analyzer { return &Analyzer{} }

// Name implements analyzers.Analyzer.
func (*Analyzer) Name() string { return "inline-secrets" }

// Available is always true: no binary required.
func (*Analyzer) Available() bool { return true }

type rule struct {
	id       string
	pattern  *regexp.Regexp
	severity cert.Severity
	message  string
}

var rules = []rule{
	{
		id:       "aws-access-key-id",
		pattern:  regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
		severity: cert.SeverityCritical,
		message:  "AWS Access Key ID detected",
	},
	{
		id:       "github-token",
		pattern:  regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{36,}`),
		severity: cert.SeverityCritical,
		message:  "GitHub personal-access token detected",
	},
	{
		id:       "private-key-pem",
		pattern:  regexp.MustCompile(`-----BEGIN (RSA|EC|OPENSSH|DSA|PGP) PRIVATE KEY-----`),
		severity: cert.SeverityCritical,
		message:  "PEM-encoded private key material detected",
	},
	{
		id:       "slack-token",
		pattern:  regexp.MustCompile(`xox[abprs]-[A-Za-z0-9-]{10,}`),
		severity: cert.SeverityHigh,
		message:  "Slack token detected",
	},
	{
		id:       "generic-api-key",
		pattern:  regexp.MustCompile(`(?i)(api[_-]?key|secret|token|password)\s*[:=]\s*["']([A-Za-z0-9+/=_\-]{24,})["']`),
		severity: cert.SeverityHigh,
		message:  "Credential-like literal assigned to API key / secret / token / password",
	},
}

// Run scans the changeset diff for matches; dir is unused.
func (a *Analyzer) Run(_ context.Context, cs *core.ChangeSet, _ string) ([]cert.Finding, error) {
	if cs == nil || cs.Diff == "" {
		return nil, nil
	}
	diff := cs.Diff
	var findings []cert.Finding

	currentPath := ""
	addedLineNum := 0
	// minimal hunk-header tracker so we can attach approximate line numbers
	hunkRE := regexp.MustCompile(`^@@\s+-\d+(?:,\d+)?\s+\+(\d+)(?:,\d+)?\s+@@`)

	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "+++ "):
			p := strings.TrimPrefix(line, "+++ ")
			p = strings.TrimSpace(p)
			p = strings.TrimPrefix(p, "b/")
			currentPath = p
			addedLineNum = 0
		case strings.HasPrefix(line, "@@"):
			if m := hunkRE.FindStringSubmatch(line); m != nil {
				if n, err := fmt.Sscanf(m[1], "%d", &addedLineNum); err != nil || n != 1 {
					addedLineNum = 0
				}
			}
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			added := strings.TrimPrefix(line, "+")
			for _, r := range rules {
				if r.pattern.MatchString(added) {
					f := cert.Finding{
						Schema:     cert.FindingsSchemaVersion,
						Analyzer:   "inline-secrets",
						Category:   cert.CategorySecret,
						RuleID:     r.id,
						Severity:   r.severity,
						Path:       currentPath,
						Line:       addedLineNum,
						Message:    r.message,
						NextAction: "remove the secret, rotate it at the provider, and re-submit",
					}
					f.Fingerprint = cert.ComputeFingerprint(f.Analyzer, f.RuleID, f.Path, f.Line, f.Message)
					findings = append(findings, f)
				}
			}
			addedLineNum++
		case strings.HasPrefix(line, " "):
			addedLineNum++
		}
	}
	return findings, nil
}
