// Package eslint adapts the eslint CLI into the Stage-2 analyzer contract.
// It runs `eslint --format=json` on the worktree and translates each
// message into a normalized Finding. Unavailable when eslint is not in PATH.
package eslint

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
	analyzers.DefaultRegistry.Register("eslint", func(_ string, cfg core.AnalyzerConfig) (analyzers.Analyzer, error) {
		a := New()
		a.ExtraArgs = cfg.ExtraArgs
		return a, nil
	})
}

// Analyzer wraps `eslint --format=json`.
type Analyzer struct {
	// Patterns are the file/dir arguments passed to eslint. When empty, "."
	// is used (relative to the worktree).
	Patterns []string
	// ExtraArgs are appended after the format flag (e.g. --no-eslintrc).
	ExtraArgs []string
}

// New returns an eslint adapter with default arguments.
func New() *Analyzer { return &Analyzer{} }

// Name implements analyzers.Analyzer.
func (*Analyzer) Name() string { return "eslint" }

// Available reports whether eslint is in PATH.
func (*Analyzer) Available() bool {
	return tools.Available("eslint")
}

// eslintReport is the subset of eslint's --format=json schema we consume.
type eslintReport []struct {
	FilePath string `json:"filePath"`
	Messages []struct {
		RuleID   string `json:"ruleId"`
		Severity int    `json:"severity"` // 0 off, 1 warn, 2 error
		Message  string `json:"message"`
		Line     int    `json:"line"`
		Column   int    `json:"column"`
	} `json:"messages"`
}

// Run executes eslint and parses its JSON output.
func (a *Analyzer) Run(ctx context.Context, _ *core.ChangeSet, dir string) ([]cert.Finding, error) {
	bin := tools.Locate("eslint")
	if bin == "" {
		return nil, nil
	}
	args := []string{"--format=json"}
	args = append(args, a.ExtraArgs...)
	if len(a.Patterns) == 0 {
		args = append(args, ".")
	} else {
		args = append(args, a.Patterns...)
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	// eslint exits non-zero when findings exist; not an error for us.
	_ = cmd.Run()

	if stdout.Len() == 0 {
		return nil, nil
	}
	var rep eslintReport
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		return nil, err
	}
	var out []cert.Finding
	for _, file := range rep {
		path := file.FilePath
		if strings.HasPrefix(path, dir+"/") {
			path = strings.TrimPrefix(path, dir+"/")
		}
		for _, m := range file.Messages {
			rule := m.RuleID
			if rule == "" {
				rule = "eslint:parse-error"
			}
			f := cert.Finding{
				Schema:   cert.FindingsSchemaVersion,
				Analyzer: "eslint",
				Category: cert.CategorySAST,
				RuleID:   rule,
				Severity: mapSeverity(m.Severity),
				Path:     path,
				Line:     m.Line,
				Column:   m.Column,
				Message:  m.Message,
			}
			f.Fingerprint = cert.ComputeFingerprint(f.Analyzer, f.RuleID, f.Path, f.Line, f.Message)
			out = append(out, f)
		}
	}
	return out, nil
}

func mapSeverity(s int) cert.Severity {
	switch s {
	case 2:
		return cert.SeverityHigh
	case 1:
		return cert.SeverityMedium
	default:
		return cert.SeverityInfo
	}
}
