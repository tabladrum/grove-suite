// relay doctor — single command that diagnoses the four things most
// likely to silently break a relay run on a fresh machine:
//
//  1. Grove HTTP daemon: reachable and authenticating?
//  2. Stage-2 tools: which binaries (semgrep, sonar, etc.) are present?
//  3. relay config: is .relay/relay.yaml parseable and what scope/floor
//     does each analyzer use?
//  4. JVM / SonarLint Core: is the wrapper jar + a Java >= 17 available?
//
// Output is human-grep-friendly: section headers + tabular rows. Exit
// code is 0 unless one of the checks the user clearly needs (Grove +
// any enabled analyzer's binary) is missing.
package cli

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/tabladrum/grove-suite/relay/internal/analyzers/sonar"
	"github.com/tabladrum/grove-suite/relay/internal/config"
	"github.com/tabladrum/grove-suite/relay/internal/grove"
	"github.com/tabladrum/grove-suite/relay/internal/tools"
)

// RunDoctor is the entry point dispatched from cli.Run.
func RunDoctor(args []string) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	startDir := fs.String("dir", ".", "start directory for .relay/ + .grove/ discovery")
	groveURL := fs.String("grove-url", defaultGroveURL(), "Grove HTTP base URL")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	w := os.Stdout
	fail := 0

	fail += checkGrove(w, *groveURL, *startDir)
	fail += checkConfig(w, *startDir)
	fail += checkAnalyzers(w, *startDir)
	fail += checkJVM(w)

	fmt.Fprintln(w)
	if fail == 0 {
		fmt.Fprintln(w, "all checks passed.")
		return 0
	}
	fmt.Fprintf(w, "%d check(s) need attention.\n", fail)
	return 1
}

func defaultGroveURL() string {
	if v := os.Getenv("GROVE_URL"); v != "" {
		return v
	}
	return "http://localhost:7777"
}

// ── grove ────────────────────────────────────────────────────────────────────

func checkGrove(w io.Writer, baseURL, dir string) int {
	section(w, "grove")
	fmt.Fprintf(w, "  url:        %s\n", baseURL)

	// Reachability is independent of auth.
	resp, err := http.Get(baseURL + "/health")
	if err != nil {
		fmt.Fprintf(w, "  reachable:  no (%v)\n", err)
		fmt.Fprintf(w, "  hint:       start grove with `grove serve %s`\n", dir)
		return 1
	}
	_ = resp.Body.Close()
	fmt.Fprintf(w, "  reachable:  yes (HTTP %d)\n", resp.StatusCode)

	// Token discovery — show every .grove/.token under dir so a user
	// with a duplicate workspace/.grove vs project/.grove can see the
	// mismatch instantly.
	matches := findGroveTokens(dir)
	if len(matches) == 0 {
		fmt.Fprintf(w, "  tokens:     none found under %s\n", dir)
		fmt.Fprintf(w, "  hint:       run `grove serve %s` to bootstrap a token\n", dir)
		return 1
	}
	for _, m := range matches {
		fmt.Fprintf(w, "  token:      %s\n", m)
	}

	// Authenticated probe using the first token directory.
	tokenDir := filepath.Dir(filepath.Dir(matches[0]))
	c := grove.NewClient(baseURL).WithTokenFromDir(tokenDir)
	if !c.HasToken() {
		fmt.Fprintf(w, "  auth:       token file empty at %s\n", c.TokenPath())
		return 1
	}
	if err := c.Health(); err != nil {
		fmt.Fprintf(w, "  auth:       FAIL (%v)\n", err)
		return 1
	}
	fmt.Fprintf(w, "  auth:       ok (token from %s)\n", c.TokenPath())
	return 0
}

// findGroveTokens scans dir and one level up for .grove/.token files —
// covers the common "two .grove dirs in the workspace" trap where the
// MCP server uses one and the CLI uses the other.
func findGroveTokens(dir string) []string {
	var out []string
	candidates := []string{
		filepath.Join(dir, ".grove", ".token"),
	}
	if abs, err := filepath.Abs(dir); err == nil {
		parent := filepath.Dir(abs)
		if parent != abs {
			candidates = append(candidates, filepath.Join(parent, ".grove", ".token"))
		}
	}
	seen := map[string]bool{}
	for _, p := range candidates {
		abs, err := filepath.Abs(p)
		if err != nil || seen[abs] {
			continue
		}
		seen[abs] = true
		if _, err := os.Stat(abs); err == nil {
			out = append(out, abs)
		}
	}
	return out
}

// ── config ───────────────────────────────────────────────────────────────────

func checkConfig(w io.Writer, dir string) int {
	section(w, "config")
	cfg, err := config.LoadRelayConfig(dir)
	if err != nil {
		fmt.Fprintf(w, "  status:     no .relay/relay.yaml from %s (using built-in defaults)\n", dir)
		return 0
	}
	fmt.Fprintf(w, "  source:     %s\n", cfg.SourcePath)
	fmt.Fprintf(w, "  scope:      %s\n", cfg.Scope.Normalize())
	fmt.Fprintf(w, "  analyzers:  %d configured\n", len(cfg.Analyzers))
	return 0
}

// ── analyzers ────────────────────────────────────────────────────────────────

func checkAnalyzers(w io.Writer, dir string) int {
	section(w, "analyzers")
	printToolsStatus(w, dir)
	// Surface failure only when a *configured-enabled* analyzer is missing
	// its binary. A missing optional analyzer isn't an error.
	cfg, err := config.LoadRelayConfig(dir)
	if err != nil {
		return 0
	}
	fail := 0
	for _, row := range analyzerStatusRows(cfg) {
		if row.enabled == "yes" && strings.Contains(row.binary, "missing") {
			fail++
		}
	}
	return fail
}

// ── jvm ──────────────────────────────────────────────────────────────────────

func checkJVM(w io.Writer) int {
	section(w, "jvm / sonar")
	java := sonar.JavaPath()
	jar := sonar.WrapperPath()
	fmt.Fprintf(w, "  java:       %s\n", emptyAs(java, "missing"))
	fmt.Fprintf(w, "  wrapper:    %s\n", emptyAs(jar, "missing"))
	fmt.Fprintf(w, "  min-java:   %d\n", sonar.MinJavaMajor)
	if java != "" && jar != "" {
		return 0
	}
	if java == "" && jar == "" {
		fmt.Fprintln(w, "  hint:       run `relay tools install --with-sonar`")
	} else if java == "" {
		fmt.Fprintln(w, "  hint:       set JAVA_HOME to a Java 17+ install or run `relay tools install --with-sonar`")
	} else {
		fmt.Fprintln(w, "  hint:       run `relay tools install --with-sonar` to fetch the wrapper jar")
	}
	return 0 // missing JVM/wrapper is only fatal if sonar is enabled — checkAnalyzers handles that.
}

// ── helpers ──────────────────────────────────────────────────────────────────

func section(w io.Writer, name string) {
	fmt.Fprintf(w, "\n[%s]\n", name)
}

func emptyAs(s, alt string) string {
	if s == "" {
		return alt
	}
	return s
}

// (tools is imported but only used transitively via printToolsStatus;
// keep the import to make grep -r "tools\." in this file consistent.)
var _ = tools.Available
