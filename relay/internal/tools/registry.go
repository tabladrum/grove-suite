package tools

import (
	"fmt"
	"runtime"
)

// Release is one downloadable artifact for a (name, version, os, arch) tuple.
type Release struct {
	Name       string // tool name (matches Tool.Name)
	Version    string // semver, no leading "v"
	OS         string // GOOS: linux, darwin, windows
	Arch       string // GOARCH: amd64, arm64
	URL        string // direct download URL
	SHA256     string // hex-encoded sha256 of the downloaded archive
	Archive    ArchiveKind
	BinaryPath string // path inside the archive to the executable (forward slashes)
	BinaryName string // installed binary name (default: tool name; ".exe" auto-appended on windows)
	// Layout controls where the installed artifact lands relative to the
	// installer root. Empty = default ("bin shim" — link into <root>/bin).
	// "java-home" = place the JRE at <root>/jre/ with the discovered
	// binary at <root>/jre/bin/java(.exe). "sonar-jar" = copy the JAR to
	// <root>/sonar/relay-sonar.jar. Both special layouts match what the
	// sonar analyzer's WrapperPath/javaPath helpers look for.
	Layout string
}

// ArchiveKind enumerates supported archive formats.
type ArchiveKind string

const (
	ArchiveRaw   ArchiveKind = "raw" // the URL points directly at the binary
	ArchiveTarGz ArchiveKind = "tar.gz"
	ArchiveZip   ArchiveKind = "zip"
)

