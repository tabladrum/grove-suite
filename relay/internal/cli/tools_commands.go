package cli

// `relay tools <install|uninstall|list|which>` — manage Stage-2 binaries.

import (
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/tabladrum/grove-suite/relay/internal/analyzers/sonar"
	"github.com/tabladrum/grove-suite/relay/internal/config"
	"github.com/tabladrum/grove-suite/relay/internal/core"
	"github.com/tabladrum/grove-suite/relay/internal/tools"
)

// RunTools dispatches `relay tools <sub>`.
func RunTools(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: relay tools <install|uninstall|list|status|which> [args]")
		return 1
	}
	switch args[0] {
	case "install":
		return cmdToolsInstall(args[1:])
	case "uninstall":
		return cmdToolsUninstall(args[1:])
	case "list":
		return cmdToolsList(args[1:])
	case "status":
		return cmdToolsStatus(args[1:])
	case "which":
		return cmdToolsWhich(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown tools subcommand: %s\n", args[0])
		return 1
	}
}

func cmdToolsInstall(args []string) int {
	fs := flag.NewFlagSet("tools install", flag.ContinueOnError)
	only := fs.String("only", "", "comma-separated subset of tools to install (default: lean set)")
	root := fs.String("root", "", "override install root (default: $RELAY_TOOLS_ROOT or ~/.relay/tools)")
	withSonar := fs.Bool("with-sonar", false, "also install the SonarLint stack (Eclipse Temurin JRE + sonarlint-ls.jar + analyzer plugins)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	inst, err := newInstaller(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	names := selectTools(*only, *withSonar)
	if len(names) == 0 {
		fmt.Fprintln(os.Stderr, "no tools selected")
		return 1
	}

	var failed int
	for _, name := range names {
		// Tools distributed via language package managers can't be
		// fetched as single binaries — print the recommended install
		// command instead of an unhelpful "not in registry" error.
		if hint, ok := packageManagerHint(name); ok {
			fmt.Printf("→ %s ... external (install with: %s)\n", name, hint)
			continue
		}
		// Honor an existing compatible JRE: skip the ~200 MB Temurin
		// download when the user already has Java >= MinJavaMajor on
		// PATH or via $JAVA_HOME. They can force a re-install by
		// passing --only=jre.
		if name == "jre" && *only == "" {
			if existing := sonar.JavaPath(); existing != "" {
				fmt.Printf("→ jre ... using existing %s (skip bundle)\n", existing)
				continue
			}
		}
		fmt.Printf("→ installing %s ... ", name)
		path, err := inst.Install(name)
		if err != nil {
			fmt.Printf("FAILED\n  %v\n", err)
			failed++
			continue
		}
		fmt.Printf("ok\n  binary: %s\n", path)
	}
	if failed > 0 {
		fmt.Fprintf(os.Stderr, "%d/%d installations failed\n", failed, len(names))
		return 1
	}
	return 0
}

func cmdToolsUninstall(args []string) int {
	fs := flag.NewFlagSet("tools uninstall", flag.ContinueOnError)
	root := fs.String("root", "", "override install root (default: $RELAY_TOOLS_ROOT or ~/.relay/tools)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	inst, err := newInstaller(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := os.RemoveAll(inst.Root); err != nil {
		fmt.Fprintln(os.Stderr, "remove tools root:", err)
		return 1
	}
	fmt.Printf("removed relay tools cache: %s\n", inst.Root)
	return 0
}

func cmdToolsList(args []string) int {
	fs := flag.NewFlagSet("tools list", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return 1
	}
	printToolsList(os.Stdout)
	return 0
}

// cmdToolsStatus shows which analyzers are wired in relay.yaml and which
// of their underlying binaries are installed. Designed so a user can
// answer "why isn't sonar running?" without grepping logs.
func cmdToolsStatus(args []string) int {
	fs := flag.NewFlagSet("tools status", flag.ContinueOnError)
	startDir := fs.String("dir", ".", "start directory for .relay/ discovery")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	printToolsStatus(os.Stdout, *startDir)
	return 0
}

func cmdToolsWhich(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: relay tools which <name>")
		return 1
	}
	name := args[0]
	path := tools.Locate(name)
	if path == "" {
		fmt.Fprintf(os.Stderr, "%s: not found (run `relay tools install`)\n", name)
		return 1
	}
	fmt.Println(path)
	return 0
}

func newInstaller(rootOverride string) (*tools.Installer, error) {
	if rootOverride != "" {
		return &tools.Installer{Root: rootOverride}, nil
	}
	return tools.NewInstaller()
}

// printToolsStatus joins the analyzer config from .relay/relay.yaml with
// the on-disk tool availability and prints one row per configured
// analyzer. Designed to answer "why isn't sonar/semgrep running?" at a
// glance.
func printToolsStatus(w io.Writer, startDir string) {
	cfg, err := config.LoadRelayConfig(startDir)
	if err != nil {
		fmt.Fprintf(w, "no .relay/relay.yaml found from %s (using defaults)\n\n", startDir)
		cfg = nil
	} else {
		fmt.Fprintf(w, "config: %s\n", cfg.SourcePath)
		fmt.Fprintf(w, "scope:  %s\n\n", cfg.Scope.Normalize())
	}
	rows := analyzerStatusRows(cfg)
	fmt.Fprintf(w, "  %-16s %-9s %-9s %s\n", "ANALYZER", "ENABLED", "BINARY", "NOTES")
	for _, r := range rows {
		fmt.Fprintf(w, "  %-16s %-9s %-9s %s\n", r.name, r.enabled, r.binary, r.notes)
	}
}

