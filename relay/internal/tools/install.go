package tools

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

// DefaultRoot returns ~/.relay/tools (or RELAY_TOOLS_ROOT when set).
func DefaultRoot() (string, error) {
	if env := os.Getenv("RELAY_TOOLS_ROOT"); env != "" {
		return env, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".relay", "tools"), nil
}

// Installer downloads + verifies + extracts pinned releases into root.
// HTTPClient is injectable so tests can use httptest.
type Installer struct {
	Root       string
	HTTPClient *http.Client
	// GoInstall, when non-nil, replaces the default `go install` invocation
	// (test seam). Returns the absolute path to the installed binary.
	GoInstall func(pkg, version, destDir string) (string, error)
}

// NewInstaller returns an Installer rooted at DefaultRoot().
func NewInstaller() (*Installer, error) {
	root, err := DefaultRoot()
	if err != nil {
		return nil, err
	}
	return &Installer{Root: root, HTTPClient: http.DefaultClient}, nil
}

// BinDir is the per-installation shim directory; Locate() prefers it.
func (i *Installer) BinDir() string { return filepath.Join(i.Root, "bin") }

// versionDir is the per-(tool, version) install root.
func (i *Installer) versionDir(name, version string) string {
	return filepath.Join(i.Root, name, version)
}

// Install fetches the pinned release for name on the current platform,
// verifies its SHA-256 (when the registry specifies one — empty means skip),
// extracts it, and links the binary into BinDir. Returns the absolute path
// to the installed binary.
func (i *Installer) Install(name string) (string, error) {
	rel, err := ReleaseFor(name)
	if err != nil {
		return "", err
	}
	return i.installRelease(rel)
}

func (i *Installer) installRelease(rel Release) (string, error) {
	if strings.HasPrefix(rel.URL, "go://") {
		return i.installGoTool(rel)
	}
	verDir := i.versionDir(rel.Name, rel.Version)
	if err := os.MkdirAll(verDir, 0o755); err != nil {
		return "", err
	}

	body, err := i.download(rel.URL)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", rel.URL, err)
	}
	defer body.Close()

	tmp, err := os.CreateTemp(verDir, "dl-*")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, h), body); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if rel.SHA256 != "" && !strings.EqualFold(got, rel.SHA256) {
		return "", fmt.Errorf("sha256 mismatch for %s: want %s got %s", rel.URL, rel.SHA256, got)
	}

	switch rel.Archive {
	case ArchiveRaw:
		dst := filepath.Join(verDir, filepath.Base(rel.BinaryPath))
		if err := copyFile(tmpName, dst, 0o755); err != nil {
			return "", err
		}
	case ArchiveTarGz:
		if err := extractTarGz(tmpName, verDir); err != nil {
			return "", err
		}
	case ArchiveZip:
		if err := extractZip(tmpName, verDir); err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("unsupported archive kind %q", rel.Archive)
	}

	binSrc := filepath.Join(verDir, filepath.FromSlash(rel.BinaryPath))
	if _, err := os.Stat(binSrc); err != nil {
		return "", fmt.Errorf("binary not found after extract at %s: %w", binSrc, err)
	}
	// Layout dispatch: standard "shim" links into <root>/bin; "java-home"
	// republishes the JRE under <root>/jre/; "sonar-jar" copies the JAR
	// to <root>/sonar/relay-sonar.jar. Both special layouts are read by
	// the sonar analyzer at run time.
	switch rel.Layout {
	case "java-home":
		if err := i.publishJavaHome(verDir, binSrc); err != nil {
			return "", err
		}
		return binSrc, nil
	case "sonar-jar":
		if err := i.publishSonarJar(binSrc); err != nil {
			return "", err
		}
		return binSrc, nil
	case "sonar-plugin":
		// Analyzer plugin jars land under <root>/sonar/analyzers/ so the
		// LSP launcher can glob them at startup without each adapter
		// reaching back into the per-version cache.
		if err := i.publishSonarPlugin(binSrc); err != nil {
			return "", err
		}
		return binSrc, nil
	}
	if err := i.linkInto(binSrc, binName(rel)); err != nil {
		return "", err
	}
	return binSrc, nil
}

// publishJavaHome materializes the JRE at <root>/jre-home/ with a stable
// bin/java entry point. We copy rather than symlink the whole tree so
// macOS Gatekeeper doesn't flag the bundle as quarantined when java
// dlopens its sibling .dylibs across a symlink.
//
// Note the publish root (<root>/jre-home) is a *sibling* of the per-
// version cache (<root>/jre/<version>/...), not its parent: nuking the
// publish root must never touch the cache the copy reads from.
func (i *Installer) publishJavaHome(verDir, binSrc string) error {
	jreRoot := filepath.Join(i.Root, "jre-home")
	if err := os.RemoveAll(jreRoot); err != nil {
		return err
	}
	// binSrc is .../bin/java (linux/windows) or .../Contents/Home/bin/java
	// (macOS); the JAVA_HOME is two directories up from binSrc on linux,
	// three on macOS. Walk up to the directory that contains "bin/java".
	home := filepath.Dir(filepath.Dir(binSrc))
	if filepath.Base(filepath.Dir(home)) == "Contents" {
		// macOS layout: <home>/Contents/Home/bin/java; JAVA_HOME = <home>/Contents/Home
		// home is already correct (it's .../Contents/Home).
	}
	if err := copyTree(home, jreRoot); err != nil {
		return err
	}
	// Sanity check: the analyzer looks for <jreRoot>/bin/java(.exe).
	javaName := "java"
	if runtime.GOOS == "windows" {
		javaName = "java.exe"
	}
	if _, err := os.Stat(filepath.Join(jreRoot, "bin", javaName)); err != nil {
		return fmt.Errorf("publishJavaHome: %w", err)
	}
	_ = verDir // versionDir kept for cache provenance
	return nil
}

