// Package tools manages Relay's batteries-included external binaries
// (semgrep, gitleaks, govulncheck, golangci-lint, ...).
//
// Two kinds of state:
//   - Registry: a compile-time table of (name, version, OS, arch) → (URL,
//     SHA-256, extracted-binary-path-inside-archive). Bumped by `make
//     tools-sync`. Pinned so an offline install is reproducible.
//   - Install dir: `~/.provasign/tools/<name>/<version>/<binary>` plus a
//     `~/.provasign/tools/bin/<name>` symlink (or copy on Windows). Locate()
//     prefers this over $PATH so all Relay subcommands see the pinned set.
package tools

// Tool is one named binary that Stage 2 (or any other relay subsystem) can use.
type Tool struct {
	// Name is the user-facing identifier (e.g. "gitleaks"). Used as the
	// directory name under ~/.provasign/tools and as the symlink/binary name.
	Name string
	// Description is one line, shown in `provasign tools list`.
	Description string
}

// KnownTools is the registered set of tools Relay manages. Stage-2 adapter
// packages register here so `tools list/install/which` covers every analyzer
// without hard-coding names in the CLI.
var KnownTools = []Tool{
	{Name: "gitleaks", Description: "secrets scanner (https://github.com/gitleaks/gitleaks)"},
	{Name: "semgrep", Description: "multi-language SAST (https://semgrep.dev) — install via pipx"},
	{Name: "govulncheck", Description: "Go vulnerability scanner (golang.org/x/vuln)"},
	{Name: "golangci-lint", Description: "Go meta-linter (https://golangci-lint.run)"},
	{Name: "eslint", Description: "JS/TS linter (https://eslint.org) — install via npm"},
	{Name: "ruff", Description: "Python linter (https://docs.astral.sh/ruff) — install via pipx"},
}

// SonarTools is the set of components installed by `provasign tools install
// --with-sonar`: a portable JRE plus SonarSource's published SonarLint
// Language Server jar plus the per-language analyzer plugin jars. The
// launch contract matches sonarlint-vscode exactly (java -jar
// sonarlint-ls.jar -stdio -analyzers <plugins...>). Listed separately
// so the lean default install doesn't pull the JVM for users who don't
// want SonarQube rule fidelity.
var SonarTools = []Tool{
	{Name: "jre", Description: "Eclipse Temurin JRE 21 (bundled for SonarLint Language Server)"},
	{Name: "sonarlint-ls", Description: "SonarLint Language Server (the jar VS Code's SonarLint extension bundles)"},
	{Name: "sonar-java", Description: "SonarSource Java analyzer plugin"},
	{Name: "sonar-java-symbolic-execution", Description: "SonarSource Java symbolic-execution analyzer plugin"},
	{Name: "sonar-javascript", Description: "SonarSource JavaScript/TypeScript analyzer plugin"},
	{Name: "sonar-python", Description: "SonarSource Python analyzer plugin"},
	{Name: "sonar-go", Description: "SonarSource Go analyzer plugin"},
	{Name: "sonar-text", Description: "SonarSource text + secrets analyzer plugin"},
	{Name: "sonar-html", Description: "SonarSource HTML/web analyzer plugin"},
	{Name: "sonar-xml", Description: "SonarSource XML analyzer plugin"},
	{Name: "sonar-iac", Description: "SonarSource IaC analyzer plugin (CloudFormation/Terraform/Kubernetes/Docker)"},
}

// AllInstallableTools returns KnownTools followed by SonarTools.
func AllInstallableTools() []Tool {
	out := make([]Tool, 0, len(KnownTools)+len(SonarTools))
	out = append(out, KnownTools...)
	out = append(out, SonarTools...)
	return out
}

// IsKnown reports whether name is in the registry (lean default + sonar).
func IsKnown(name string) bool {
	for _, t := range AllInstallableTools() {
		if t.Name == name {
			return true
		}
	}
	return false
}
