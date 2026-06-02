package profiles

import (
	"os"
	"path/filepath"
	"testing"
)

func TestKnown_IncludesShippedProfiles(t *testing.T) {
	got, err := Known()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"soc2-baseline":          true,
		"pci-dss-baseline":       true,
		"go-microservice-strict": true,
		"node-api-strict":        true,
		"python-service-strict":  true,
		"java-spring-strict":     true,
	}
	for _, p := range got {
		if p.Description == "" {
			t.Errorf("profile %q missing description", p.Name)
		}
		delete(want, p.Name)
	}
	if len(want) > 0 {
		t.Fatalf("missing profiles: %v", want)
	}
}

func TestIsKnown(t *testing.T) {
	if !IsKnown("soc2-baseline") {
		t.Fatal("soc2-baseline must be known")
	}
	if IsKnown("nope-not-real") {
		t.Fatal("bogus profile reported as known")
	}
}

func TestApply_WritesAndSkipsExisting(t *testing.T) {
	dir := t.TempDir()
	written, skipped, err := Apply("soc2-baseline", dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(written) == 0 {
		t.Fatal("expected at least one written file")
	}
	if len(skipped) != 0 {
		t.Fatalf("expected zero skipped on fresh dir, got %v", skipped)
	}
	// Verify the relay.yaml landed on disk and is non-empty.
	body, err := os.ReadFile(filepath.Join(dir, "provasign.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(body) == 0 {
		t.Fatal("relay.yaml is empty")
	}
	// Second apply must skip the now-existing file.
	written2, skipped2, err := Apply("soc2-baseline", dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(written2) != 0 {
		t.Fatalf("expected zero written on re-apply, got %v", written2)
	}
	if len(skipped2) == 0 {
		t.Fatal("expected at least one skipped on re-apply")
	}
}

func TestApply_UnknownProfile(t *testing.T) {
	_, _, err := Apply("totally-fake-profile", t.TempDir())
	if err == nil {
		t.Fatal("expected error for unknown profile")
	}
}

func TestDescriptionOf_Default(t *testing.T) {
	if descriptionOf("not-a-real-profile") != "" {
		t.Fatal("unknown profile should have empty description")
	}
}

func TestApply_AllShippedProfiles(t *testing.T) {
	names := []string{
		"soc2-baseline",
		"pci-dss-baseline",
		"go-microservice-strict",
		"node-api-strict",
		"python-service-strict",
		"java-spring-strict",
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			written, skipped, err := Apply(name, dir)
			if err != nil {
				t.Fatal(err)
			}
			if len(written) == 0 {
				t.Fatal("expected at least one written file")
			}
			if len(skipped) != 0 {
				t.Fatalf("unexpected skipped on fresh dir: %v", skipped)
			}
			if d := descriptionOf(name); d == "" {
				t.Fatalf("description for %s is empty", name)
			}
		})
	}
}
