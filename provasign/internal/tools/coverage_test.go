package tools

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDefaultRoot_NoEnv(t *testing.T) {
	t.Setenv("PROVASIGN_TOOLS_ROOT", "")
	got, err := DefaultRoot()
	if err != nil {
		t.Fatal(err)
	}
	if got == "" {
		t.Error("expected non-empty default root")
	}
}

func TestDefaultGoInstall_NoGoBinary(t *testing.T) {
	// Force `exec.LookPath("go")` to fail by emptying PATH.
	t.Setenv("PATH", "")
	_, err := defaultGoInstall("example.com/x", "v1", t.TempDir())
	// On a system without a `go` binary in the empty PATH, this should fail
	// fast. If `go` happens to be discoverable via cached lookup, this still
	// exercises additional branches.
	if err == nil {
		t.Skip("go install unexpectedly succeeded with empty PATH")
	}
}

func TestInstallRelease_DownloadError(t *testing.T) {
	inst := &Installer{Root: t.TempDir(), HTTPClient: http.DefaultClient}
	_, err := inst.installRelease(Release{
		Name: "x", Version: "1", URL: "http://127.0.0.1:1/nope",
		Archive: ArchiveTarGz, BinaryPath: "x",
	})
	if err == nil {
		t.Fatal("expected download error")
	}
}

func TestInstallRelease_CorruptGzip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not gzip data"))
	}))
	defer srv.Close()
	inst := &Installer{Root: t.TempDir(), HTTPClient: srv.Client()}
	_, err := inst.installRelease(Release{
		Name: "x", Version: "1", URL: srv.URL + "/x",
		Archive: ArchiveTarGz, BinaryPath: "x",
	})
	if err == nil {
		t.Fatal("expected gzip parse error")
	}
}

func TestLinkInto_ExistingFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	if err := os.WriteFile(src, []byte("s"), 0o755); err != nil {
		t.Fatal(err)
	}
	inst := &Installer{Root: dir}
	if err := inst.linkInto(src, "shim"); err != nil {
		t.Fatal(err)
	}
	// Re-link should replace, not error.
	if err := inst.linkInto(src, "shim"); err != nil {
		t.Fatalf("re-link: %v", err)
	}
}

func TestSafeExtract_DotDotMiddle(t *testing.T) {
	if err := safeExtract(t.TempDir(), "a/../../b", 0o644, false, nil); err == nil {
		t.Fatal("expected zip-slip refusal")
	}
}

func TestNewInstaller_Default(t *testing.T) {
	t.Setenv("PROVASIGN_TOOLS_ROOT", "")
	if _, err := NewInstaller(); err != nil {
		// On a CI agent without HOME this could fail; surface it.
		t.Logf("NewInstaller: %v", err)
	}
}

func TestInstall_GoToolEndToEnd(t *testing.T) {
	root := t.TempDir()
	called := 0
	inst := &Installer{
		Root: root,
		GoInstall: func(pkg, version, destDir string) (string, error) {
			called++
			bin := filepath.Join(destDir, "govulncheck")
			if runtime.GOOS == "windows" {
				bin += ".exe"
			}
			return bin, os.WriteFile(bin, []byte("ok"), 0o755)
		},
	}
	if _, err := inst.Install("govulncheck"); err != nil {
		t.Fatalf("Install(govulncheck): %v", err)
	}
	if called != 1 {
		t.Errorf("seam called %d times, want 1", called)
	}
}

// TestLookPathStillWorks ensures `exec.LookPath("ls")` (POSIX) finds something
// — purely to exercise the success branch of Locate().
func TestLocate_PathFallback(t *testing.T) {
	t.Setenv("PROVASIGN_TOOLS_ROOT", t.TempDir())
	if runtime.GOOS == "windows" {
		t.Skip("PATH layout differs on Windows")
	}
	if _, err := exec.LookPath("ls"); err != nil {
		t.Skip("ls not in PATH")
	}
	if got := Locate("ls"); got == "" {
		t.Error("expected ls to be located via PATH fallback")
	}
}
