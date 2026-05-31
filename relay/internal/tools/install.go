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
	if err := os.MkdirAll(i.Root, 0o755); err != nil {
		return "", err
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
	if err := os.Chmod(binSrc, 0o755); err != nil && runtime.GOOS != "windows" {
		return "", err
	}
	if err := i.linkInto(binSrc, binName(rel)); err != nil {
		return "", err
	}
	return binSrc, nil
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
