package sonarqube

import (
	"os"
	"path/filepath"
	"testing"
)

// fixtureProfile is the minimal valid SonarQube quality-profile XML used to
// exercise the verbatim import path. The structure matches the export
// produced by `GET /api/qualityprofiles/backup` on a real SonarQube server.
const fixtureProfile = `<?xml version="1.0" encoding="UTF-8"?>
<profile>
  <name>Acme Java</name>
  <language>java</language>
  <rules>
    <rule>
      <repositoryKey>java</repositoryKey>
      <key>S1764</key>
      <priority>MAJOR</priority>
    </rule>
    <rule>
      <repositoryKey>java</repositoryKey>
      <key>S2068</key>
      <priority>BLOCKER</priority>
    </rule>
  </rules>
</profile>
`

// TestImportVerbatim_PreservesBytes is the core contract: the XML lands at
// destPath byte-for-byte identical to the source, so the Phase 2B SonarLint
// Core engine can consume it directly without Relay imposing a lossy mapping.
func TestImportVerbatim_PreservesBytes(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "acme-java.xml")
	if err := os.WriteFile(src, []byte(fixtureProfile), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, ".provasign", "rulesets", "acme-java.xml")
	summary, err := ImportVerbatim(src, dest)
	if err != nil {
		t.Fatalf("ImportVerbatim: %v", err)
	}
	if summary.Name != "Acme Java" {
		t.Errorf("Name = %q, want %q", summary.Name, "Acme Java")
	}
	if summary.Language != "java" {
		t.Errorf("Language = %q", summary.Language)
	}
	if summary.RuleCount != 2 {
		t.Errorf("RuleCount = %d, want 2", summary.RuleCount)
	}
	written, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if string(written) != fixtureProfile {
		t.Errorf("dest content was not preserved byte-for-byte\n--- want ---\n%s\n--- got ---\n%s", fixtureProfile, written)
	}
}

func TestImportVerbatim_InvalidXMLRejected(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "bad.xml")
	if err := os.WriteFile(src, []byte("<not a valid profile>"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, ".provasign", "rulesets", "bad.xml")
	if _, err := ImportVerbatim(src, dest); err == nil {
		t.Errorf("expected error for invalid profile XML")
	}
	// Confirm dest was NOT created.
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Errorf("dest should not exist when parse fails")
	}
}

func TestImportVerbatim_MissingSource(t *testing.T) {
	dir := t.TempDir()
	if _, err := ImportVerbatim(filepath.Join(dir, "nope.xml"), filepath.Join(dir, "out.xml")); err == nil {
		t.Errorf("expected error for missing source file")
	}
}
