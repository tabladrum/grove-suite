package core

// AnalyzerConfig is the per-analyzer block in `.relay/relay.yaml`. The map
// key is the analyzer name (e.g. "semgrep", "sonar", "my-custom"). The
// engine resolves the name through the analyzer factory registry; unknown
// names with Type=="external" are loaded as user-supplied subprocess
// adapters (see internal/analyzers/external).
type AnalyzerConfig struct {
	// Enabled toggles the analyzer. When absent in YAML, defaults to true
	// for built-ins and false for externals.
	Enabled *bool `json:"enabled,omitempty" yaml:"enabled,omitempty"`

	// Type selects the adapter shape:
	//   "" / "builtin" → look up Name in the built-in factory registry.
	//   "external"     → spawn Command/Args and parse JSON findings.
	Type string `json:"type,omitempty" yaml:"type,omitempty"`

	// SeverityFloor drops findings below this severity before they reach
	// the gates. Valid: "info", "low", "medium", "high", "critical".
	SeverityFloor string `json:"severity_floor,omitempty" yaml:"severity_floor,omitempty"`

	// RulePacks is the analyzer-specific list of rule packs to load. For
	// semgrep these become `--config <pack>` invocations (e.g. "auto",
	// "p/security-audit", local paths). For sonar, the first entry is
	// the active quality-profile XML path (relative to .relay/).
	RulePacks []string `json:"rule_packs,omitempty" yaml:"rule_packs,omitempty"`

	// RulesetsFromRelayDir, when true, auto-loads every file under
	// .relay/rulesets/ that the analyzer can natively consume in addition
	// to RulePacks. Default false.
	RulesetsFromRelayDir bool `json:"rulesets_from_relay_dir,omitempty" yaml:"rulesets_from_relay_dir,omitempty"`

	// ScopeOverride lets one analyzer ignore the global Scope (e.g. sonar
	// "all" for full-codebase audits even when Scope="new"). Valid: "",
	// "new", "all".
	ScopeOverride string `json:"scope,omitempty" yaml:"scope,omitempty"`

	// ExtraArgs are appended to the analyzer's invocation (built-in
	// analyzers that honor them pass through). For external analyzers,
	// see Args.
	ExtraArgs []string `json:"extra_args,omitempty" yaml:"extra_args,omitempty"`

	// External-only fields ------------------------------------------------

	// Command is the binary to spawn for Type=="external".
	Command string `json:"command,omitempty" yaml:"command,omitempty"`
	// Args are the argv for an external analyzer. The token "${dir}" is
	// substituted with the worktree path, "${diff}" with a temp file
	// containing the unified diff.
	Args []string `json:"args,omitempty" yaml:"args,omitempty"`
	// Category overrides the cert.Category assigned to external findings
	// (default "sast"). Valid: "sast", "secret", "vuln", "fileclass".
	Category string `json:"category,omitempty" yaml:"category,omitempty"`
}

// IsEnabled reports the effective enabled state honoring builtinDefault.
func (a AnalyzerConfig) IsEnabled(builtinDefault bool) bool {
	if a.Enabled != nil {
		return *a.Enabled
	}
	if a.Type == "external" {
		return false
	}
	return builtinDefault
}

// ScanScope governs whether analyzer findings are filtered to new-code only
// or reported across the full codebase. Applied uniformly post-hoc by the
// engine — analyzers always scan the full worktree; the engine drops
// findings outside the added-line ranges when Scope=="new".
type ScanScope string

const (
	// ScopeAll reports every finding (default).
	ScopeAll ScanScope = "all"
	// ScopeNew reports only findings on lines added by the ChangeSet diff.
	ScopeNew ScanScope = "new"
)

// Normalize returns a valid ScanScope, defaulting to ScopeAll.
func (s ScanScope) Normalize() ScanScope {
	switch s {
	case ScopeNew:
		return ScopeNew
	default:
		return ScopeAll
	}
}