// Registry is keyed by tool name; each value is the list of pinned releases
// across (version, os, arch). Builds of relay embed this table directly.
//
// To bump a tool: run `make tools-sync NAME=<n>`, paste the new entries here,
// commit. CI verifies every URL's checksum on a clean install.
//
// Initial pins (May 2026). Checksums are intentionally left as empty strings
// for the first cut so the install path is exercised without making CI depend
// on third-party CDN stability; production deployments should regenerate
// these via `relay tools sync` (added in MVP-L4b) which fetches and verifies.
var Registry = map[string][]Release{
	"gitleaks": {
		{
			Name: "gitleaks", Version: "8.18.4",
			OS: "linux", Arch: "amd64",
			URL:        "https://github.com/gitleaks/gitleaks/releases/download/v8.18.4/gitleaks_8.18.4_linux_x64.tar.gz",
			Archive:    ArchiveTarGz,
			BinaryPath: "gitleaks",
		},
		{
			Name: "gitleaks", Version: "8.18.4",
			OS: "darwin", Arch: "arm64",
			URL:        "https://github.com/gitleaks/gitleaks/releases/download/v8.18.4/gitleaks_8.18.4_darwin_arm64.tar.gz",
			Archive:    ArchiveTarGz,
			BinaryPath: "gitleaks",
		},
		{
			Name: "gitleaks", Version: "8.18.4",
			OS: "darwin", Arch: "amd64",
			URL:        "https://github.com/gitleaks/gitleaks/releases/download/v8.18.4/gitleaks_8.18.4_darwin_x64.tar.gz",
			Archive:    ArchiveTarGz,
			BinaryPath: "gitleaks",
		},
		{
			Name: "gitleaks", Version: "8.18.4",
			OS: "windows", Arch: "amd64",
			URL:        "https://github.com/gitleaks/gitleaks/releases/download/v8.18.4/gitleaks_8.18.4_windows_x64.zip",
			Archive:    ArchiveZip,
			BinaryPath: "gitleaks.exe",
		},
	},
	"govulncheck": {
		// govulncheck is installed via `go install`, not direct download.
		// Marked here so `tools list` reports it; Install() dispatches to
		// installGoTool when Archive=="" and URL starts with "go://".
		{
			Name: "govulncheck", Version: "1.3.0",
			OS: "any", Arch: "any",
			URL:        "go://golang.org/x/vuln/cmd/govulncheck@v1.3.0",
			Archive:    ArchiveRaw,
			BinaryPath: "govulncheck",
		},
	},
	"golangci-lint": {
		{
			Name: "golangci-lint", Version: "1.59.0",
			OS: "linux", Arch: "amd64",
			URL:        "https://github.com/golangci/golangci-lint/releases/download/v1.59.0/golangci-lint-1.59.0-linux-amd64.tar.gz",
			Archive:    ArchiveTarGz,
			BinaryPath: "golangci-lint-1.59.0-linux-amd64/golangci-lint",
		},
		{
			Name: "golangci-lint", Version: "1.59.0",
			OS: "darwin", Arch: "arm64",
			URL:        "https://github.com/golangci/golangci-lint/releases/download/v1.59.0/golangci-lint-1.59.0-darwin-arm64.tar.gz",
			Archive:    ArchiveTarGz,
			BinaryPath: "golangci-lint-1.59.0-darwin-arm64/golangci-lint",
		},
		{
			Name: "golangci-lint", Version: "1.59.0",
			OS: "darwin", Arch: "amd64",
			URL:        "https://github.com/golangci/golangci-lint/releases/download/v1.59.0/golangci-lint-1.59.0-darwin-amd64.tar.gz",
			Archive:    ArchiveTarGz,
			BinaryPath: "golangci-lint-1.59.0-darwin-amd64/golangci-lint",
		},
		{
			Name: "golangci-lint", Version: "1.59.0",
			OS: "windows", Arch: "amd64",
			URL:        "https://github.com/golangci/golangci-lint/releases/download/v1.59.0/golangci-lint-1.59.0-windows-amd64.zip",
			Archive:    ArchiveZip,
			BinaryPath: "golangci-lint-1.59.0-windows-amd64/golangci-lint.exe",
		},
	},
	// semgrep is intentionally absent: it is distributed as a Python package
	// (pip install semgrep), not a single binary, and Relay relies on the
	// system pip/pipx install. `relay tools install semgrep` returns an
	// explanatory error pointing the user at pipx.

	// jre: Eclipse Temurin 21 LTS. Pinned per OS/arch. Required for the
	// SonarLint Core wrapper. Extracted into ~/.relay/tools/jre/ with a
	// bin/java symlink the sonar analyzer resolves at run time.
	// SHAs left empty (see header note); `relay tools sync` regenerates.
	"jre": {
		{
			Name: "jre", Version: "21.0.5+11",
			OS: "linux", Arch: "amd64",
			URL:        "https://github.com/adoptium/temurin21-binaries/releases/download/jdk-21.0.5%2B11/OpenJDK21U-jre_x64_linux_hotspot_21.0.5_11.tar.gz",
			Archive:    ArchiveTarGz,
			BinaryPath: "jdk-21.0.5+11-jre/bin/java",
			Layout:     "java-home",
		},
		{
			Name: "jre", Version: "21.0.5+11",
			OS: "darwin", Arch: "arm64",
			URL:        "https://github.com/adoptium/temurin21-binaries/releases/download/jdk-21.0.5%2B11/OpenJDK21U-jre_aarch64_mac_hotspot_21.0.5_11.tar.gz",
			Archive:    ArchiveTarGz,
			BinaryPath: "jdk-21.0.5+11-jre/Contents/Home/bin/java",
			Layout:     "java-home",
		},
		{
			Name: "jre", Version: "21.0.5+11",
			OS: "darwin", Arch: "amd64",
			URL:        "https://github.com/adoptium/temurin21-binaries/releases/download/jdk-21.0.5%2B11/OpenJDK21U-jre_x64_mac_hotspot_21.0.5_11.tar.gz",
			Archive:    ArchiveTarGz,
			BinaryPath: "jdk-21.0.5+11-jre/Contents/Home/bin/java",
			Layout:     "java-home",
		},
		{
			Name: "jre", Version: "21.0.5+11",
			OS: "windows", Arch: "amd64",
			URL:        "https://github.com/adoptium/temurin21-binaries/releases/download/jdk-21.0.5%2B11/OpenJDK21U-jre_x64_windows_hotspot_21.0.5_11.zip",
			Archive:    ArchiveZip,
			BinaryPath: "jdk-21.0.5+11-jre/bin/java.exe",
			Layout:     "java-home",
		},
	},
	// sonarlint-ls: SonarLint Language Server, the same shaded jar
	// VS Code's SonarLint extension bundles. We download it directly
	// from Maven Central (the artifact SonarSource publishes) instead
	// of building our own wrapper. Launch contract (per
	// sonarlint-vscode/src/lsp/server.ts):
	//
	//   java -jar sonarlint-ls.jar -stdio -analyzers <plugin1.jar> ...
	//
	// The Go-side adapter speaks LSP/JSON-RPC over stdio.
	"sonarlint-ls": {
		{
			Name: "sonarlint-ls", Version: "5.3.0.78466",
			OS: "any", Arch: "any",
			URL:        "https://repo1.maven.org/maven2/org/sonarsource/sonarlint/ls/sonarlint-language-server/5.3.0.78466/sonarlint-language-server-5.3.0.78466.jar",
			Archive:    ArchiveRaw,
			BinaryPath: "sonarlint-ls.jar",
			Layout:     "sonar-jar",
		},
	},
	// Analyzer plugin jars, fetched separately so users can opt in/out per
	// language. Versions match the set bundled by sonarlint-vscode for LS
	// 5.3.x (see sonarlint-vscode/package.json jarDependencies).
	"sonar-java": {
		{
			Name: "sonar-java", Version: "8.29.0.43460", OS: "any", Arch: "any",
			URL:        "https://repo1.maven.org/maven2/org/sonarsource/java/sonar-java-plugin/8.29.0.43460/sonar-java-plugin-8.29.0.43460.jar",
			Archive:    ArchiveRaw,
			BinaryPath: "sonar-java.jar",
			Layout:     "sonar-plugin",
		},
	},
	"sonar-java-symbolic-execution": {
		{
			Name: "sonar-java-symbolic-execution", Version: "8.16.4.1912", OS: "any", Arch: "any",
			URL:        "https://repo1.maven.org/maven2/org/sonarsource/java/sonar-java-symbolic-execution-plugin/8.16.4.1912/sonar-java-symbolic-execution-plugin-8.16.4.1912.jar",
			Archive:    ArchiveRaw,
			BinaryPath: "sonar-java-symbolic-execution.jar",
			Layout:     "sonar-plugin",
		},
	},
	"sonar-javascript": {
		{
			Name: "sonar-javascript", Version: "12.5.0.41048", OS: "any", Arch: "any",
			URL:        "https://repo1.maven.org/maven2/org/sonarsource/javascript/sonar-javascript-plugin/12.5.0.41048/sonar-javascript-plugin-12.5.0.41048.jar",
			Archive:    ArchiveRaw,
			BinaryPath: "sonar-javascript.jar",
			Layout:     "sonar-plugin",
		},
	},
	"sonar-python": {
		{
			Name: "sonar-python", Version: "5.22.0.33216", OS: "any", Arch: "any",
			URL:        "https://repo1.maven.org/maven2/org/sonarsource/python/sonar-python-plugin/5.22.0.33216/sonar-python-plugin-5.22.0.33216.jar",
			Archive:    ArchiveRaw,
			BinaryPath: "sonar-python.jar",
			Layout:     "sonar-plugin",
		},
	},
	"sonar-go": {
		{
			Name: "sonar-go", Version: "1.38.0.6335", OS: "any", Arch: "any",
			URL:        "https://repo1.maven.org/maven2/org/sonarsource/go/sonar-go-plugin/1.38.0.6335/sonar-go-plugin-1.38.0.6335.jar",
			Archive:    ArchiveRaw,
			BinaryPath: "sonar-go.jar",
			Layout:     "sonar-plugin",
		},
	},
	"sonar-text": {
		{
			Name: "sonar-text", Version: "2.43.0.11106", OS: "any", Arch: "any",
			URL:        "https://repo1.maven.org/maven2/org/sonarsource/text/sonar-text-plugin/2.43.0.11106/sonar-text-plugin-2.43.0.11106.jar",
			Archive:    ArchiveRaw,
			BinaryPath: "sonar-text.jar",
			Layout:     "sonar-plugin",
		},
	},
	"sonar-html": {
		{
			Name: "sonar-html", Version: "3.26.0.7600", OS: "any", Arch: "any",
			URL:        "https://repo1.maven.org/maven2/org/sonarsource/html/sonar-html-plugin/3.26.0.7600/sonar-html-plugin-3.26.0.7600.jar",
			Archive:    ArchiveRaw,
			BinaryPath: "sonar-html.jar",
			Layout:     "sonar-plugin",
		},
	},
	"sonar-xml": {
		{
			Name: "sonar-xml", Version: "2.17.0.7895", OS: "any", Arch: "any",
			URL:        "https://repo1.maven.org/maven2/org/sonarsource/xml/sonar-xml-plugin/2.17.0.7895/sonar-xml-plugin-2.17.0.7895.jar",
			Archive:    ArchiveRaw,
			BinaryPath: "sonar-xml.jar",
			Layout:     "sonar-plugin",
		},
	},
	"sonar-iac": {
		{
			Name: "sonar-iac", Version: "2.10.0.20406", OS: "any", Arch: "any",
			URL:        "https://repo1.maven.org/maven2/org/sonarsource/iac/sonar-iac-plugin/2.10.0.20406/sonar-iac-plugin-2.10.0.20406.jar",
			Archive:    ArchiveRaw,
			BinaryPath: "sonar-iac.jar",
			Layout:     "sonar-plugin",
		},
	},
}

// ReleaseFor returns the registry entry for (name, current OS, current arch),
// or an error if no match exists.
func ReleaseFor(name string) (Release, error) {
	return ReleaseForPlatform(name, runtime.GOOS, runtime.GOARCH)
}

// ReleaseForPlatform is ReleaseFor with explicit OS/arch (test seam).
func ReleaseForPlatform(name, goos, goarch string) (Release, error) {
	entries, ok := Registry[name]
	if !ok {
		return Release{}, fmt.Errorf("tools: %q is not in the registry", name)
	}
	for _, r := range entries {
		if r.OS == "any" || (r.OS == goos && r.Arch == goarch) {
			return r, nil
		}
	}
	return Release{}, fmt.Errorf("tools: no pinned release for %s on %s/%s", name, goos, goarch)
}
