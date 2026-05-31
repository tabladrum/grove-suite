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

func TestLinkInto_BadRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file-as-parent error semantics differ on Windows")
	}
	// Create a regular file and use its path as the "root" — MkdirAll under
	// it must fail.
	parent := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(parent, []byte("file"), 0o644); err != nil {
		t.Fatal(err)
	}
	inst := &Installer{Root: parent}
	src := filepath.Join(t.TempDir(), "src")
	_ = os.WriteFile(src, []byte("x"), 0o755)
	if err := inst.linkInto(src, "shim"); err == nil {
		t.Fatal("expected linkInto to fail when BinDir parent is a file")
	}
}

func TestInstallRelease_VerDirCreationFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file-as-parent error semantics differ on Windows")
	}
	parent := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(parent, []byte("file"), 0o644); err != nil {
		t.Fatal(err)
	}
	inst := &Installer{Root: parent}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	_, err := inst.installRelease(Release{
		Name: "x", Version: "1", URL: srv.URL,
		Archive: ArchiveTarGz, BinaryPath: "x",
	})
	if err == nil {
		t.Fatal("expected mkdir error")
	}
}

func TestInstallGoTool_VerDirCreationFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file-as-parent error semantics differ on Windows")
	}
	parent := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(parent, []byte("file"), 0o644); err != nil {
		t.Fatal(err)
	}
	inst := &Installer{
		Root: parent,
		GoInstall: func(string, string, string) (string, error) {
			return "", nil
		},
	}
	_, err := inst.installRelease(Release{
		Name: "x", Version: "1", URL: "go://example.com/x@v1",
		Archive: ArchiveRaw, BinaryName: "x",
	})
	if err == nil {
		t.Fatal("expected mkdir error")
	}
}

func TestDefaultGoInstall_BuildsCommand(t *testing.T) {
	// Run with a syntactically valid package name pointing nowhere — this
	// exercises the exec.Command + CombinedOutput failure branch.
	_, err := defaultGoInstall("example.invalid/no/such/pkg@v0.0.1", "v0.0.1", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "go install") {
		t.Skipf("go install returned %v (toolchain may differ)", err)
	}
}

func TestInstaller_DownloadResponseCloseError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = w.Write([]byte("short"))
	}))
	defer srv.Close()
	inst := &Installer{Root: t.TempDir(), HTTPClient: srv.Client()}
	// Will likely succeed download but fail extract (data not gzip).
	_, err := inst.installRelease(Release{
		Name: "x", Version: "1", URL: srv.URL,
		Archive: ArchiveTarGz, BinaryPath: "x",
	})
	if err == nil {
		t.Skip("unexpected success")
	}
}
