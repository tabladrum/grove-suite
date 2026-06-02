package stacks

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApply_PreservesNestedDirsAndFiles(t *testing.T) {
	dest := t.TempDir()
	// Pre-create the target directory to exercise the MkdirAll-already-exists path.
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	written, _, err := Apply("node-api", dest)
	if err != nil {
		t.Fatal(err)
	}
	if len(written) == 0 {
		t.Fatal("expected at least one file")
	}
	for _, w := range written {
		if _, err := os.Stat(filepath.Join(dest, w)); err != nil {
			t.Errorf("written file missing: %s (%v)", w, err)
		}
	}
}

func TestApply_AllStacks(t *testing.T) {
	known, err := Known()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range known {
		s := s
		t.Run(s.Name, func(t *testing.T) {
			dest := t.TempDir()
			written, _, err := Apply(s.Name, dest)
			if err != nil {
				t.Fatalf("Apply(%s): %v", s.Name, err)
			}
			if len(written) == 0 {
				t.Errorf("stack %s produced no files", s.Name)
			}
		})
	}
}
