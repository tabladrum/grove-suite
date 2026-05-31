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
	"strings"
)

// clientSpec describes one supported MCP client and where its config lives.
// macOS paths are the primary target (Phase 2A); Linux paths are mirrored
// where each client exists there. Windows is WSL — same Linux paths apply.
type clientSpec struct {
	id     string // canonical id, also the flag value
	label  string // human-readable label
	pathFn func(home string) (string, error)
}

func macOSAppSupport(home, sub string) string {
	return filepath.Join(home, "Library", "Application Support", sub)
}

// supportedClients returns the static client list. Each spec resolves to a
// JSON file path on first call; nothing is created until install runs.
func supportedClients() []clientSpec {
	return []clientSpec{
		{
			id:    "claude-code",
			label: "Claude Code",
			pathFn: func(home string) (string, error) {
				// Claude Code stores per-user MCP servers at ~/.claude.json
				// (cross-platform).
				return filepath.Join(home, ".claude.json"), nil
			},
		},
		{
			id:    "cursor",
			label: "Cursor",
			pathFn: func(home string) (string, error) {
				// Cursor reads MCP server config from ~/.cursor/mcp.json.
				return filepath.Join(home, ".cursor", "mcp.json"), nil
			},
		},
		{
			id:    "continue",
			label: "Continue",
			pathFn: func(home string) (string, error) {
				// Continue stores config in ~/.continue/config.json.
				return filepath.Join(home, ".continue", "config.json"), nil
			},
		},
		{
			id:    "windsurf",
			label: "Windsurf",
			pathFn: func(home string) (string, error) {
				// Windsurf (Codeium) stores MCP at ~/.codeium/windsurf/mcp_config.json.
				return filepath.Join(home, ".codeium", "windsurf", "mcp_config.json"), nil
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
	configPath, err := spec.pathFn(home)
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

	if *uninstall {
		removed, err := updateClientConfig(configPath, "relay", nil)
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

	entry := buildRelayEntry(relayBin, *repo)
	if _, err := updateClientConfig(configPath, "relay", entry); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("installed relay MCP entry into %s\n", configPath)
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
func buildRelayEntry(relayBin, repo string) map[string]any {
	args := []string{"mcp", "serve"}
	if repo != "" {
		args = append(args, "--repo", repo)
	}
	return map[string]any{
		"command": relayBin,
		"args":    args,
		"env":     map[string]string{},
	}
}

// updateClientConfig reads the JSON file at path, ensures it has an
// "mcpServers" object, then sets or removes the named entry. Returns
// removed=true if --uninstall actually deleted something; the install path
// returns false (unused).
//
// The file is created if absent; surrounding fields are preserved so users
// don't lose unrelated config they've added themselves.
func updateClientConfig(path, name string, entry map[string]any) (bool, error) {
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
	servers, _ := doc["mcpServers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}
	if entry == nil {
		_, removed := servers[name]
		delete(servers, name)
		doc["mcpServers"] = servers
		return removed, writeJSON(path, doc)
	}
	servers[name] = entry
	doc["mcpServers"] = servers
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

// supportedClientLabels returns a stable, sorted, comma-joined list for
// error messages.
func supportedClientLabels() string {
	ids := make([]string, 0, 4)
	for _, c := range supportedClients() {
		ids = append(ids, c.id)
	}
	sort.Strings(ids)
	return strings.Join(ids, ", ")
}

// Touch runtime so older toolchains (go 1.21 and below) compile this file
// even on platforms without `os.UserConfigDir`.
var _ = runtime.GOOS
