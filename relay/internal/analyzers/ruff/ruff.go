// Package ruff adapts the ruff CLI (https://docs.astral.sh/ruff/) into the
// Stage-2 analyzer contract. It runs `ruff check --output-format=json` on
// the worktree and translates each diagnostic into a Finding. Unavailable
// when ruff is not in PATH.
package ruff

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"strings"

	"github.com/tabladrum/grove-suite/relay/internal/analyzers"
	"github.com/tabladrum/grove-suite/relay/internal/cert"
	"github.com/tabladrum/grove-suite/relay/internal/core"
	"github.com/tabladrum/grove-suite/relay/internal/tools"
)

func init() {
	analyzers.DefaultRegistry.Register("ruff", func(_ string, cfg core.AnalyzerConfig) (analyzers.Analyzer, error) {
		a := New()
		a.ExtraArgs = cfg.ExtraArgs
		return a, nil
	})
}

// Analyzer wraps `ruff check --output-format=json`.
type Analyzer struct {
	// ExtraArgs are appended after the format flag (e.g. --select=E,F).
	ExtraArgs []string
}

// New returns a ruff adapter with default arguments.
func New() *Analyzer { return &Analyzer{} }

// Name implements analyzers.Analyzer.
func (*Analyzer) Name() string { return "ruff" }

// Available reports whether ruff is in PATH.
func (*Analyzer) Available() bool {
	return tools.Available("ruff")
}

// ruffReport is the subset of ruff's --output-format=json schema we consume.
type ruffReport []struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Filename string `json:"filename"`
	Location struct {
		Row    int `json:"row"`
		Column int `json:"column"`
	} `json:"location"`
}

// Run executes ruff and parses its JSON output.
func (a *Analyzer) Run(ctx context.Context, _ *core.ChangeSet, dir string) ([]cert.Finding, error) {
	bin := tools.Locate("ruff")
	if bin == "" {
		return nil, nil
	}
	args := []string{"check", "--output-format=json"}
	args = append(args, a.ExtraArgs...)
	args = append(args, ".")
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	// ruff exits 1 when findings exist; not an error.
	_ = cmd.Run()

	if stdout.Len() == 0 {
		return nil, nil
	}
	var rep ruffReport
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		return nil, err
	}
	var out []cert.Finding
	for _, r := range rep {
		path := r.Filename
		if strings.HasPrefix(path, dir+"/") {
			path = strings.TrimPrefix(path, dir+"/")
		}
		code := r.Code
		if code == "" {
			code = "ruff:unknown"
		}
		f := cert.Finding{
			Schema:   cert.FindingsSchemaVersion,
			Analyzer: "ruff",
			Category: cert.CategorySAST,
			RuleID:   code,
			Severity: mapSeverity(code),
			Path:     path,
			Line:     r.Location.Row,
			Column:   r.Location.Column,
			Message:  r.Message,
		}
		f.Fingerprint = cert.ComputeFingerprint(f.Analyzer, f.RuleID, f.Path, f.Line, f.Message)
		out = append(out, f)
	}
	return out, nil
}

// mapSeverity translates ruff rule prefixes to coarse severity buckets.
// Ruff doesn't report severities; we use the rule category convention.
// E (pycodestyle errors), F (pyflakes), S (bandit-style security) → High.
// W (warnings) → Medium. Everything else → Info.
func mapSeverity(code string) cert.Severity {
	if code == "" {
		return cert.SeverityInfo
	}
	switch code[0] {
	case 'E', 'F', 'S':
		return cert.SeverityHigh
	case 'W':
		return cert.SeverityMedium
	default:
		return cert.SeverityInfo
	}
}
