package sonarqube

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "profile.xml")
	if err := os.WriteFile(p, []byte(sampleProfile), 0o644); err != nil {
		t.Fatal(err)
	}
	prof, err := ParseFile(p)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if prof.Name != "Sonar way" {
		t.Errorf("name %q", prof.Name)
	}
}

func TestParseFile_Missing(t *testing.T) {
	if _, err := ParseFile(filepath.Join(t.TempDir(), "nope.xml")); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestParse_MalformedXML(t *testing.T) {
	if _, err := Parse(strings.NewReader("<<<not xml")); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestLookupKey_NoRepository(t *testing.T) {
	got := lookupKey(ProfileRule{Key: "S0001"})
	if got != "S0001" {
		t.Errorf("got %q", got)
	}
}

func TestImport_EmptyProfile(t *testing.T) {
	rs := Import(&Profile{Name: "empty"}, "src")
	if rs.Coverage.Total != 0 || rs.Coverage.Ratio != 0 {
		t.Errorf("expected zero totals, got %+v", rs.Coverage)
	}
}
