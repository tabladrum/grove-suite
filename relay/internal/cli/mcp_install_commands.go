package cli

// `relay mcp install-for <client>` — register the laptop Relay daemon's
// stdio MCP server with a supported IDE client so the user doesn't have to
// hand-edit each client's MCP config per machine.
//
// Strategy: each client stores its MCP server list as a small JSON file
// under the user's home directory. We do an idempotent merge: read the
// existing file (or start fresh), add/overwrite the "relay" entry, and
// write it back atomically. `--uninstall` reverses the operation.

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
)

// clientSpec describes one supported MCP client and where its config lives.
// macOS paths are the primary target (Phase 2A); Linux paths are mirrored
// where each client exists there. Windows is WSL — same Linux paths apply.
type clientSpec struct {
	id        string // canonical id, also the flag value
	label     string // human-readable label
	serverKey string // JSON key holding MCP servers (default "mcpServers"; "servers" for VS Code)
	arrayFmt  bool   // true if the servers list is a JSON array, not object (e.g. Continue)
	tomlFmt   bool   // true if the config file is TOML, not JSON (e.g. Codex CLI)
	pathFn    func(home, repo string) (string, error)
}

// supportedClients returns the static client list. Each spec resolves to a
// JSON file path on first call; nothing is created until install runs.
func supportedClients() []clientSpec {
	return []clientSpec{
		{
			id:    "claude-code",
			label: "Claude Code",
			pathFn: func(home, _ string) (string, error) {
				// Claude Code stores per-user MCP servers at ~/.claude.json
				// (cross-platform).
				return filepath.Join(home, ".claude.json"), nil
			},
		},
		{
			id:    "cursor",
			label: "Cursor",
			pathFn: func(home, _ string) (string, error) {
				// Cursor reads MCP server config from ~/.cursor/mcp.json.
				return filepath.Join(home, ".cursor", "mcp.json"), nil
			},
		},
		{
			id:       "continue",
			label:    "Continue",
			arrayFmt: true, // Continue uses [{name, command, args}] not {name: {command, args}}
			pathFn: func(home, _ string) (string, error) {
				// Continue stores config in ~/.continue/config.json.
				return filepath.Join(home, ".continue", "config.json"), nil
			},
		},
		{
			id:    "windsurf",
			label: "Windsurf",
			pathFn: func(home, _ string) (string, error) {
				// Windsurf (Codeium) stores MCP at ~/.codeium/windsurf/mcp_config.json.
				return filepath.Join(home, ".codeium", "windsurf", "mcp_config.json"), nil
			},
		},
		{
			id:    "claude-desktop",
			label: "Claude Desktop",
			pathFn: func(home, _ string) (string, error) {
				// Claude Desktop stores global MCP config in a platform-specific location.
				switch runtime.GOOS {
				case "darwin":
					return filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json"), nil
				case "windows":
					appData := os.Getenv("APPDATA")
					if appData == "" {
						appData = filepath.Join(home, "AppData", "Roaming")
					}
					return filepath.Join(appData, "Claude", "claude_desktop_config.json"), nil
				default:
					return filepath.Join(home, ".config", "Claude", "claude_desktop_config.json"), nil
				}
			},
		},
		{
			id:        "zed",
			label:     "Zed",
			serverKey: "context_servers", // Zed uses context_servers with a nested command object
			pathFn: func(home, _ string) (string, error) {
				// Zed reads MCP context servers from ~/.config/zed/settings.json.
				return filepath.Join(home, ".config", "zed", "settings.json"), nil
			},
		},
		{
			// Codex CLI (OpenAI) reads MCP servers from ~/.codex/config.toml.
			id:      "codex",
			label:   "Codex CLI",
			tomlFmt: true,
			pathFn: func(home, _ string) (string, error) {
				return filepath.Join(home, ".codex", "config.toml"), nil
			},
		},
		{
			// Kiro (Amazon's AI IDE) stores project MCP servers in .kiro/settings/mcp.json.
			id:    "kiro",
			label: "Kiro",
			pathFn: func(_, repo string) (string, error) {
				base := repo
				if base == "" {
					if wd, err := os.Getwd(); err == nil {
						base = wd
					} else {
						base = "."
					}
				}
				abs, err := filepath.Abs(base)
				if err != nil {
					return "", err
				}
				return filepath.Join(abs, ".kiro", "settings", "mcp.json"), nil
			},
		},
		{
			id:        "vscode",
			label:     "VS Code",
			serverKey: "servers", // VS Code uses {"servers":{...}} not {"mcpServers":{...}}
			pathFn: func(_, repo string) (string, error) {
				// VS Code reads workspace MCP servers from .vscode/mcp.json.
				// Fall back to cwd when --repo is not provided.
				base := repo
				if base == "" {
					if wd, err := os.Getwd(); err == nil {
						base = wd
					} else {
						base = "."
					}
				}
				abs, err := filepath.Abs(base)
				if err != nil {
					return "", err
				}
				return filepath.Join(abs, ".vscode", "mcp.json"), nil
			},
		},
	}
}

