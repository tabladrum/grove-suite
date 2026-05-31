package pytest

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

func TestDetect_Markers(t *testing.T) {
	markers := []string{"pyproject.toml", "setup.py", "setup.cfg", "pytest.ini", "tox.ini"}
	for _, m := range markers {
		t.Run(m, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, filepath.Join(dir, m), "")
			if !New().Detect(dir) {
				t.Fatalf("expected detect for %s", m)
			}
		})
	}
}

func TestDetect_LoosePyFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "sample.py"), "print('ok')\n")
	if !New().Detect(dir) {
		t.Fatal("expected detect when *.py file is present")
	}
}

func TestDetect_None(t *testing.T) {
	dir := t.TempDir()
	if New().Detect(dir) {
		t.Fatal("did not expect detect on empty dir")
	}
}

func TestName(t *testing.T) {
	if n := New().Name(); n != "pytest" {
		t.Errorf("name=%q want pytest", n)
	}
}
