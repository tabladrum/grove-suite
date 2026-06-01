package cli

// `relay local init` — one-command bootstrap for a developer machine.
//
// Combines (in order):
//   1. `relay tools install --with-sonar`  — fetch jre, sonarlint-ls, plugins,
//      and Stage-2 analyzer binaries (semgrep, gitleaks, ...).
//   2. Install git pre-commit + pre-push hooks in the workspace repo so every
//      commit gets gitleaks+semgrep scanned and every push consults relay.
//   3. Write agent steering instructions (CLAUDE.md, AGENTS.md, GEMINI.md,
//      .cursorrules, .windsurfrules, .github/copilot-instructions.md) so any
//      MCP-aware or instruction-aware AI agent picks up relay automatically.
//   4. Register the relay MCP server with every detected client
//      (Claude Code, Cursor, Continue, Windsurf, VS Code) and write
//      .vscode/mcp.json for VS Code's native MCP host.
//
// All steps are idempotent. Failures in optional steps (e.g. agent client
// not installed) print a note and continue.

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tabladrum/grove-suite/relay/internal/githook"
	"github.com/tabladrum/grove-suite/relay/internal/tools"
)

// RunLocal dispatches `relay local <sub>`.
func RunLocal(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: relay local <init|uninstall|status> [args]")
		return 1
	}
	switch args[0] {
	case "init":
		return cmdLocalInit(args[1:])
	case "uninstall":
		return cmdLocalUninstall(args[1:])
	case "status":
		return cmdLocalStatus(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown local subcommand: %s\n", args[0])
		return 1
	}
}

func cmdLocalInit(args []string) int {
	fs := flag.NewFlagSet("local init", flag.ContinueOnError)
	repo := fs.String("repo", ".", "path to the workspace git repository")
	skipTools := fs.Bool("skip-tools", false, "skip `relay tools install` (use when binaries are already provisioned)")
	skipHooks := fs.Bool("skip-hooks", false, "skip git pre-commit/pre-push hook installation")
	skipInstr := fs.Bool("skip-instructions", false, "skip writing agent steering files (CLAUDE.md, AGENTS.md, ...)")
	skipMCP := fs.Bool("skip-mcp", false, "skip MCP client registration")
	withSonar := fs.Bool("with-sonar", true, "install SonarLint stack as part of `tools install`")
	force := fs.Bool("force", false, "overwrite non-relay-managed hooks/configs where applicable")
	clients := fs.String("mcp-clients", "claude-code,cursor,continue,windsurf", "comma-separated MCP clients to register with")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	repoAbs, err := filepath.Abs(*repo)
	if err != nil {
		fmt.Fprintln(os.Stderr, "local init:", err)
		return 1
	}
	w := os.Stdout

	// 1. tools install
	if !*skipTools {
		section(w, "tools install")
		var toolArgs []string
		if *withSonar {
			toolArgs = append(toolArgs, "--with-sonar")
		}
		if rc := cmdToolsInstall(toolArgs); rc != 0 {
			fmt.Fprintln(os.Stderr, "local init: tools install reported failures (continuing)")
		}
	}

	// 2. git hooks
	if !*skipHooks {
		section(w, "git hooks")
		if path, err := githook.Install(repoAbs, *force); err != nil {
			fmt.Fprintf(w, "  pre-push:    SKIP (%v)\n", err)
		} else {
			fmt.Fprintf(w, "  pre-push:    %s\n", path)
		}
		if path, err := githook.InstallPreCommit(repoAbs, *force); err != nil {
			fmt.Fprintf(w, "  pre-commit:  SKIP (%v)\n", err)
		} else {
			fmt.Fprintf(w, "  pre-commit:  %s\n", path)
		}
	}

	// 3. agent steering instructions
	if !*skipInstr {
		section(w, "agent instructions")
		written := writeAgentInstructions(repoAbs)
		for _, p := range written {
			fmt.Fprintf(w, "  wrote:       %s\n", p)
		}
	}

	// 4. MCP client registration
	if !*skipMCP {
		section(w, "MCP clients")
		relayBin, _ := os.Executable()
		if relayBin == "" {
			relayBin = "relay"
		}
		for _, id := range splitCSV(*clients) {
			rc := cmdMCPInstallFor([]string{"--repo", repoAbs, "--bin", relayBin, id})
			if rc != 0 {
				fmt.Fprintf(w, "  %s: not registered\n", id)
			}
		}
		// VS Code: write .vscode/mcp.json (native VS Code MCP host).
		if path, err := writeVSCodeMCP(repoAbs, relayBin); err != nil {
			fmt.Fprintf(w, "  vscode:      SKIP (%v)\n", err)
		} else {
			fmt.Fprintf(w, "  vscode:      %s\n", path)
		}
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "local setup ready. Next:")
	fmt.Fprintln(w, "  relay doctor                  # verify install")
	fmt.Fprintln(w, "  relay daemon start            # background daemon for MCP")
	return 0
}

