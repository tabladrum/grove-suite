package cli

import (
	"archive/tar"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// chdirKeys switches the test's CWD for the duration of the test. A separate
// helper from engine_commands_test.go::chdir to avoid duplicate-symbol clashes.
func chdirKeys(t *testing.T, dir string) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
}

// TestKeysGenCreatesKey covers the laptop-bootstrap path: a fresh `.provasign/`
// dir, `keys gen` produces signing.ed25519.{key,pub} with the right perms.
func TestKeysGenCreatesKey(t *testing.T) {
	dir := t.TempDir()
	chdirKeys(t, dir)
	if code := RunKeys([]string{"gen"}); code != 0 {
		t.Fatalf("keys gen exit = %d", code)
	}
	priv := filepath.Join(dir, ".provasign", "keys", "signing.ed25519.key")
	info, err := os.Stat(priv)
	if err != nil {
		t.Fatalf("priv key missing: %v", err)
	}
	// 0600 on POSIX. Windows does not honour chmod so skip the check there.
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		t.Errorf("priv key mode %v is too permissive", info.Mode().Perm())
	}
	pub := filepath.Join(dir, ".provasign", "keys", "signing.ed25519.pub")
	if _, err := os.Stat(pub); err != nil {
		t.Errorf("pub key missing: %v", err)
	}
}

// TestKeysGenFailsIfExists asserts the safety guard against accidentally
// regenerating a key (which would invalidate all prior cert signatures).
func TestKeysGenFailsIfExists(t *testing.T) {
	dir := t.TempDir()
	chdirKeys(t, dir)
	if code := RunKeys([]string{"gen"}); code != 0 {
		t.Fatalf("first gen exit = %d", code)
	}
	if code := RunKeys([]string{"gen"}); code == 0 {
		t.Errorf("second gen should fail without --force, got 0")
	}
	if code := RunKeys([]string{"gen", "--force"}); code != 0 {
		t.Errorf("gen --force should succeed, got %d", code)
	}
}

// TestKeysExportImportRoundTrip simulates `dev moves to new machine` —
// export the bundle, wipe the keys dir, import from the bundle, fingerprint
// is identical.
func TestKeysExportImportRoundTrip(t *testing.T) {
	dir1 := t.TempDir()
	chdirKeys(t, dir1)
	if code := RunKeys([]string{"gen"}); code != 0 {
		t.Fatalf("gen exit = %d", code)
	}
	bundlePath := filepath.Join(t.TempDir(), "bundle.tar")
	if code := RunKeys([]string{"export", "--out", bundlePath}); code != 0 {
		t.Fatalf("export exit = %d", code)
	}
	if _, err := os.Stat(bundlePath); err != nil {
		t.Fatalf("bundle missing: %v", err)
	}

	// Simulate "new machine" by switching to a fresh repo dir.
	dir2 := t.TempDir()
	chdirKeys(t, dir2)
	if code := RunKeys([]string{"import", bundlePath}); code != 0 {
		t.Fatalf("import exit = %d", code)
	}
	if _, err := os.Stat(filepath.Join(dir2, ".provasign", "keys", "signing.ed25519.key")); err != nil {
		t.Errorf("imported priv key missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir2, ".provasign", "keys", "signing.ed25519.pub")); err != nil {
		t.Errorf("imported pub key missing: %v", err)
	}

	// And re-running fingerprint must succeed.
	if code := RunKeys([]string{"fingerprint"}); code != 0 {
		t.Errorf("fingerprint exit = %d", code)
	}
}

// TestKeysImportRefusesUnexpectedFiles guards against tar-bomb-style bundles
// containing extra files Relay would otherwise blindly extract.
func TestKeysImportRefusesUnexpectedFiles(t *testing.T) {
	bundlePath := filepath.Join(t.TempDir(), "evil.tar")
	if err := writeMaliciousTar(bundlePath); err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	chdirKeys(t, dest)
	if code := RunKeys([]string{"import", bundlePath}); code == 0 {
		t.Errorf("import should reject bundle with unexpected file, got exit 0")
	}
	// And the stray file must not have been written into the dest dir.
	if _, err := os.Stat(filepath.Join(dest, ".provasign", "keys", "etc")); err == nil {
		t.Errorf("import should not have written stray file")
	}
}

// writeMaliciousTar builds a tarball with two legitimate key files plus a
// rogue extra entry. Uses archive/tar directly so the rogue record is a
// proper tar entry (not just appended bytes that tar.Reader skips).
func writeMaliciousTar(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	tw := tar.NewWriter(f)
	defer tw.Close()
	entries := []struct {
		name string
		body []byte
		mode int64
	}{
		{"signing.ed25519.key", make([]byte, 64), 0o600},
		{"signing.ed25519.pub", make([]byte, 32), 0o644},
		{"etc/passwd", []byte("root:x:0:0\n"), 0o644}, // rogue
	}
	for _, e := range entries {
		hdr := &tar.Header{Name: e.name, Mode: e.mode, Size: int64(len(e.body))}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if _, err := tw.Write(e.body); err != nil {
			return err
		}
	}
	return nil
}