// runInstallFor is registered into RunMCP via the `install-for` subcommand.
func cmdMCPInstallFor(args []string) int {
	fs := flag.NewFlagSet("mcp install-for", flag.ContinueOnError)
	uninstall := fs.Bool("uninstall", false, "remove the relay MCP entry instead of installing it")
	repo := fs.String("repo", "", "default repo for relay mcp serve (passed via args)")
	bin := fs.String("bin", "", "absolute path to the relay binary (defaults to the running executable)")
	listFlag := fs.Bool("list", false, "list supported clients and exit")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if *listFlag {
		for _, c := range supportedClients() {
			fmt.Printf("  %-12s %s\n", c.id, c.label)
		}
		return 0
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: relay mcp install-for <client> [--uninstall] [--repo <dir>] [--bin <path>]")
		fmt.Fprintln(os.Stderr, "       relay mcp install-for --list")
		return 1
	}
	clientID := fs.Arg(0)

	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	spec, ok := findClient(clientID)
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown client %q (run --list to see supported ones)\n", clientID)
		return 1
	}
	configPath, err := spec.pathFn(home, *repo)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	relayBin := *bin
	if relayBin == "" {
		exe, err := os.Executable()
		if err != nil {
			fmt.Fprintln(os.Stderr, "resolve relay binary:", err)
			return 1
		}
		relayBin = exe
	}

	serverKey := spec.serverKey
	if serverKey == "" {
		serverKey = "mcpServers"
	}

	if *uninstall {
		var removed bool
		var err error
		switch {
		case spec.tomlFmt:
			// TOML removal: overwrite without the relay block.
			err = updateCodexConfig(configPath, "", nil)
			removed = err == nil
		case spec.arrayFmt:
			removed, err = updateContinueConfig(configPath, serverKey, "relay", nil)
		default:
			removed, err = updateClientConfig(configPath, serverKey, "relay", nil)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if removed {
			fmt.Printf("removed relay MCP entry from %s\n", configPath)
		} else {
			fmt.Printf("no relay MCP entry found in %s\n", configPath)
		}
		return 0
	}

	var entry map[string]any
	switch spec.id {
	case "zed":
		entry = buildZedEntry(relayBin, *repo)
	default:
		entry = buildRelayEntry(relayBin, *repo)
		if serverKey == "servers" {
			// VS Code requires a "type" field; env map is not used.
			entry["type"] = "stdio"
			delete(entry, "env")
		}
	}
	var installErr error
	switch {
	case spec.tomlFmt:
		installErr = updateCodexConfig(configPath, relayBin, []string{"mcp", "serve", "--repo", *repo})
	case spec.arrayFmt:
		_, installErr = updateContinueConfig(configPath, serverKey, "relay", entry)
	default:
		_, installErr = updateClientConfig(configPath, serverKey, "relay", entry)
	}
	if installErr != nil {
		fmt.Fprintln(os.Stderr, installErr)
		return 1
	}
	fmt.Printf("installed relay MCP entry into %s\n", configPath)
	if spec.id == "claude-code" {
		ensureClaudeCodeApproval("relay")
	}
	fmt.Println()
	fmt.Printf("Restart %s to pick up the change. The agent will now have access to the relay_*\n", spec.label)
	fmt.Println("tools, including Auto-Intent Capture. Run `relay daemon status` to confirm the")
	fmt.Println("local daemon is running.")
	return 0
}

func findClient(id string) (clientSpec, bool) {
	for _, c := range supportedClients() {
		if c.id == id {
			return c, true
		}
	}
	return clientSpec{}, false
}

// buildRelayEntry constructs the MCP-server descriptor every client agrees
// on. This is the smallest cross-client common subset: name, command, args.
// "env" is intentionally omitted: an empty env map causes unnecessary diffs
// on repeated relay init runs, which resets Claude Code's MCP approval state.
func buildRelayEntry(relayBin, repo string) map[string]any {
	args := []string{"mcp", "serve"}
	if repo != "" {
		args = append(args, "--repo", repo)
	}
	return map[string]any{
		"command": relayBin,
		"args":    args,
	}
}