func cmdLocalStatus(args []string) int {
	fs := flag.NewFlagSet("local status", flag.ContinueOnError)
	repo := fs.String("repo", ".", "path to the workspace git repository")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	repoAbs, _ := filepath.Abs(*repo)
	w := os.Stdout

	section(w, "hooks")
	prePushPath := filepath.Join(repoAbs, ".git", "hooks", githook.HookFileName)
	preCommitPath := filepath.Join(repoAbs, ".git", "hooks", githook.PreCommitFileName)
	for label, p := range map[string]string{"pre-push": prePushPath, "pre-commit": preCommitPath} {
		managed, _ := githook.IsManaged(p)
		state := "missing"
		if _, err := os.Stat(p); err == nil {
			if managed {
				state = "relay-managed"
			} else {
				state = "foreign"
			}
		}
		fmt.Fprintf(w, "  %-12s %s (%s)\n", label, state, p)
	}

	section(w, "instructions")
	for _, rel := range agentInstructionFiles() {
		p := filepath.Join(repoAbs, rel)
		state := "missing"
		if _, err := os.Stat(p); err == nil {
			state = "present"
		}
		fmt.Fprintf(w, "  %-40s %s\n", rel, state)
	}

	return 0
}

func cmdLocalUninstall(args []string) int {
	fs := flag.NewFlagSet("local uninstall", flag.ContinueOnError)
	repo := fs.String("repo", ".", "path to the workspace git repository")
	keepTools := fs.Bool("keep-tools", false, "keep relay tool cache (~/.relay/tools)")
	keepHooks := fs.Bool("keep-hooks", false, "keep git pre-commit/pre-push hooks")
	keepInstr := fs.Bool("keep-instructions", false, "keep relay steering blocks in agent instruction files")
	keepMCP := fs.Bool("keep-mcp", false, "keep relay MCP registrations in clients")
	clients := fs.String("mcp-clients", "claude-code,cursor,continue,windsurf,claude-desktop,zed,codex,kiro,vscode", "comma-separated MCP clients to unregister")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	repoAbs, err := filepath.Abs(*repo)
	if err != nil {
		fmt.Fprintln(os.Stderr, "local uninstall:", err)
		return 1
	}
	w := os.Stdout

	if !*keepHooks {
		section(w, "git hooks")
		if removed, err := githook.Uninstall(repoAbs); err != nil {
			fmt.Fprintf(w, "  pre-push:    SKIP (%v)\n", err)
		} else if removed {
			fmt.Fprintln(w, "  pre-push:    removed")
		} else {
			fmt.Fprintln(w, "  pre-push:    not present")
		}
		if removed, err := githook.UninstallPreCommit(repoAbs); err != nil {
			fmt.Fprintf(w, "  pre-commit:  SKIP (%v)\n", err)
		} else if removed {
			fmt.Fprintln(w, "  pre-commit:  removed")
		} else {
			fmt.Fprintln(w, "  pre-commit:  not present")
		}
	}

	if !*keepInstr {
		section(w, "agent instructions")
		removed := removeAgentInstructions(repoAbs)
		if len(removed) == 0 {
			fmt.Fprintln(w, "  no relay steering blocks found")
		} else {
			for _, p := range removed {
				fmt.Fprintf(w, "  cleaned:     %s\n", p)
			}
		}
	}

	if !*keepMCP {
		section(w, "MCP clients")
		relayBin, _ := os.Executable()
		if relayBin == "" {
			relayBin = "relay"
		}
		for _, id := range splitCSV(*clients) {
			rc := cmdMCPInstallFor([]string{"--uninstall", "--repo", repoAbs, "--bin", relayBin, id})
			if rc != 0 {
				fmt.Fprintf(w, "  %s: uninstall failed\n", id)
			}
		}
	}

	if !*keepTools {
		section(w, "tools cache")
		if root, err := tools.DefaultRoot(); err != nil {
			fmt.Fprintf(w, "  tools:       SKIP (%v)\n", err)
		} else if err := os.RemoveAll(root); err != nil {
			fmt.Fprintf(w, "  tools:       SKIP (%v)\n", err)
		} else {
			fmt.Fprintf(w, "  removed:     %s\n", root)
		}
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "local uninstall complete.")
	return 0
}

// agentInstructionFiles lists the per-tool instruction files relay writes.
func agentInstructionFiles() []string {
	return []string{
		"CLAUDE.md",
		"AGENTS.md",
		"GEMINI.md",
		".cursorrules",
		".windsurfrules",
		".github/copilot-instructions.md",
	}
}

