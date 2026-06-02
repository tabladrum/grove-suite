package nodetest

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDetect(t *testing.T) {
	dir := t.TempDir()
	if New().Detect(dir) {
		t.Fatal("did not expect detect on empty dir")
	}
	writeFile(t, filepath.Join(dir, "package.json"), "{}")
	if !New().Detect(dir) {
		t.Fatal("expected detect with package.json")
	}
}

func TestName(t *testing.T) {
	if n := New().Name(); n != "nodetest" {
		t.Errorf("name=%q want nodetest", n)
	}
}
