// Package tools manages Relay's batteries-included external binaries
// (semgrep, gitleaks, govulncheck, golangci-lint, ...).
//
// Two kinds of state:
//   - Registry: a compile-time table of (name, version, OS, arch) → (URL,
//     SHA-256, extracted-binary-path-inside-archive). Bumped by `make
//     tools-sync`. Pinned so an offline install is reproducible.
//   - Install dir: `~/.relay/tools/<name>/<version>/<binary>` plus a
//     `~/.relay/tools/bin/<name>` symlink (or copy on Windows). Locate()
//     prefers this over $PATH so all Relay subcommands see the pinned set.
package tools

// Tool is one named binary that Stage 2 (or any other relay subsystem) can use.
type Tool struct {
	// Name is the user-facing identifier (e.g. "gitleaks"). Used as the
	// directory name under ~/.relay/tools and as the symlink/binary name.
	Name string
	// Description is one line, shown in `relay tools list`.
	Description string
}

// KnownTools is the registered set of tools Relay manages. Stage-2 adapter
// packages register here so `tools list/install/which` covers every analyzer
// without hard-coding names in the CLI.
var KnownTools = []Tool{
	{Name: "gitleaks", Description: "secrets scanner (https://github.com/gitleaks/gitleaks)"},
	{Name: "semgrep", Description: "multi-language SAST (https://semgrep.dev)"},
	{Name: "govulncheck", Description: "Go vulnerability scanner (golang.org/x/vuln)"},
	{Name: "golangci-lint", Description: "Go meta-linter (https://golangci-lint.run)"},
}

// IsKnown reports whether name is in the registry.
func IsKnown(name string) bool {
	for _, t := range KnownTools {
		if t.Name == name {
			return true
		}
	}
	return false
}
