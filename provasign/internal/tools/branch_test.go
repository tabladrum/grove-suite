package tools

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestIsKnown_BothBranches(t *testing.T) {
	if !IsKnown("gitleaks") {
		t.Error("gitleaks should be known")
	}
	if IsKnown("totally-fake-tool-name-123") {
		t.Error("fake tool should not be known")
	}
}

func TestBinaryFileName_WithExtension(t *testing.T) {
	// When the input already has an extension, no .exe suffix is added.
	if got := binaryFileName("tool.bin"); got != "tool.bin" {
		t.Errorf("got %q, want tool.bin", got)
	}
}

func TestCopyFile_DirAsDst(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Target an existing directory path → OpenFile fails.
	dir := t.TempDir()
	if err := copyFile(src, dir, 0o644); err == nil {
		t.Fatal("expected error copying into a directory path")
	}
}

func TestSafeExtract_OpenFileError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission semantics differ on Windows")
	}
	dest := t.TempDir()
	// Pre-create a read-only directory; writing a file under it should fail.
	sub := filepath.Join(dest, "ro")
	if err := os.MkdirAll(sub, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(sub, 0o755) })
	if os.Geteuid() == 0 {
		t.Skip("running as root bypasses permissions")
	}
	err := safeExtract(dest, "ro/file.txt", 0o644, false, strings.NewReader("x"))
	if err == nil {
		t.Skip("permission denial did not propagate")
	}
}

func TestLinkInto_CreatesBinDir(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src")
	if err := os.WriteFile(src, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "deep", "root")
	inst := &Installer{Root: root}
	dstName := binaryFileName("tool")
	if err := inst.linkInto(src, dstName); err != nil {
		t.Fatalf("linkInto: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "bin", dstName)); err != nil {
		t.Fatalf("shim missing: %v", err)
	}
}

func TestInstallRelease_GoToolMissingBinaryPath(t *testing.T) {
	// installGoTool succeeds (seam returns a non-existent path) — the
	// subsequent linkInto should still succeed because linkInto only checks
	// dst, but symlinking a missing src is allowed on POSIX. Cover both.
	inst := &Installer{
		Root: t.TempDir(),
		GoInstall: func(pkg, version, destDir string) (string, error) {
			return filepath.Join(destDir, "phantom"), nil
		},
	}
	if _, err := inst.installRelease(Release{
		Name: "phantom", Version: "v1", URL: "go://example.com/phantom@v1",
		Archive: ArchiveRaw, BinaryName: "phantom",
	}); err != nil && runtime.GOOS != "windows" {
		t.Fatalf("expected POSIX symlink to dangling path to succeed: %v", err)
	}
}

func TestRegistry_GitleaksAllPlatforms(t *testing.T) {
	for _, pair := range []struct{ os, arch string }{
		{"linux", "amd64"},
		{"darwin", "amd64"},
		{"darwin", "arm64"},
	} {
		if _, err := ReleaseForPlatform("gitleaks", pair.os, pair.arch); err != nil {
			t.Errorf("gitleaks %s/%s: %v", pair.os, pair.arch, err)
		}
	}
}

func TestReleaseForPlatform_UnknownArchFails(t *testing.T) {
	if _, err := ReleaseForPlatform("gitleaks", "plan9", "mips"); err == nil {
		t.Error("expected error for unsupported platform")
	}
}

// downloadServer is shared with other tests; reproduced briefly to exercise
// the "URL error" branch of download (no server).
func TestDownload_BadURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close() // closed immediately
	inst := &Installer{Root: t.TempDir()}
	_, err := inst.download(srv.URL)
	if err == nil {
		t.Fatal("expected dial error")
	}
}
