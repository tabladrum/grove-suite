package tools

import "testing"

func TestKnownTools_NonEmpty(t *testing.T) {
	if len(KnownTools) == 0 {
		t.Fatal("expected KnownTools to be populated")
	}
	for _, k := range []string{"gitleaks", "semgrep", "govulncheck", "golangci-lint"} {
		if !IsKnown(k) {
			t.Errorf("expected %q to be a known tool", k)
		}
	}
}

func TestReleaseFor_UnknownReturnsError(t *testing.T) {
	if _, err := ReleaseFor("not-a-tool"); err == nil {
		t.Fatal("expected error for unknown tool")
	}
}

func TestReleaseForPlatform_FallsBackToAny(t *testing.T) {
	// govulncheck is registered as OS=any Arch=any (go install).
	r, err := ReleaseForPlatform("govulncheck", "freebsd", "riscv64")
	if err != nil {
		t.Fatalf("expected govulncheck to resolve via OS=any, got %v", err)
	}
	if r.Name != "govulncheck" {
		t.Errorf("unexpected name %q", r.Name)
	}
}

func TestReleaseForPlatform_BinaryToolPlatformSpecific(t *testing.T) {
	r, err := ReleaseForPlatform("gitleaks", "linux", "amd64")
	if err != nil {
		t.Fatalf("expected gitleaks linux/amd64, got %v", err)
	}
	if r.URL == "" {
		t.Error("expected URL to be set")
	}
}
