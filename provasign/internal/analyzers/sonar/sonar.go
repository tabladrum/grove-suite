// Package sonar adapts SonarSource's official SonarLint Language Server
// (the same jar bundled with the SonarLint VS Code extension) into the
// Stage-2 analyzer contract. No SonarQube server is required; the
// engine runs entirely locally against the worktree.
//
// Topology — mirrors sonarlint-vscode exactly:
//
//	relay (Go)
//	   └─ exec.Command(java -jar sonarlint-ls.jar -stdio
//	                        -analyzers <plugin1.jar> <plugin2.jar> ...)
//	         └─ LSP / JSON-RPC 2.0 over stdio
//	              ↳ initialize → didOpen(file) → publishDiagnostics
//	                → shutdown
//
// Both binaries (sonarlint-language-server and per-language analyzer
// plugin jars) are downloaded straight from Maven Central by
// `provasign tools install --with-sonar`; see internal/tools/registry.go.
// We do not build any Java ourselves — SonarSource already publishes
// everything we need.
//
// New-vs-full-code: scope-based filtering happens upstream in stage2 via
// core.AddedLineMap, identical to every other analyzer. The adapter
// itself opts to only `didOpen` files that appear in the diff — that
// keeps wall-clock time proportional to the patch size, which is the
// behaviour PR validation actually wants.
package sonar

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/provasign/provasign/internal/analyzers"
	"github.com/provasign/provasign/internal/cert"
	"github.com/provasign/provasign/internal/core"
	"github.com/provasign/provasign/internal/tools"
)

// Analyzer drives SonarLint Language Server over LSP.
type Analyzer struct {
	// ProfilePath is the absolute or .provasign/-relative path to the
	// quality-profile XML (the file written by `relay import
	// sonarqube-profile`). Currently unused by the LSP-based adapter
	// (the server's built-in Sonar Way profiles are applied); the
	// field is retained so existing configs keep loading.
	ProfilePath string
	// ExtraArgs is reserved for future per-call overrides.
	ExtraArgs []string
}

// New constructs a SonarLint adapter from its config block.
func New(_ string, cfg core.AnalyzerConfig) (analyzers.Analyzer, error) {
	a := &Analyzer{ExtraArgs: cfg.ExtraArgs}
	if len(cfg.RulePacks) > 0 {
		a.ProfilePath = cfg.RulePacks[0]
	}
	return a, nil
}

// Name implements analyzers.Analyzer.
func (*Analyzer) Name() string { return "sonar" }

// Available reports whether sonarlint-ls.jar AND a compatible JRE are
// installed. Both come from `provasign tools install --with-sonar`.
func (*Analyzer) Available() bool {
	if WrapperPath() == "" {
		return false
	}
	return javaPath() != ""
}

