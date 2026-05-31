package analysis

import (
	"testing"

	"github.com/tabladrum/grove-suite/relay/internal/cert"
	"github.com/tabladrum/grove-suite/relay/internal/core"
)

func mkFinding(sev cert.Severity, path string, line int) cert.Finding {
	return cert.Finding{Severity: sev, Path: path, Line: line}
}

func TestFilterFloor_DropsBelow(t *testing.T) {
	in := []cert.Finding{
		mkFinding(cert.SeverityInfo, "a.go", 1),
		mkFinding(cert.SeverityMedium, "b.go", 2),
		mkFinding(cert.SeverityCritical, "c.go", 3),
	}
	got := filterFloor(in, cert.SeverityMedium)
	if len(got) != 2 || got[0].Path != "b.go" || got[1].Path != "c.go" {
		t.Fatalf("unexpected: %#v", got)
	}
}

func TestFilterFloor_NoFloorReturnsAll(t *testing.T) {
	in := []cert.Finding{mkFinding(cert.SeverityInfo, "a.go", 1)}
	got := filterFloor(in, "")
	if len(got) != 1 {
		t.Fatalf("expected passthrough, got %#v", got)
	}
}

func TestFilterNewCode(t *testing.T) {
	added := core.AddedLineMap{
		"touched.go": {{Start: 10, End: 20}},
	}
	in := []cert.Finding{
		mkFinding(cert.SeverityHigh, "touched.go", 15), // in scope
		mkFinding(cert.SeverityHigh, "touched.go", 25), // out of scope
		mkFinding(cert.SeverityHigh, "other.go", 1),    // file untouched
		mkFinding(cert.SeverityHigh, "", 0),            // no path → infra error, passthrough
	}
	got := filterNewCode(in, added)
	if len(got) != 2 {
		t.Fatalf("expected 2 (in-scope + no-path), got %d: %#v", len(got), got)
	}
	if got[0].Path != "touched.go" || got[0].Line != 15 {
		t.Errorf("unexpected first: %#v", got[0])
	}
	if got[1].Path != "" {
		t.Errorf("expected passthrough infra finding, got %#v", got[1])
	}
}
