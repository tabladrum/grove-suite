package stacks

import (
	"os"
	"path/filepath"
	"testing"
)

func TestKnown_ContainsExpectedStacks(t *testing.T) {
	known, err := Known()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"go-microservice": false,
		"node-api":        false,
		"python-service":  false,
		"java-spring":     false,
	}
	for _, s := range known {
		if _, ok := want[s.Name]; ok {
			want[s.Name] = true
		}
		if s.Description == "" {
			t.Errorf("stack %q missing description", s.Name)
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("expected stack %q in Known()", name)
		}
	}
}

func TestApply_WritesYAML(t *testing.T) {
	dest := t.TempDir()
	written, skipped, err := Apply("go-microservice", dest)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(written) == 0 {
		t.Fatal("expected files to be written")
	}
	if len(skipped) != 0 {
		t.Fatalf("expected no skipped files in empty dest, got %v", skipped)
	}
	p := filepath.Join(dest, "relay.yaml")
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if len(data) == 0 {
		t.Error("written file is empty")
	}
}

func TestApply_SkipsExistingFiles(t *testing.T) {
	dest := t.TempDir()
	p := filepath.Join(dest, "relay.yaml")
	if err := os.WriteFile(p, []byte("preserved"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, skipped, err := Apply("go-microservice", dest)
	if err != nil {
		t.Fatal(err)
	}
	if len(skipped) == 0 {
		t.Error("expected relay.yaml to be skipped")
	}
	got, _ := os.ReadFile(p)
	if string(got) != "preserved" {
		t.Errorf("existing file overwritten: got %q", got)
	}
}

func TestApply_UnknownStackErrors(t *testing.T) {
	if _, _, err := Apply("definitely-not-a-stack", t.TempDir()); err == nil {
		t.Fatal("expected error for unknown stack")
	}
}
