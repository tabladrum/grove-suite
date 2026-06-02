package tools

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// buildTarGz returns an in-memory tar.gz containing a single executable file
// at path bin/<name> with the given contents.
func buildTarGz(t *testing.T, binPath, contents string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{
		Name: binPath,
		Mode: 0o755,
		Size: int64(len(contents)),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(contents)); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func TestInstaller_TarGzHappyPath(t *testing.T) {
	data := buildTarGz(t, "bin/fake-tool", "#!/bin/sh\necho fake\n")
	wantSHA := sha256Hex(data)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	root := t.TempDir()
	inst := &Installer{Root: root, HTTPClient: srv.Client()}
	rel := Release{
		Name:       "fake-tool",
		Version:    "1.2.3",
		OS:         "any",
		Arch:       "any",
		URL:        srv.URL + "/fake.tar.gz",
		SHA256:     wantSHA,
		Archive:    ArchiveTarGz,
		BinaryPath: "bin/fake-tool",
		BinaryName: "fake-tool",
	}
	bin, err := inst.installRelease(rel)
	if err != nil {
		t.Fatalf("installRelease: %v", err)
	}
	if _, err := os.Stat(bin); err != nil {
		t.Fatalf("expected installed binary at %s: %v", bin, err)
	}
	shim := filepath.Join(inst.BinDir(), binName(rel))
	if _, err := os.Stat(shim); err != nil {
		t.Fatalf("expected shim at %s: %v", shim, err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Lstat(shim)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Errorf("expected shim to be a symlink on POSIX, got mode %v", info.Mode())
		}
	}
}

func TestInstaller_SHA256Mismatch(t *testing.T) {
	data := buildTarGz(t, "bin/fake", "x")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	root := t.TempDir()
	inst := &Installer{Root: root, HTTPClient: srv.Client()}
	_, err := inst.installRelease(Release{
		Name:       "fake",
		Version:    "1.0.0",
		URL:        srv.URL + "/x.tar.gz",
		SHA256:     strings.Repeat("a", 64),
		Archive:    ArchiveTarGz,
		BinaryPath: "bin/fake",
	})
	if err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("expected sha256 mismatch error, got %v", err)
	}
}

func TestInstaller_SkipSHAWhenEmpty(t *testing.T) {
	data := buildTarGz(t, "bin/fake", "y")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	root := t.TempDir()
	inst := &Installer{Root: root, HTTPClient: srv.Client()}
	if _, err := inst.installRelease(Release{
		Name:       "fake",
		Version:    "1.0.0",
		URL:        srv.URL + "/x.tar.gz",
		SHA256:     "", // skip verify
		Archive:    ArchiveTarGz,
		BinaryPath: "bin/fake",
	}); err != nil {
		t.Fatalf("expected install without sha, got %v", err)
	}
}

func TestInstaller_GoTool_UsesSeam(t *testing.T) {
	root := t.TempDir()
	var calledPkg string
	inst := &Installer{
		Root: root,
		GoInstall: func(pkg, version, destDir string) (string, error) {
			calledPkg = pkg
			bin := filepath.Join(destDir, "fakego")
			if err := os.WriteFile(bin, []byte("ok"), 0o755); err != nil {
				return "", err
			}
			return bin, nil
		},
	}
	rel := Release{
		Name:       "fakego",
		Version:    "v1.0.0",
		URL:        "go://example.com/fake@v1.0.0",
		Archive:    ArchiveRaw,
		BinaryName: "fakego",
	}
	got, err := inst.installRelease(rel)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if calledPkg != "example.com/fake@v1.0.0" {
		t.Errorf("seam not called with package URL, got %q", calledPkg)
	}
	if !strings.HasPrefix(got, root) {
		t.Errorf("expected installed bin under root %s, got %s", root, got)
	}
}

func TestSafeExtract_RejectsZipSlip(t *testing.T) {
	dest := t.TempDir()
	err := safeExtract(dest, "../etc/passwd", 0o644, false, bytes.NewReader([]byte("pwn")))
	if err == nil {
		t.Fatal("expected zip-slip refusal")
	}
}

func TestSafeExtract_RejectsAbsolute(t *testing.T) {
	dest := t.TempDir()
	err := safeExtract(dest, "/etc/passwd", 0o644, false, bytes.NewReader([]byte("pwn")))
	if err == nil {
		t.Fatal("expected absolute-path refusal")
	}
}

// silence unused-imports if a future test trims helpers.
var _ = io.EOF
