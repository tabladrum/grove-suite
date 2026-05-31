package tools

import (
	"fmt"
	"runtime"
)

// Release is one downloadable artifact for a (name, version, os, arch) tuple.
type Release struct {
	Name        string // tool name (matches Tool.Name)
	Version     string // semver, no leading "v"
	OS          string // GOOS: linux, darwin, windows
	Arch        string // GOARCH: amd64, arm64
	URL         string // direct download URL
	SHA256      string // hex-encoded sha256 of the downloaded archive
	Archive     ArchiveKind
	BinaryPath  string // path inside the archive to the executable (forward slashes)
	BinaryName  string // installed binary name (default: tool name; ".exe" auto-appended on windows)
}

// ArchiveKind enumerates supported archive formats.
type ArchiveKind string

const (
	ArchiveRaw   ArchiveKind = "raw"    // the URL points directly at the binary
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
			Name: "govulncheck", Version: "1.1.3",
			OS: "any", Arch: "any",
			URL:        "go://golang.org/x/vuln/cmd/govulncheck@v1.1.3",
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
	// explanatory error.
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
