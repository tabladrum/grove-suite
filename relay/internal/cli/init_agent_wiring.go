package cli

// init_agent_wiring.go — called by `relay init` to auto-wire the Relay MCP
// server and Pre-Flight Autopilot instructions into every detected AI coding
// tool in the project directory. Mirrors how `prism init` handles Prism.

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// relaySteeringInstructions is appended to per-agent instruction files
// (CLAUDE.md, .cursorrules, AGENTS.md, etc.) so any agent that opens the
// project immediately knows the Pre-Flight Autopilot workflow.
const relaySteeringInstructions = `
## Relay — certified merge gate (ALWAYS use these tools)

This project uses [Relay](https://github.com/tabladrum/grove-suite) for
certified code admission. Relay MCP tools are registered. Follow this
workflow on EVERY coding task:

### Pre-Flight Autopilot

1. **Open an intent BEFORE making code changes** — call relay_intent_open with
   {title: <short summary>, description: <verbatim user request>}.
   Save the returned intent_id.

2. **Before asking the user to review** — call relay_check with the unified
   diff plus {intent: <intent_id>, brief: <one-liner>}.

3. **If Allowed=false** — for each policy entry with Verdict != "allow":
   - Call relay_explain {gate, rule} for the recommended fix.
   - Apply the fix, re-diff, and re-call relay_check.
   - Loop up to 3 times; on the 3rd failure surface the verdict to the user.

4. **Only call relay_submit when relay_check returns Allowed=true** on the
   EXACT same diff. Never call relay_submit speculatively.

5. **Close the intent when done** — call relay_intent_close {intent_id}.
   Pass the returned trailer_block to relay_submit so the commit is linked
   to the intent YAML.

### Tool quick-reference

| Tool                 | When                                          |
|----------------------|-----------------------------------------------|
| relay_intent_open    | First — capture the user request as an intent |
| relay_check          | Before every review request                   |
| relay_explain        | On any Verdict != allow                       |
| relay_submit         | Only after relay_check Allowed=true           |
| relay_policy         | Discover which gates are active               |
| relay_intent_close   | When the task is complete                     |
`

// writeRelaySteeringInstructions appends the Relay Pre-Flight Autopilot
// instructions into every per-agent instruction file found in projectDir.
// It is idempotent: files already containing the marker are skipped.
func writeRelaySteeringInstructions(projectDir string) {
	type target struct {
		name    string
		relPath string
	}
	targets := []target{
		{"Claude Code", "CLAUDE.md"},
		{"Cursor", ".cursorrules"},
		{"Windsurf", ".windsurfrules"},
		{"Cline / Roo Code", ".clinerules"},
		{"Kiro", ".kiro/steering/relay.md"},
		{"Devin", ".devin/instructions.md"},
		{"Amp", ".amp/instructions.md"},
		{"GitHub Copilot", ".github/copilot-instructions.md"},
		{"AGENTS.md", "AGENTS.md"},
		{"Gemini CLI", "GEMINI.md"},
	}
	const marker = "## Relay — certified merge gate"

	for _, t := range targets {
		path := filepath.Join(projectDir, t.relPath)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "warning: mkdir for %s: %v\n", t.name, err)
			continue
		}
		existing, _ := os.ReadFile(path)
		if strings.Contains(string(existing), marker) {
			continue // already wired — idempotent
		}
		var content string
		if len(existing) > 0 {
			content = string(existing) + "\n" + relaySteeringInstructions
		} else {
			content = relaySteeringInstructions
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "warning: write %s steering instructions: %v\n", t.name, err)
			continue
		}
		fmt.Printf("wrote relay instructions: %s\n", path)
	}
}