// Run spins up sonarlint-language-server, opens every changed source
// file, drains publishDiagnostics until idle, then shuts the server
// down. Returns one cert.Finding per LSP Diagnostic.
func (a *Analyzer) Run(ctx context.Context, cs *core.ChangeSet, dir string) ([]cert.Finding, error) {
	jar := WrapperPath()
	if jar == "" {
		return nil, nil
	}
	java := javaPath()
	if java == "" {
		return nil, nil
	}

	// Build the launch command verbatim per sonarlint-vscode's
	// languageServerCommand(). The `-stdio` flag + Content-Length-framed
	// JSON-RPC is the same protocol VS Code speaks.
	args := []string{
		"-Dsonarlint.telemetry.disabled=true",
		"-Dsonarlint.monitoring.enabled=false",
		"-jar", jar,
		"-stdio",
	}
	plugins := tools.SonarPluginJars()
	if len(plugins) > 0 {
		args = append(args, "-analyzers")
		args = append(args, plugins...)
	}

	cmd := exec.CommandContext(ctx, java, args...)
	cmd.Dir = dir
	client, err := newLSPClient(cmd)
	if err != nil {
		return nil, fmt.Errorf("start sonarlint-ls: %w", err)
	}
	// Ensure the JVM is reaped even on early errors.
	defer func() {
		shCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = client.shutdown(shCtx)
	}()

	// LSP handshake.
	rootURI := pathToFileURI(dir)
	initParams := map[string]any{
		"processId": nil,
		"clientInfo": map[string]any{
			"name":    "Visual Studio Code",
			"version": "0.1.0",
		},
		"rootUri": rootURI,
		"workspaceFolders": []map[string]any{
			{"uri": rootURI, "name": filepath.Base(dir)},
		},
		"capabilities": map[string]any{
			"textDocument": map[string]any{
				"publishDiagnostics": map[string]any{
					"relatedInformation": true,
				},
			},
		},
		"initializationOptions": map[string]any{
			// These mirror the values sonarlint-vscode sends in
			// extension.ts → clientOptions.initializationOptions. Missing
			// any of productKey/productName/productVersion makes the
			// server accept initialize() but silently refuse to analyze.
			"productKey":       "relay",
			"productName":      "Relay",
			"productVersion":   "0.1.0",
			"workspaceName":    filepath.Base(dir),
			"telemetryStorage": filepath.Join(os.TempDir(), "relay-sonarlint-telemetry"),
			"enableTelemetry":  false,
			"showVerboseLogs":  os.Getenv("RELAY_SONAR_DEBUG") != "",
			"showAnalyzerLogs": os.Getenv("RELAY_SONAR_DEBUG") != "",
			"platform":         runtime.GOOS,
			"architecture":     runtime.GOARCH,
			"enableNotebooks":  false,
			"connections": map[string]any{
				"sonarqube":  []any{},
				"sonarcloud": []any{},
			},
			"rules":             map[string]any{},
			"focusOnNewCode":    false,
			"automaticAnalysis": true,
		},
	}
	if _, err := client.request(ctx, "initialize", initParams); err != nil {
		return nil, fmt.Errorf("sonarlint-ls initialize: %w", err)
	}
	if err := client.notify("initialized", map[string]any{}); err != nil {
		return nil, fmt.Errorf("sonarlint-ls initialized: %w", err)
	}

	// Pick the set of files to scan. Default: every added/modified path
	// in the diff. If there's no diff (full scan / standalone run),
	// fall back to a bounded worktree walk so the adapter still has
	// something to do.
	targets := changedTargets(cs, dir)
	if len(targets) == 0 {
		targets = walkTargets(dir, 500)
	}

	openedCount := 0
	for _, abs := range targets {
		lang := languageIDForExt(filepath.Ext(abs))
		if lang == "" {
			continue
		}
		body, err := os.ReadFile(abs)
		if err != nil {
			continue
		}
		uri := pathToFileURI(abs)
		_ = client.notify("textDocument/didOpen", map[string]any{
			"textDocument": map[string]any{
				"uri":        uri,
				"languageId": lang,
				"version":    1,
				"text":       string(body),
			},
		})
		openedCount++
	}
	if openedCount == 0 {
		return nil, nil
	}

	// Wait for diagnostics. SonarLint's backend takes 5–15 s to finish
	// booting after `initialized` — it loads all analyzer plugin jars
	// before processing any didOpen. We therefore use a generous 10 s
	// idle window with a per-file hard ceiling.
	idle := 10 * time.Second
	maxWait := time.Duration(60+openedCount*5) * time.Second
	if maxWait > 5*time.Minute {
		maxWait = 5 * time.Minute
	}
	batches := client.drainDiagnostics(idle, maxWait)

	// Convert LSP diagnostics → cert.Finding. Path is the URI minus
	// the worktree prefix, matching how every other analyzer reports
	// paths.
	dirSlash := strings.TrimSuffix(filepath.ToSlash(dir), "/") + "/"
	var out []cert.Finding
	for _, b := range batches {
		absPath := fileURIToPath(b.URI)
		rel := absPath
		if strings.HasPrefix(filepath.ToSlash(absPath), dirSlash) {
			rel = strings.TrimPrefix(filepath.ToSlash(absPath), dirSlash)
		}
		for _, d := range b.Diagnostics {
			ruleID := d.codeString()
			f := cert.Finding{
				Schema:   cert.FindingsSchemaVersion,
				Analyzer: "sonar",
				Category: categoryForRule(ruleID),
				RuleID:   ruleID,
				Severity: severityFromLSP(d.Severity),
				Path:     rel,
				Line:     d.Range.Start.Line + 1, // LSP is 0-based
				Column:   d.Range.Start.Character + 1,
				Message:  d.Message,
				Extra: map[string]any{
					"source": d.Source,
				},
			}
			f.Fingerprint = cert.ComputeFingerprint(f.Analyzer, f.RuleID, f.Path, f.Line, f.Message)
			out = append(out, f)
		}
	}
	return out, nil
}

// changedTargets resolves the absolute paths of every added/modified
// file in cs.Diff that exists on disk under dir.
func changedTargets(cs *core.ChangeSet, dir string) []string {
	if cs == nil || cs.Diff == "" {
		return nil
	}
	added := core.ParseAddedLines(cs.Diff)
	if len(added) == 0 {
		return nil
	}
	var out []string
	for path := range added {
		abs := filepath.Join(dir, filepath.FromSlash(path))
		if _, err := os.Stat(abs); err == nil {
			out = append(out, abs)
		}
	}
	return out
}

// walkTargets returns up to `limit` source files under dir whose
// extensions are recognized by one of the bundled analyzers. Skips
// .git, node_modules, vendor, and dotfile directories.
func walkTargets(dir string, limit int) []string {
	var out []string
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if p != dir && (name == ".git" || name == "node_modules" || name == "vendor" ||
				name == "target" || name == "build" || name == "dist" ||
				(len(name) > 1 && name[0] == '.')) {
				return filepath.SkipDir
			}
			return nil
		}
		if languageIDForExt(filepath.Ext(p)) == "" {
			return nil
		}
		out = append(out, p)
		if len(out) >= limit {
			return filepath.SkipAll
		}
		return nil
	})
	return out
}