// relaySteeringBlock is appended to every agent instruction file. The leading
// marker is what we look for on subsequent runs to detect we've already
// inserted our block, so we never duplicate.
const relaySteeringMarker = "## Relay — certified delivery for autonomous agents"

const relaySteeringBlock = `
` + relaySteeringMarker + `

This workspace uses [Relay](https://github.com/tabladrum/grove-suite/relay) to
gate every change set with deterministic policy + SAST/secret scans before it
lands on main. Agents (and humans using relay-managed git hooks) MUST:

1. **Submit through Relay, not raw git.** Use the MCP tools (` + "`relay_submit`" + `,
   ` + "`relay_check`" + `, ` + "`relay_cert`" + `) — they run the same gates the pre-commit /
   pre-push hooks enforce, but with rich, structured output the agent can act
   on instead of a terse hook failure.

2. **Run ` + "`relay check`" + ` before every commit.** It runs gitleaks + semgrep on
   the staged diff. The pre-commit hook does the same; calling it explicitly
   surfaces findings earlier in the loop.

3. **Treat ` + "`high`" + ` / ` + "`critical`" + ` findings as blocking.** They block both the
   pre-commit hook and ` + "`relay cert`" + `. Fix or annotate; do not ` + "`--no-verify`" + `.

4. **Use ` + "`relay tools status`" + ` to verify analyzer binaries are present** before
   reporting an analyzer "passed".

5. **Capture intent.** Wrap multi-commit work in a ` + "`relay intent open ... / close`" + `
   pair so the certification chain is auditable.

### Tool priority
| When you want to...                | Use...                          |
|------------------------------------|---------------------------------|
| Commit a diff                      | ` + "`relay submit`" + ` (MCP) or pre-commit hook |
| Re-run gates locally               | ` + "`relay check`" + `                   |
| Inspect what blocked a commit      | ` + "`relay cert show`" + `               |
| Verify environment                 | ` + "`relay doctor`" + `                  |
`

// writeAgentInstructions writes the relaySteeringBlock into every standard
// instruction file. Existing content is preserved; we only append if our
// marker is not already present. Returns the paths that were touched.
func writeAgentInstructions(repoRoot string) []string {
	var written []string
	for _, rel := range agentInstructionFiles() {
		p := filepath.Join(repoRoot, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			continue
		}
		existing, _ := os.ReadFile(p)
		if strings.Contains(string(existing), relaySteeringMarker) {
			continue
		}
		var content string
		if len(existing) > 0 {
			content = string(existing) + "\n" + relaySteeringBlock
		} else {
			content = relaySteeringBlock
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			continue
		}
		written = append(written, p)
	}
	return written
}

// removeAgentInstructions removes only Relay's managed steering block from
// agent instruction files. It preserves any pre-existing user content.
func removeAgentInstructions(repoRoot string) []string {
	var cleaned []string
	trimmedBlock := strings.TrimPrefix(relaySteeringBlock, "\n")
	for _, rel := range agentInstructionFiles() {
		p := filepath.Join(repoRoot, rel)
		raw, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		s := string(raw)
		orig := s
		s = strings.ReplaceAll(s, relaySteeringBlock, "\n")
		s = strings.ReplaceAll(s, trimmedBlock, "")
		s = strings.TrimLeft(s, "\n")
		if s == orig {
			continue
		}
		if strings.TrimSpace(s) == "" {
			if err := os.Remove(p); err == nil {
				cleaned = append(cleaned, p)
			}
			continue
		}
		if err := os.WriteFile(p, []byte(s), 0o644); err == nil {
			cleaned = append(cleaned, p)
		}
	}
	return cleaned
}

// writeVSCodeMCP writes the workspace-scoped .vscode/mcp.json so VS Code's
// native MCP host launches the relay stdio server automatically.
func writeVSCodeMCP(repoRoot, relayBin string) (string, error) {
	dir := filepath.Join(repoRoot, ".vscode")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "mcp.json")

	type srv struct {
		Type    string   `json:"type"`
		Command string   `json:"command"`
		Args    []string `json:"args"`
	}
	type doc struct {
		Servers map[string]srv `json:"servers"`
	}

	// Merge with existing file rather than overwriting.
	merged := doc{Servers: map[string]srv{}}
	if raw, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(raw, &merged)
		if merged.Servers == nil {
			merged.Servers = map[string]srv{}
		}
	}
	merged.Servers["relay"] = srv{
		Type:    "stdio",
		Command: relayBin,
		Args:    []string{"mcp", "serve", "--repo", repoRoot},
	}
	out, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// section is provided by doctor.go in the same package.
