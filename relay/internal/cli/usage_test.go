package cli

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

// captureUsageStdout returns stdout produced by fn. Replaces os.Stdout for
// the duration of the call and restores it on exit. (Separate helper from
// engine_commands_test.go::captureStdout to avoid duplicate-symbol clash.)
func captureUsageStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	fn()
	_ = w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String()
}

// TestPrintUsage_ListsAllLaptopCommands locks down the production-gap
// fix: every laptop-mode command we ship must be discoverable in
// `relay --help`. If a future change adds a CLI subcommand without
// updating printUsage, this test fails and the author has a list of
// commands they need to document.
func TestPrintUsage_ListsAllLaptopCommands(t *testing.T) {
	out := captureUsageStdout(t, printUsage)
	required := []string{
		// Laptop-mode core
		"relay init",
		"relay keys gen",
		"relay keys fingerprint",
		"relay keys export",
		"relay keys import",
		// Daemon
		"relay daemon start",
		"relay daemon status",
		"relay daemon stop",
		// Intent capture
		"relay intent open",
		"relay intent update",
		"relay intent close",
		"relay intent list",
		"relay intent list-captured",
		"relay intent get-captured",
		// Engine + admission
		"relay check",
		"relay submit",
		"relay cert",
		// Hook + MCP install
		"relay hook install",
		"relay mcp serve",
		"relay mcp install-for",
		// Tools + importer
		"relay tools",
		"relay import sonarqube-profile",
		// Team mode (must still be advertised)
		"relay serve",
		"relay repo",
		"relay project",
		"relay intent create",
		// Env vars
		"DATABASE_URL",
		"GROVE_URL",
	}
	for _, tok := range required {
		if !strings.Contains(out, tok) {
			t.Errorf("printUsage() missing %q", tok)
		}
	}
}
