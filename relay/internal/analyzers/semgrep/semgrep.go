// Package semgrep adapts the semgrep CLI (https://semgrep.dev) into the
// Stage-2 analyzer contract. By default it runs semgrep's `auto` ruleset,
// which covers OWASP Top-10 style checks across many languages.
// Unavailable when the binary is not in PATH.
package semgrep

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"strings"

	"github.com/tabladrum/grove-suite/relay/internal/cert"
	"github.com/tabladrum/grove-suite/relay/internal/core"
)

// Analyzer wraps `semgrep --json --config <Config>`.
type Analyzer struct {
	// Config is the semgrep --config value (rule pack). Defaults to "auto".
	Config string
}

// New returns a semgrep adapter with default config.
func New() *Analyzer { return &Analyzer{Config: "auto"} }

// Name implements analyzers.Analyzer.
func (*Analyzer) Name() string { return "semgrep" }

// Available reports whether semgrep is in PATH.
func (*Analyzer) Available() bool {
	_, err := exec.LookPath("semgrep")
	return err == nil
}

// semgrepReport is a subset of semgrep's JSON schema.
type semgrepReport struct {
	Results []struct {
		CheckID string `json:"check_id"`
		Path    string `json:"path"`
		Start   struct {
			Line int `json:"line"`
			Col  int `json:"col"`
		} `json:"start"`
		Extra struct {
			Message  string `json:"message"`
			Severity string `json:"severity"` // INFO | WARNING | ERROR
		} `json:"extra"`
	} `json:"results"`
}

// Run executes semgrep against dir and parses its JSON output.
func (a *Analyzer) Run(ctx context.Context, _ *core.ChangeSet, dir string) ([]cert.Finding, error) {
	cfg := a.Config
	if cfg == "" {
		cfg = "auto"
	}
	cmd := exec.CommandContext(ctx, "semgrep",
		"--json",
		"--quiet",
		"--disable-version-check",
		"--config", cfg,
		dir,
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	// semgrep exits 1 when findings are present; not an error for us.
	_ = cmd.Run()

	if stdout.Len() == 0 {
		return nil, nil
	}
	var rep semgrepReport
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		return nil, err
	}
	var out []cert.Finding
	for _, r := range rep.Results {
		sev := mapSeverity(r.Extra.Severity)
		path := r.Path
		if strings.HasPrefix(path, dir+"/") {
			path = strings.TrimPrefix(path, dir+"/")
		}
		f := cert.Finding{
			Schema:   cert.FindingsSchemaVersion,
			Analyzer: "semgrep",
			Category: cert.CategorySAST,
			RuleID:   r.CheckID,
			Severity: sev,
			Path:     path,
			Line:     r.Start.Line,
			Column:   r.Start.Col,
			Message:  r.Extra.Message,
		}
		f.Fingerprint = cert.ComputeFingerprint(f.Analyzer, f.RuleID, f.Path, f.Line, f.Message)
		out = append(out, f)
	}
	return out, nil
}

func mapSeverity(s string) cert.Severity {
	switch strings.ToUpper(s) {
	case "ERROR":
		return cert.SeverityHigh
	case "WARNING":
		return cert.SeverityMedium
	case "INFO":
		return cert.SeverityInfo
	default:
		return cert.SeverityLow
	}
}