// publishSonarJar copies the wrapper JAR to its stable location.
func (i *Installer) publishSonarJar(binSrc string) error {
	dst := filepath.Join(i.Root, "sonar", "sonarlint-ls.jar")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return copyFile(binSrc, dst, 0o644)
}

// publishSonarPlugin copies an analyzer plugin jar to
// <root>/sonar/analyzers/<basename>. The launcher passes every jar in
// that directory to sonarlint-ls via -analyzers, so adding a plugin is
// just `relay tools install --only=sonar-php` with the matching
// registry entry.
func (i *Installer) publishSonarPlugin(binSrc string) error {
	dst := filepath.Join(i.Root, "sonar", "analyzers", filepath.Base(binSrc))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return copyFile(binSrc, dst, 0o644)
}

// copyTree copies the directory tree at src into dst. Existing dst is
// not removed (caller's responsibility). Preserves file mode bits.
func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		switch {
		case info.IsDir():
			return os.MkdirAll(target, info.Mode().Perm()|0o700)
		case info.Mode()&os.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		default:
			return copyFile(path, target, info.Mode().Perm())
		}
	})
}

func (i *Installer) installGoTool(rel Release) (string, error) {
	pkg := strings.TrimPrefix(rel.URL, "go://")
	verDir := i.versionDir(rel.Name, rel.Version)
	if err := os.MkdirAll(verDir, 0o755); err != nil {
		return "", err
	}
	var (
		bin string
		err error
	)
	if i.GoInstall != nil {
		bin, err = i.GoInstall(pkg, rel.Version, verDir)
	} else {
		bin, err = defaultGoInstall(pkg, rel.Version, verDir)
	}
	if err != nil {
		return "", err
	}
	if err := i.linkInto(bin, binName(rel)); err != nil {
		return "", err
	}
	return bin, nil
}

func defaultGoInstall(pkg, _, destDir string) (string, error) {
	if _, err := exec.LookPath("go"); err != nil {
		return "", errors.New("`go` toolchain not in PATH; install Go or use a binary release")
	}
	cmd := exec.Command("go", "install", pkg)
	cmd.Env = append(os.Environ(), "GOBIN="+destDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("go install %s: %w\n%s", pkg, err, out)
	}
	base := filepath.Base(strings.SplitN(pkg, "@", 2)[0])
	if runtime.GOOS == "windows" {
		base += ".exe"
	}
	return filepath.Join(destDir, base), nil
}

// installMavenJar and findMavenSource were removed when we switched
// from building our own SonarLint wrapper to downloading SonarSource's
// published sonarlint-language-server jar directly from Maven Central.
// See Registry["sonarlint-ls"] in registry.go.

func (i *Installer) linkInto(src, dstName string) error {
	if err := os.MkdirAll(i.BinDir(), 0o755); err != nil {
		return err
	}
	dst := filepath.Join(i.BinDir(), dstName)
	_ = os.Remove(dst)
	// Symlink on POSIX, copy on Windows (NTFS symlinks require elevated perms
	// by default).
	if runtime.GOOS == "windows" {
		return copyFile(src, dst, 0o755)
	}
	return os.Symlink(src, dst)
}

func binName(rel Release) string {
	n := rel.BinaryName
	if n == "" {
		n = rel.Name
	}
	if runtime.GOOS == "windows" && !strings.HasSuffix(n, ".exe") {
		n += ".exe"
	}
	return n
}

// download GETs url and returns the body. Non-200 is an error.
func (i *Installer) download(url string) (io.ReadCloser, error) {
	client := i.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		resp.Body.Close()
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return resp.Body, nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func extractTarGz(archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if err := safeExtract(destDir, hdr.Name, hdr.FileInfo().Mode(), hdr.Typeflag == tar.TypeDir, tr); err != nil {
			return err
		}
	}
}

func extractZip(archivePath, destDir string) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			return err
		}
		err = safeExtract(destDir, f.Name, f.Mode(), f.FileInfo().IsDir(), rc)
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// safeExtract writes one archive entry, defending against zip-slip.
func safeExtract(destDir, entryName string, mode os.FileMode, isDir bool, src io.Reader) error {
	clean := path.Clean(entryName)
	if strings.HasPrefix(clean, "/") || strings.HasPrefix(clean, "..") || strings.Contains(clean, "/../") {
		return fmt.Errorf("refusing to extract entry outside destDir: %q", entryName)
	}
	target := filepath.Join(destDir, filepath.FromSlash(clean))
	rel, err := filepath.Rel(destDir, target)
	if err != nil || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("zip-slip refused: %q", entryName)
	}
	if isDir {
		return os.MkdirAll(target, 0o755)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode|0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, src); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