// languageIDForExt maps a file extension to the LSP `languageId` the
// SonarLint server expects. Returns "" for extensions no bundled
// analyzer handles.
func languageIDForExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".java":
		return "java"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "javascript"
	case ".ts", ".tsx":
		return "typescript"
	case ".py":
		return "python"
	case ".go":
		return "go"
	case ".html", ".htm":
		return "html"
	case ".xml":
		return "xml"
	case ".yaml", ".yml":
		return "yaml"
	case ".json":
		return "json"
	case ".tf":
		return "terraform"
	case ".dockerfile":
		return "dockerfile"
	}
	return ""
}

// severityFromLSP maps LSP DiagnosticSeverity (1..4) to cert.Severity.
// LSP: 1=Error, 2=Warning, 3=Information, 4=Hint.
func severityFromLSP(s int) cert.Severity {
	switch s {
	case 1:
		return cert.SeverityHigh
	case 2:
		return cert.SeverityMedium
	case 3:
		return cert.SeverityLow
	case 4:
		return cert.SeverityInfo
	default:
		return cert.SeverityMedium
	}
}

// categoryForRule classifies a Sonar rule key into a cert.Category.
// Sonar rule keys look like "java:S1234" or "javasecurity:S5145"; the
// `*security:` prefixes denote SAST/vulnerability rules.
func categoryForRule(ruleID string) cert.Category {
	r := strings.ToLower(ruleID)
	if strings.Contains(r, "security") || strings.HasPrefix(r, "secrets:") {
		return cert.CategoryVuln
	}
	return cert.CategorySAST
}

// pathToFileURI converts an absolute filesystem path to a `file://` URI.
func pathToFileURI(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		abs = p
	}
	u := &url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}
	if runtime.GOOS == "windows" && !strings.HasPrefix(u.Path, "/") {
		u.Path = "/" + u.Path
	}
	return u.String()
}

// fileURIToPath inverts pathToFileURI.
func fileURIToPath(uri string) string {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" {
		return uri
	}
	p := u.Path
	if runtime.GOOS == "windows" && strings.HasPrefix(p, "/") && len(p) > 2 && p[2] == ':' {
		p = p[1:]
	}
	return filepath.FromSlash(p)
}

// WrapperPath returns the absolute path to sonarlint-ls.jar, or "" when
// not installed. Honors $RELAY_SONAR_JAR for overrides.
func WrapperPath() string {
	if env := os.Getenv("RELAY_SONAR_JAR"); env != "" {
		if _, err := os.Stat(env); err == nil {
			return env
		}
	}
	if root, err := tools.DefaultRoot(); err == nil {
		p := filepath.Join(root, "sonar", "sonarlint-ls.jar")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// MinJavaMajor is the lowest JRE feature version sonarlint-ls can run
// on. SonarLint 3.24 requires Java 17+.
const MinJavaMajor = 17

// JavaPath returns the absolute path to a java binary suitable for
// running sonarlint-ls. Search order:
//  1. $RELAY_JAVA (explicit override; no version check)
//  2. Bundled JRE installed by `provasign tools install --with-sonar`
//  3. $JAVA_HOME/bin/java if its `java -version` reports >= MinJavaMajor
//  4. exec.LookPath("java") if its version is >= MinJavaMajor
func JavaPath() string { return javaPath() }

func javaPath() string {
	if env := os.Getenv("RELAY_JAVA"); env != "" {
		if _, err := os.Stat(env); err == nil {
			return env
		}
	}
	bin := "java"
	if runtime.GOOS == "windows" {
		bin = "java.exe"
	}
	if root, err := tools.DefaultRoot(); err == nil {
		p := filepath.Join(root, "jre-home", "bin", bin)
		if _, err := os.Stat(p); err == nil {
			return p // bundled — version guaranteed by registry pin
		}
	}
	if jh := os.Getenv("JAVA_HOME"); jh != "" {
		p := filepath.Join(jh, "bin", bin)
		if _, err := os.Stat(p); err == nil && javaVersionOK(p) {
			return p
		}
	}
	if p, err := exec.LookPath("java"); err == nil && javaVersionOK(p) {
		return p
	}
	return ""
}

func javaVersionOK(javaBin string) bool {
	cmd := exec.Command(javaBin, "-version")
	var buf bytes.Buffer
	cmd.Stderr = &buf
	cmd.Stdout = &buf
	if err := cmd.Run(); err != nil {
		return false
	}
	return parseJavaMajor(buf.String()) >= MinJavaMajor
}

// parseJavaMajor extracts the feature version from `java -version`
// output. Exposed for tests.
func parseJavaMajor(out string) int {
	i := strings.Index(out, "\"")
	if i < 0 {
		return 0
	}
	rest := out[i+1:]
	j := strings.Index(rest, "\"")
	if j < 0 {
		return 0
	}
	ver := rest[:j]
	parts := strings.Split(ver, ".")
	if len(parts) == 0 {
		return 0
	}
	first := parts[0]
	if first == "1" && len(parts) >= 2 {
		return atoiSafe(parts[1])
	}
	return atoiSafe(first)
}

func atoiSafe(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	return n
}

func init() {
	analyzers.DefaultRegistry.Register("sonar", New)
}
