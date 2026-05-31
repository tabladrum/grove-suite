// Package gitleaks adapts the gitleaks binary (https://github.com/gitleaks/gitleaks)
// into the Stage-2 analyzer contract. Unavailable when the binary is not in PATH.
package gitleaks

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/tabladrum/grove-suite/relay/internal/cert"
	"github.com/tabladrum/grove-suite/relay/internal/core"
)

// Analyzer wraps `gitleaks detect --no-banner --report-format=json --no-git`.
type Analyzer struct{}

// New returns a gitleaks adapter.
func New() *Analyzer { return &Analyzer{} }

// Name implements analyzers.Analyzer.
func (*Analyzer) Name() string { return "gitleaks" }

// Available reports whether gitleaks is in PATH.
func (*Analyzer) Available() bool {
	_, err := exec.LookPath("gitleaks")
	return err == nil
}

// gitleaksFinding is a subset of the gitleaks JSON schema.
type gitleaksFinding struct {
	RuleID      string `json:"RuleID"`
	Description string `json:"Description"`
	File        string `json:"File"`
	StartLine   int    `json:"StartLine"`
	StartColumn int    `json:"StartColumn"`
	Match       string `json:"Match"`
	Secret      string `json:"Secret"`
}

// Run executes gitleaks against dir and parses its JSON report.
func (a *Analyzer) Run(ctx context.Context, _ *core.ChangeSet, dir string) ([]cert.Finding, error) {
	report := filepath.Join(os.TempDir(), "relay-gitleaks-report.json")
	defer os.Remove(report)

	cmd := exec.CommandContext(ctx, "gitleaks", "detect",
		"--no-banner",
		"--no-git",
		"--report-format=json",
		"--report-path", report,
		"--source", dir,
	)
	// gitleaks exits 1 when findings are detected; that is NOT an error for us.
	_ = cmd.Run()

	data, err := os.ReadFile(report)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // no report written → no findings
		}
		return nil, err
	}
	var raw []gitleaksFinding
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	var out []cert.Finding
	for _, r := range raw {
		f := cert.Finding{
			Schema:     cert.FindingsSchemaVersion,
			Analyzer:   "gitleaks",
			Category:   cert.CategorySecret,
			RuleID:     r.RuleID,
			Severity:   cert.SeverityCritical,
			Path:       r.File,
			Line:       r.StartLine,
			Column:     r.StartColumn,
			Message:    r.Description,
			NextAction: "remove the secret, rotate it at the provider, and re-submit",
		}
		f.Fingerprint = cert.ComputeFingerprint(f.Analyzer, f.RuleID, f.Path, f.Line, f.Message)
		out = append(out, f)
	}
	return out, nil
}
