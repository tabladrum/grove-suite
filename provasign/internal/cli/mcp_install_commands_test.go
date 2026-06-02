package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// withFakeHome redirects HOME (and USERPROFILE on Windows) to a temp dir so
// install-for writes into a sandbox instead of clobbering the developer's
// real IDE config. os.UserHomeDir() reads USERPROFILE on Windows, HOME on
// Unix — both must be set for the redirect to work cross-platform.
func withFakeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

// readJSONDoc loads a config file written by updateClientConfig and returns
// its mcpServers entry for assertion.
func readJSONDoc(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return doc
}

// TestInstallForClaudeCodeWritesEntry covers the primary install flow for
// the most common laptop client. The output file should contain a
// well-formed mcpServers.relay entry pointing at the provasign binary.
func TestInstallForClaudeCodeWritesEntry(t *testing.T) {
	home := withFakeHome(t)
	if code := cmdMCPInstallFor([]string{"--bin", "/usr/local/bin/relay", "claude-code"}); code != 0 {
		t.Fatalf("install-for exit = %d", code)
	}
	doc := readJSONDoc(t, filepath.Join(home, ".claude.json"))
	servers, _ := doc["mcpServers"].(map[string]any)
	if servers == nil {
		t.Fatal("mcpServers missing")
	}
	entry, _ := servers["relay"].(map[string]any)
	if entry == nil {
		t.Fatal("mcpServers.relay missing")
	}
	if entry["command"] != "/usr/local/bin/relay" {
		t.Errorf("command = %v", entry["command"])
	}
	args, _ := entry["args"].([]any)
	if len(args) < 2 || args[0] != "mcp" || args[1] != "serve" {
		t.Errorf("args = %v", args)
	}
}

// TestInstallForIsIdempotent re-runs install-for and confirms the file
// remains valid (no duplicate "relay" keys, no corruption).
func TestInstallForIsIdempotent(t *testing.T) {
	home := withFakeHome(t)
	for i := 0; i < 3; i++ {
		if code := cmdMCPInstallFor([]string{"--bin", "/usr/local/bin/relay", "cursor"}); code != 0 {
			t.Fatalf("install-for iteration %d exit = %d", i, code)
		}
	}
	doc := readJSONDoc(t, filepath.Join(home, ".cursor", "mcp.json"))
	servers, _ := doc["mcpServers"].(map[string]any)
	if len(servers) != 1 {
		t.Errorf("expected exactly 1 server entry, got %d", len(servers))
	}
}

// TestInstallForPreservesUnrelatedEntries asserts an existing user-managed
// MCP server is not stomped by the relay install.
func TestInstallForPreservesUnrelatedEntries(t *testing.T) {
	home := withFakeHome(t)
	configPath := filepath.Join(home, ".codeium", "windsurf", "mcp_config.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	pre := map[string]any{
		"mcpServers": map[string]any{
			"my-other-server": map[string]any{"command": "/opt/bin/other"},
		},
		"theme": "dark",
	}
	raw, _ := json.Marshal(pre)
	if err := os.WriteFile(configPath, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if code := cmdMCPInstallFor([]string{"--bin", "/usr/local/bin/relay", "windsurf"}); code != 0 {
		t.Fatalf("install-for exit = %d", code)
	}
	doc := readJSONDoc(t, configPath)
	if doc["theme"] != "dark" {
		t.Errorf("theme key lost: %v", doc["theme"])
	}
	servers, _ := doc["mcpServers"].(map[string]any)
	if _, ok := servers["my-other-server"]; !ok {
		t.Errorf("my-other-server entry lost")
	}
	if _, ok := servers["relay"]; !ok {
		t.Errorf("relay entry not added")
	}
}

// TestInstallForUninstallReverses covers the rollback path.
func TestInstallForUninstallReverses(t *testing.T) {
	home := withFakeHome(t)
	if code := cmdMCPInstallFor([]string{"--bin", "/usr/local/bin/relay", "windsurf"}); code != 0 {
		t.Fatalf("install-for exit = %d", code)
	}
	if code := cmdMCPInstallFor([]string{"--uninstall", "windsurf"}); code != 0 {
		t.Fatalf("uninstall exit = %d", code)
	}
	doc := readJSONDoc(t, filepath.Join(home, ".codeium", "windsurf", "mcp_config.json"))
	servers, _ := doc["mcpServers"].(map[string]any)
	if _, ok := servers["relay"]; ok {
		t.Errorf("relay entry still present after uninstall")
	}
}

// TestInstallForRejectsUnknownClient covers the validation path.
func TestInstallForRejectsUnknownClient(t *testing.T) {
	withFakeHome(t)
	if code := cmdMCPInstallFor([]string{"--bin", "/usr/local/bin/relay", "vim"}); code == 0 {
		t.Errorf("expected non-zero exit for unknown client")
	}
}

// TestInstallForList covers `--list`.
func TestInstallForList(t *testing.T) {
	if code := cmdMCPInstallFor([]string{"--list"}); code != 0 {
		t.Errorf("--list exit = %d", code)
	}
	// Sanity check the static list still includes the 4 anchors.
	have := map[string]bool{}
	for _, c := range supportedClients() {
		have[c.id] = true
	}
	for _, want := range []string{"claude-code", "cursor", "continue", "windsurf"} {
		if !have[want] {
			t.Errorf("missing client %q in supportedClients()", want)
		}
	}
}
