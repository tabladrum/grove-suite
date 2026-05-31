// Package external implements the user-supplied subprocess analyzer
// adapter. Any binary that emits a JSON array of {path, line, column,
// rule_id, severity, message} objects on stdout can be wired into Stage 2
// via a `.relay/relay.yaml` entry:
//
//	analyzers:
//	  my-scanner:
//	    type: external
//	    enabled: true
//	    command: my-scanner
//	    args: ["--json", "${dir}"]
//	    category: sast
//	    severity_floor: medium
//
// The placeholders ${dir} and ${diff} are substituted at invocation time
// with the worktree path and a tempfile containing the unified diff.
package external

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/tabladrum/grove-suite/relay/internal/analyzers"
	"github.com/tabladrum/grove-suite/relay/internal/cert"
	"github.com/tabladrum/grove-suite/relay/internal/core"
	"github.com/tabladrum/grove-suite/relay/internal/tools"
)

// Analyzer is the subprocess shim for user-supplied analyzers.
type Analyzer struct {
	name     string
	cfg      core.AnalyzerConfig
	category cert.Category
}

// New constructs an external Analyzer from its config.
func New(name string, cfg core.AnalyzerConfig) (*Analyzer, error) {
	if strings.TrimSpace(cfg.Command) == "" {
		return nil, fmt.Errorf("command is required")
	}
	cat := cert.Category(cfg.Category)
	if cat == "" {
		cat = cert.CategorySAST
	}
	return &Analyzer{name: name, cfg: cfg, category: cat}, nil
}

// Name implements analyzers.Analyzer.
func (a *Analyzer) Name() string { return a.name }

// Available reports whether the configured command is locatable.
func (a *Analyzer) Available() bool {
	return locate(a.cfg.Command) != ""
}

// Run spawns the configured command and parses its stdout as JSON.
func (a *Analyzer) Run(ctx context.Context, cs *core.ChangeSet, dir string) ([]cert.Finding, error) {
	bin := locate(a.cfg.Command)
	if bin == "" {
		return nil, nil
	}
	var diffFile string
	defer func() {
		if diffFile != "" {
			_ = os.Remove(diffFile)
		}
	}()
	args := make([]string, 0, len(a.cfg.Args))
	for _, raw := range a.cfg.Args {
		switch {
		case strings.Contains(raw, "${dir}"):
			args = append(args, strings.ReplaceAll(raw, "${dir}", dir))
		case strings.Contains(raw, "${diff}"):
			if diffFile == "" && cs != nil && cs.Diff != "" {
				f, err := os.CreateTemp("", "relay-external-diff-*.patch")
				if err != nil {
					return nil, fmt.Errorf("write diff file: %w", err)
				}
				if _, err := f.WriteString(cs.Diff); err != nil {
					f.Close()
					return nil, err
				}
				f.Close()
				diffFile = f.Name()
			}
			args = append(args, strings.ReplaceAll(raw, "${diff}", diffFile))
		default:
			args = append(args, raw)
		}
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	// Non-zero exit is tolerated (mirrors built-in adapters); a real
	// execution failure produces no stdout, which we treat as zero
	// findings rather than blocking the pipeline.
	_ = cmd.Run()
	if stdout.Len() == 0 {
		return nil, nil
	}
	var raw []externalFinding
	if err := json.Unmarshal(stdout.Bytes(), &raw); err != nil {
		return nil, fmt.Errorf("external %q: parse JSON: %w", a.name, err)
	}
	out := make([]cert.Finding, 0, len(raw))
	for _, r := range raw {
		path := r.Path
		if strings.HasPrefix(path, dir+"/") {
			path = strings.TrimPrefix(path, dir+"/")
		}
		sev := normalizeSeverity(r.Severity)
		f := cert.Finding{
			Schema:   cert.FindingsSchemaVersion,
			Analyzer: a.name,
			Category: a.category,
			RuleID:   r.RuleID,
			Severity: sev,
			Path:     path,
			Line:     r.Line,
			Column:   r.Column,
			Message:  r.Message,
		}
		f.Fingerprint = cert.ComputeFingerprint(f.Analyzer, f.RuleID, f.Path, f.Line, f.Message)
		out = append(out, f)
	}
	return out, nil
}

type externalFinding struct {
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	RuleID   string `json:"rule_id"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

func normalizeSeverity(s string) cert.Severity {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "critical", "blocker":
		return cert.SeverityCritical
	case "high", "error", "major":
		return cert.SeverityHigh
	case "medium", "warning", "warn", "minor":
		return cert.SeverityMedium
	case "low":
		return cert.SeverityLow
	case "info", "note":
		return cert.SeverityInfo
	default:
		return cert.SeverityMedium
	}
}

// locate honors the relay tools shim dir first, then $PATH (mirrors
// internal/tools.Locate but accepts an absolute path verbatim).
func locate(cmd string) string {
	if strings.ContainsAny(cmd, "/\\") {
		if _, err := os.Stat(cmd); err == nil {
			return cmd
		}
	}
	return tools.Locate(cmd)
}

func init() {
	analyzers.ExternalFactory = func(name string, cfg core.AnalyzerConfig) (analyzers.Analyzer, error) {
		return New(name, cfg)
	}
}