// registerRelayMCPTools writes relay MCP server entries into every detected
// AI coding tool's config file. It is idempotent: existing files are merged,
// not overwritten. Project-local directories are always created; global
// user-level directories are only written when they already exist (i.e. the
// tool is installed on this machine).
func registerRelayMCPTools(projectDir, relayBin string) {
	home, _ := os.UserHomeDir()
	args := []string{"mcp", "serve", "--repo", projectDir}

	type mcpWriter struct {
		name      string
		path      string
		serverKey string // "mcpServers" (standard), "servers" (VS Code), "context_servers" (Zed)
		entry     map[string]any
		global    bool // true = skip if parent directory doesn't already exist
		arrayFmt  bool // true = servers list is JSON array (Continue)
		tomlFmt   bool // true = config is TOML, not JSON (Codex CLI)
	}

	// Standard format: {"mcpServers": {"relay": {"command":..,"args":..,"env":{}}}}
	stdEntry := map[string]any{
		"command": relayBin,
		"args":    args,
		"env":     map[string]string{},
	}
	// VS Code format: {"servers": {"relay": {"type":"stdio","command":..,"args":..}}}
	vscodeEntry := map[string]any{
		"type":    "stdio",
		"command": relayBin,
		"args":    args,
	}
	// Zed format: {"context_servers": {"relay": {"command": {"path":..,"args":..,"env":{}}}}}
	zedEntry := map[string]any{
		"command": map[string]any{
			"path": relayBin,
			"args": args,
			"env":  map[string]string{},
		},
	}

	// Claude Desktop — OS-dependent path, standard mcpServers format.
	var cdPath string
	switch runtime.GOOS {
	case "darwin":
		cdPath = filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json")
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			appData = filepath.Join(home, "AppData", "Roaming")
		}
		cdPath = filepath.Join(appData, "Claude", "claude_desktop_config.json")
	default:
		cdPath = filepath.Join(home, ".config", "Claude", "claude_desktop_config.json")
	}

	writers := []mcpWriter{
		{
			name:      "Claude Code",
			path:      filepath.Join(projectDir, ".claude", "mcp.json"),
			serverKey: "mcpServers",
			entry:     stdEntry,
			global:    false, // project-local — always create
		},
		{
			name:      "Claude Desktop",
			path:      cdPath,
			serverKey: "mcpServers",
			entry:     stdEntry,
			global:    true,
		},
		{
			name:      "Cursor (project)",
			path:      filepath.Join(projectDir, ".cursor", "mcp.json"),
			serverKey: "mcpServers",
			entry:     stdEntry,
			global:    false, // project-local — always create
		},
		{
			name:    "Codex CLI",
			path:    filepath.Join(home, ".codex", "config.toml"),
			entry:   stdEntry, // content derived from entry fields; serverKey unused for TOML
			global:  true,
			tomlFmt: true,
		},
		{
			name:      "Cursor (global)",
			path:      filepath.Join(home, ".cursor", "mcp.json"),
			serverKey: "mcpServers",
			entry:     stdEntry,
			global:    true,
		},
		{
			name:      "Continue",
			path:      filepath.Join(home, ".continue", "config.json"),
			serverKey: "mcpServers",
			entry:     stdEntry,
			global:    true,
			arrayFmt:  true,
		},
		{
			name:      "Windsurf",
			path:      filepath.Join(home, ".codeium", "windsurf", "mcp_config.json"),
			serverKey: "mcpServers",
			entry:     stdEntry,
			global:    true,
		},
		{
			name:      "Zed",
			path:      filepath.Join(home, ".config", "zed", "settings.json"),
			serverKey: "context_servers",
			entry:     zedEntry,
			global:    true,
		},
		{
			name:      "VS Code",
			path:      filepath.Join(projectDir, ".vscode", "mcp.json"),
			serverKey: "servers",
			entry:     vscodeEntry,
			global:    false, // project-local — always create
		},
		{
			// Kiro (Amazon) stores project MCP servers in .kiro/settings/mcp.json.
			// Standard mcpServers format.
			name:      "Kiro",
			path:      filepath.Join(projectDir, ".kiro", "settings", "mcp.json"),
			serverKey: "mcpServers",
			entry:     stdEntry,
			global:    false, // project-local — always create
		},
	}

	for _, w := range writers {
		parent := filepath.Dir(w.path)
		if _, err := os.Stat(parent); err != nil {
			if w.global {
				continue // tool not installed on this machine — skip silently
			}
			if err := os.MkdirAll(parent, 0o755); err != nil {
				fmt.Fprintf(os.Stderr, "warning: mkdir %s config dir: %v\n", w.name, err)
				continue
			}
		}
		var writeErr error
		switch {
		case w.tomlFmt:
			writeErr = updateCodexConfig(w.path, relayBin, args)
		case w.arrayFmt:
			_, writeErr = updateContinueConfig(w.path, w.serverKey, "relay", w.entry)
		default:
			_, writeErr = updateClientConfig(w.path, w.serverKey, "relay", w.entry)
		}
		if writeErr != nil {
			fmt.Fprintf(os.Stderr, "warning: write %s MCP config: %v\n", w.name, writeErr)
			continue
		}
		fmt.Printf("registered relay with %s: %s\n", w.name, w.path)
	}
}

// updateCodexConfig writes a relay [[mcp_servers]] entry to Codex CLI's
// config.toml (~/.codex/config.toml). The file is created if absent.
// An existing relay block is replaced idempotently.
func updateCodexConfig(path, relayBin string, args []string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir codex config dir: %w", err)
	}
	var lines []string
	if raw, err := os.ReadFile(path); err == nil {
		lines = strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read %s: %w", path, err)
	}
	// Remove any existing relay [[mcp_servers]] block.
	lines = stripTOMLBlock(lines, "mcp_servers", "relay")
	// If relayBin is empty this is a removal — stop here.
	if relayBin == "" {
		return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
	}
	// Append blank separator then new block.
	if len(lines) > 0 && lines[len(lines)-1] != "" {
		lines = append(lines, "")
	}
	lines = append(lines,
		"[[mcp_servers]]",
		`name = "relay"`,
		`type = "stdio"`,
		fmt.Sprintf("command = %q", relayBin),
		tomlStringArray("args", args),
	)
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

// stripTOMLBlock removes all [[section]] array-of-tables blocks whose
// "name" field equals targetName, preserving everything else.
func stripTOMLBlock(lines []string, section, targetName string) []string {
	header := "[[" + section + "]]"
	nameKV := `name = "` + targetName + `"`
	var out []string
	i := 0
	for i < len(lines) {
		if strings.TrimSpace(lines[i]) != header {
			out = append(out, lines[i])
			i++
			continue
		}
		// Collect the full block (until the next [[...]] header or EOF).
		start := i
		i++
		isMatch := false
		for i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), "[[") {
			if strings.TrimSpace(lines[i]) == nameKV {
				isMatch = true
			}
			i++
		}
		if !isMatch {
			out = append(out, lines[start:i]...)
		}
	}
	return out
}

// tomlStringArray formats a TOML key = ["v1", "v2"] line.
func tomlStringArray(key string, vals []string) string {
	quoted := make([]string, len(vals))
	for i, v := range vals {
		quoted[i] = fmt.Sprintf("%q", v)
	}
	return key + " = [" + strings.Join(quoted, ", ") + "]"
}
