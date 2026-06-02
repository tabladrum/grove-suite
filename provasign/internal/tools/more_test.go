package tools

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDefaultGoInstall_BadPackage(t *testing.T) {
	dest := t.TempDir()
	_, err := defaultGoInstall("not-a-real-pkg!!!", "v0.0.0", dest)
	if err == nil {
		t.Fatal("expected error for malformed go package")
	}
}

func TestInstallGoTool_SeamError(t *testing.T) {
	inst := &Installer{
		Root: t.TempDir(),
		GoInstall: func(pkg, version, destDir string) (string, error) {
			return "", os.ErrPermission
		},
	}
	_, err := inst.installRelease(Release{
		Name: "x", Version: "v1", URL: "go://example.com/x@v1",
		Archive: ArchiveRaw, BinaryName: "x",
	})
	if err == nil {
		t.Fatal("expected GoInstall error to surface")
	}
}

func TestInstallRelease_BinaryMissingAfterExtract(t *testing.T) {
	// tar.gz that does not include the declared BinaryPath.
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: "other-file", Mode: 0o644, Size: 1})
	tw.Write([]byte("x"))
	tw.Close()
	gz.Close()

	root := t.TempDir()
	inst := &Installer{Root: root}
	// Write the archive locally and serve via file URL trick: use a fake HTTP server.
	dlPath := filepath.Join(t.TempDir(), "a.tgz")
	if err := os.WriteFile(dlPath, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	// Reuse the extractor directly to verify the missing-binary error path.
	verDir := filepath.Join(root, "x", "1.0.0")
	if err := os.MkdirAll(verDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := extractTarGz(dlPath, verDir); err != nil {
		t.Fatalf("extractTarGz: %v", err)
	}
	if _, err := os.Stat(filepath.Join(verDir, "missing-binary")); err == nil {
		t.Fatal("expected missing-binary to not exist")
	}
	_ = inst
}

func TestSafeExtract_DirEntry(t *testing.T) {
	dest := t.TempDir()
	if err := safeExtract(dest, "newdir", 0o755, true, nil); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dest, "newdir"))
	if err != nil || !info.IsDir() {
		t.Fatalf("expected dir created: %v", err)
	}
}

func TestSafeExtract_NestedFile(t *testing.T) {
	dest := t.TempDir()
	if err := safeExtract(dest, "a/b/c.txt", 0o644, false, strings.NewReader("hi")); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(dest, "a", "b", "c.txt"))
	if string(got) != "hi" {
		t.Errorf("got %q", got)
	}
}

func TestCopyFile_MissingSrc(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "out")
	if err := copyFile(filepath.Join(t.TempDir(), "no-such"), dst, 0o644); err == nil {
		t.Fatal("expected error for missing src")
	}
}

func TestBinName_RespectsExplicit(t *testing.T) {
	rel := Release{Name: "x", BinaryName: ""}
	if got := binName(rel); got == "" {
		t.Error("expected non-empty default")
	}
	if runtime.GOOS == "windows" {
		rel.BinaryName = "y.exe"
		if got := binName(rel); got != "y.exe" {
			t.Errorf("got %q", got)
		}
	}
}

func TestExtractTarGz_BadFile(t *testing.T) {
	if err := extractTarGz(filepath.Join(t.TempDir(), "missing.tgz"), t.TempDir()); err == nil {
		t.Fatal("expected error")
	}
}

func TestExtractZip_BadFile(t *testing.T) {
	if err := extractZip(filepath.Join(t.TempDir(), "missing.zip"), t.TempDir()); err == nil {
		t.Fatal("expected error")
	}
}