// buildZedEntry constructs the MCP-server descriptor for Zed. Zed wraps the
// command under a nested "command" object with "path"/"args"/"env" fields,
// stored under the "context_servers" key rather than "mcpServers".
func buildZedEntry(relayBin, repo string) map[string]any {
	args := []string{"mcp", "serve"}
	if repo != "" {
		args = append(args, "--repo", repo)
	}
	return map[string]any{
		"command": map[string]any{
			"path": relayBin,
			"args": args,
			"env":  map[string]string{},
		},
	}
}

// updateContinueConfig manages relay's entry in an array-style servers list
// (used by Continue, which stores mcpServers as [{name, command, args}]).
// Entries are matched by the "name" field; name is injected automatically.
func updateContinueConfig(path, serverKey, name string, entry map[string]any) (bool, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, fmt.Errorf("mkdir config dir: %w", err)
	}
	var doc map[string]any
	if raw, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(raw, &doc); err != nil {
			return false, fmt.Errorf("parse %s: %w", path, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	if doc == nil {
		doc = map[string]any{}
	}
	var servers []any
	if s, ok := doc[serverKey].([]any); ok {
		servers = s
	}
	// Locate existing entry by "name" field.
	idx := -1
	for i, s := range servers {
		if m, ok := s.(map[string]any); ok && m["name"] == name {
			idx = i
			break
		}
	}
	if entry == nil {
		if idx < 0 {
			return false, writeJSON(path, doc)
		}
		servers = append(servers[:idx], servers[idx+1:]...)
		doc[serverKey] = servers
		return true, writeJSON(path, doc)
	}
	// Copy entry and inject the "name" field so we don't mutate the caller's map.
	e := make(map[string]any, len(entry)+1)
	for k, v := range entry {
		e[k] = v
	}
	e["name"] = name
	if idx >= 0 {
		servers[idx] = e
	} else {
		servers = append(servers, e)
	}
	doc[serverKey] = servers
	return false, writeJSON(path, doc)
}

// updateClientConfig reads the JSON file at path, ensures it has a
// serverKey object (e.g. "mcpServers" or "servers"), then sets or removes
// the named entry. Returns removed=true if --uninstall actually deleted
// something; the install path returns false (unused).
//
// The file is created if absent; surrounding fields are preserved so users
// don't lose unrelated config they've added themselves.
func updateClientConfig(path, serverKey, name string, entry map[string]any) (bool, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, fmt.Errorf("mkdir config dir: %w", err)
	}
	var doc map[string]any
	if raw, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(raw, &doc); err != nil {
			return false, fmt.Errorf("parse %s: %w", path, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	if doc == nil {
		doc = map[string]any{}
	}
	servers, _ := doc[serverKey].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	if entry == nil {
		_, removed := servers[name]
		delete(servers, name)
		doc[serverKey] = servers
		return removed, writeJSON(path, doc)
	}
	servers[name] = entry
	doc[serverKey] = servers
	return false, writeJSON(path, doc)
}

// writeJSON marshals doc with sorted top-level keys for deterministic
// diffs, then writes it atomically (write to .tmp + rename) so a crash
// mid-write doesn't leave a half-written client config.
func writeJSON(path string, doc map[string]any) error {
	keys := make([]string, 0, len(doc))
	for k := range doc {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	ordered := make(map[string]any, len(doc))
	for _, k := range keys {
		ordered[k] = doc[k]
	}
	data, err := json.MarshalIndent(ordered, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ensureClaudeCodeApproval adds serverName to enabledMcpjsonServers in
// ~/.claude/settings.json so Claude Code trusts the server without requiring
// interactive re-approval after every `relay init` run.
func ensureClaudeCodeApproval(serverName string) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	path := filepath.Join(home, ".claude", "settings.json")
	var doc map[string]any
	if raw, err := os.ReadFile(path); err == nil {
		json.Unmarshal(raw, &doc) //nolint:errcheck
	}
	if doc == nil {
		doc = map[string]any{}
	}
	var servers []any
	if s, ok := doc["enabledMcpjsonServers"].([]any); ok {
		servers = s
	}
	for _, s := range servers {
		if s == serverName {
			return // already approved
		}
	}
	servers = append(servers, serverName)
	doc["enabledMcpjsonServers"] = servers
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		return
	}
	fmt.Printf("approved %s in Claude Code MCP settings\n", serverName)
}

// Touch runtime so older toolchains (go 1.21 and below) compile this file
// even on platforms without `os.UserConfigDir`.
var _ = runtime.GOOS
