// Package semgrep adapts the semgrep CLI (https://semgrep.dev) into the
// Stage-2 analyzer contract. Rule packs are supplied via the
// `rule_packs` config block (e.g. ["auto", "p/security-audit",
// "rulesets/my-org.yml"]); each becomes a `--config <pack>` argument.
// When `rulesets_from_relay_dir: true`, every `*.yml`/`*.yaml`/`.json`
// file under .provasign/rulesets/ is appended.
//
// Unavailable when the binary is not in PATH.
package semgrep

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/provasign/provasign/internal/analyzers"
	"github.com/provasign/provasign/internal/cert"
	"github.com/provasign/provasign/internal/core"
	"github.com/provasign/provasign/internal/tools"
)

// Analyzer wraps `semgrep --json --config <RulePack> ...`.
type Analyzer struct {
	// RulePacks is the list of --config values. Empty defaults to ["auto"].
	RulePacks []string
	// LoadFromRulesetsDir, when true, also loads .provasign/rulesets/*.yml
	// (and .yaml/.json) at run time from the worktree root.
	LoadFromRulesetsDir bool
	// ExtraArgs are appended after the config flags.
	ExtraArgs []string
}

// NewWithConfig constructs a Semgrep adapter from its analyzer config block.
// This is the factory the registry uses.
func NewWithConfig(_ string, cfg core.AnalyzerConfig) (analyzers.Analyzer, error) {
	a := &Analyzer{
		RulePacks:           append([]string(nil), cfg.RulePacks...),
		LoadFromRulesetsDir: cfg.RulesetsFromRelayDir,
		ExtraArgs:           append([]string(nil), cfg.ExtraArgs...),
	}
	return a, nil
}

// New returns the zero-config Semgrep adapter (legacy API). Equivalent
// to NewWithConfig("semgrep", AnalyzerConfig{}).
func New() *Analyzer { return &Analyzer{} }

// Name implements analyzers.Analyzer.
func (*Analyzer) Name() string { return "semgrep" }

// Available reports whether semgrep is in PATH.
func (*Analyzer) Available() bool {
	return tools.Available("semgrep")
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
	bin := tools.Locate("semgrep")
	if bin == "" {
		return nil, nil
	}
	packs := append([]string(nil), a.RulePacks...)
	if a.LoadFromRulesetsDir {
		packs = append(packs, discoverRulesets(dir)...)
	}
	if len(packs) == 0 {
		packs = []string{"auto"}
	}

	args := []string{"--json", "--quiet", "--disable-version-check"}
	for _, p := range packs {
		args = append(args, "--config", p)
	}
	args = append(args, a.ExtraArgs...)
	args = append(args, dir)

	cmd := exec.CommandContext(ctx, bin, args...)
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
		return nil, fmt.Errorf("semgrep: parse JSON: %w", err)
	}
	out := make([]cert.Finding, 0, len(rep.Results))
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

// discoverRulesets returns the absolute paths of every
// .provasign/rulesets/*.{yml,yaml,json} file at workDir or any parent up to
// 8 levels. XML profiles are skipped — those belong to the sonar adapter.
func discoverRulesets(workDir string) []string {
	cur := workDir
	for i := 0; i < 8; i++ {
		dir := filepath.Join(cur, ".provasign", "rulesets")
		if entries, err := os.ReadDir(dir); err == nil {
			var out []string
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				ext := strings.ToLower(filepath.Ext(e.Name()))
				if ext == ".yml" || ext == ".yaml" || ext == ".json" {
					out = append(out, filepath.Join(dir, e.Name()))
				}
			}
			if len(out) > 0 {
				return out
			}
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	return nil
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

func init() {
	analyzers.DefaultRegistry.Register("semgrep", NewWithConfig)
}
