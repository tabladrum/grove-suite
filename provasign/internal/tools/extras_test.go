package tools

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func buildZip(t *testing.T, name, contents string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte(contents)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestInstaller_ZipExtract(t *testing.T) {
	data := buildZip(t, "bin/fake-zip", "zip-bin")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	root := t.TempDir()
	inst := &Installer{Root: root, HTTPClient: srv.Client()}
	rel := Release{
		Name:       "fake-zip",
		Version:    "1.0.0",
		URL:        srv.URL + "/x.zip",
		SHA256:     "", // skip verify
		Archive:    ArchiveZip,
		BinaryPath: "bin/fake-zip",
		BinaryName: "fake-zip",
	}
	if _, err := inst.installRelease(rel); err != nil {
		t.Fatalf("install zip: %v", err)
	}
	if _, err := os.Stat(filepath.Join(inst.BinDir(), binName(rel))); err != nil {
		t.Fatalf("shim missing: %v", err)
	}
}

func TestInstaller_RawArchive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("raw-binary-bytes"))
	}))
	defer srv.Close()

	root := t.TempDir()
	inst := &Installer{Root: root, HTTPClient: srv.Client()}
	if _, err := inst.installRelease(Release{
		Name:       "fake-raw",
		Version:    "1.0.0",
		URL:        srv.URL + "/x",
		Archive:    ArchiveRaw,
		BinaryPath: "fake-raw",
		BinaryName: "fake-raw",
	}); err != nil {
		t.Fatalf("raw: %v", err)
	}
}

func TestInstaller_Download404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	inst := &Installer{Root: t.TempDir(), HTTPClient: srv.Client()}
	_, err := inst.installRelease(Release{
		Name: "x", Version: "1.0.0", URL: srv.URL + "/missing",
		Archive: ArchiveTarGz, BinaryPath: "x",
	})
	if err == nil || !strings.Contains(err.Error(), "HTTP 404") {
		t.Fatalf("expected HTTP 404, got %v", err)
	}
}

func TestInstaller_UnsupportedArchive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("x"))
	}))
	defer srv.Close()
	inst := &Installer{Root: t.TempDir(), HTTPClient: srv.Client()}
	_, err := inst.installRelease(Release{
		Name: "x", Version: "1.0.0", URL: srv.URL + "/x",
		Archive: ArchiveKind("bogus"), BinaryPath: "x",
	})
	if err == nil {
		t.Fatal("expected error for unsupported archive kind")
	}
}

func TestInstaller_Install_LooksUpRegistry(t *testing.T) {
	inst := &Installer{Root: t.TempDir()}
	_, err := inst.Install("not-a-tool")
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
}

func TestNewInstaller_UsesDefaultRoot(t *testing.T) {
	t.Setenv("PROVASIGN_TOOLS_ROOT", t.TempDir())
	inst, err := NewInstaller()
	if err != nil {
		t.Fatal(err)
	}
	if inst.Root == "" {
		t.Error("expected non-empty root")
	}
	if inst.HTTPClient == nil {
		t.Error("expected default HTTP client")
	}
}

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(src, dst, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello" {
		t.Errorf("got %q", got)
	}
}

func TestBinaryFileName_WindowsSuffix(t *testing.T) {
	got := binaryFileName("tool")
	if runtime.GOOS == "windows" && !strings.HasSuffix(got, ".exe") {
		t.Errorf("expected .exe on Windows, got %q", got)
	}
	if runtime.GOOS != "windows" && got != "tool" {
		t.Errorf("expected plain on POSIX, got %q", got)
	}
}

func TestDefaultRoot_EnvOverride(t *testing.T) {
	t.Setenv("PROVASIGN_TOOLS_ROOT", "/some/override")
	got, err := DefaultRoot()
	if err != nil {
		t.Fatal(err)
	}
	if got != "/some/override" {
		t.Errorf("got %q", got)
	}
}

func TestReleaseFor_CurrentPlatform(t *testing.T) {
	// govulncheck registers OS=any so it must always resolve.
	if _, err := ReleaseFor("govulncheck"); err != nil {
		t.Fatalf("ReleaseFor(govulncheck): %v", err)
	}
}