type analyzerStatusRow struct {
	name, enabled, binary, notes string
}

func analyzerStatusRows(cfg *core.RelayConfig) []analyzerStatusRow {
	configured := map[string]core.AnalyzerConfig{}
	if cfg != nil {
		configured = cfg.Analyzers
	}
	// Stable display order: builtins first (in their canonical sequence),
	// then user-configured externals lexicographically.
	builtinOrder := []string{"inline-secrets", "gitleaks", "semgrep", "govulncheck", "eslint", "ruff", "sonar"}
	seen := map[string]bool{}
	var rows []analyzerStatusRow
	for _, name := range builtinOrder {
		seen[name] = true
		rows = append(rows, statusRow(name, configured[name]))
	}
	var extras []string
	for name := range configured {
		if !seen[name] {
			extras = append(extras, name)
		}
	}
	sort.Strings(extras)
	for _, name := range extras {
		rows = append(rows, statusRow(name, configured[name]))
	}
	return rows
}

func statusRow(name string, cfg core.AnalyzerConfig) analyzerStatusRow {
	row := analyzerStatusRow{name: name}
	enabled := cfg.IsEnabled(true)
	if cfg.Type == "external" {
		enabled = cfg.IsEnabled(false)
	}
	if enabled {
		row.enabled = "yes"
	} else {
		row.enabled = "no"
	}
	// Sonar is special: it doesn't have a single binary on PATH. It
	// needs BOTH a Java >=17 and the sonarlint-ls.jar. Report the
	// actual JRE source (bundled / JAVA_HOME / PATH) so users know
	// whether they can skip --with-sonar.
	if name == "sonar" {
		row.binary, row.notes = sonarBinaryStatus()
		return row
	}
	binaryName, hint := analyzerBinary(name, cfg)
	if binaryName == "" {
		row.binary = "n/a"
	} else if tools.Available(binaryName) {
		row.binary = "ok"
	} else {
		row.binary = "missing"
		if hint != "" {
			row.notes = hint
		}
	}
	return row
}

// sonarBinaryStatus probes both halves of the sonar stack (JRE + jar)
// and returns a (binary, notes) pair for the status grid. The notes
// column tells the user which install step is still missing.
func sonarBinaryStatus() (string, string) {
	java := sonar.JavaPath()
	jar := sonar.WrapperPath()
	switch {
	case java != "" && jar != "":
		return "ok", "java=" + java
	case java == "" && jar == "":
		return "missing", "run `relay tools install --with-sonar`"
	case java == "":
		return "partial", fmt.Sprintf("jar ok; need Java >= %d (set JAVA_HOME or run install --with-sonar)", sonar.MinJavaMajor)
	default:
		return "partial", "java ok (" + java + "); need sonarlint-ls.jar (run install --with-sonar)"
	}
}

// analyzerBinary maps an analyzer name to the binary `tools.Available`
// should be queried for, plus an optional install hint.
func analyzerBinary(name string, cfg core.AnalyzerConfig) (string, string) {
	switch name {
	case "inline-secrets":
		return "", "" // built-in, no binary
	case "gitleaks":
		return "gitleaks", "run `relay tools install`"
	case "semgrep":
		return "semgrep", "pipx install semgrep"
	case "govulncheck":
		return "govulncheck", "run `relay tools install`"
	case "eslint":
		return "eslint", "npm install -g eslint"
	case "ruff":
		return "ruff", "pipx install ruff"
	case "sonar":
		// Handled separately in statusRow; "jre" here is a placeholder
		// the caller never reaches.
		return "", ""
	}
	if cfg.Type == "external" && cfg.Command != "" {
		return cfg.Command, "external — install per analyzer docs"
	}
	return "", ""
}

func selectTools(only string, withSonar bool) []string {
	if only == "" {
		// Default install set is "lean" (no JVM). --with-sonar adds the
		// SonarLint Core stack.
		base := make([]string, 0, len(tools.KnownTools))
		for _, t := range tools.KnownTools {
			base = append(base, t.Name)
		}
		if withSonar {
			for _, t := range tools.SonarTools {
				base = append(base, t.Name)
			}
		}
		return base
	}
	parts := strings.Split(only, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !tools.IsKnown(p) {
			fmt.Fprintf(os.Stderr, "warning: %q is not a known tool, skipping\n", p)
			continue
		}
		out = append(out, p)
	}
	return out
}

func printToolsList(w io.Writer) {
	for _, t := range tools.AllInstallableTools() {
		status := "missing"
		if tools.Available(t.Name) {
			status = "ok"
		}
		fmt.Fprintf(w, "  %-16s [%s]  %s\n", t.Name, status, t.Description)
	}
}

// packageManagerHint returns the recommended install command for tools
// that aren't shipped as standalone binaries.
func packageManagerHint(name string) (string, bool) {
	switch name {
	case "semgrep":
		return "pipx install semgrep", true
	case "ruff":
		return "pipx install ruff", true
	case "eslint":
		return "npm install -g eslint", true
	}
	return "", false
}
