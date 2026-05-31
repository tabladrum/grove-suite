package sonarqube

import (
	"bytes"
	"strings"
	"testing"
)

const sampleProfile = `<?xml version="1.0" encoding="UTF-8"?>
<profile>
  <name>Sonar way</name>
  <language>java</language>
  <rules>
    <rule>
      <repositoryKey>java</repositoryKey>
      <key>S2068</key>
      <priority>BLOCKER</priority>
    </rule>
    <rule>
      <repositoryKey>java</repositoryKey>
      <key>S2076</key>
      <priority>CRITICAL</priority>
    </rule>
    <rule>
      <repositoryKey>java</repositoryKey>
      <key>S4423</key>
      <priority>MAJOR</priority>
    </rule>
    <rule>
      <repositoryKey>java</repositoryKey>
      <key>S9999</key>
      <priority>MINOR</priority>
    </rule>
  </rules>
</profile>`

func TestParse_RoundTrip(t *testing.T) {
	p, err := Parse(strings.NewReader(sampleProfile))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if p.Name != "Sonar way" {
		t.Errorf("name = %q", p.Name)
	}
	if got := len(p.Rules.Rules); got != 4 {
		t.Errorf("rules = %d, want 4", got)
	}
}

func TestImport_MappingCoverage(t *testing.T) {
	p, err := Parse(bytes.NewReader([]byte(sampleProfile)))
	if err != nil {
		t.Fatal(err)
	}
	rs := Import(p, "test.xml")
	if rs.Coverage.Total != 4 {
		t.Errorf("total = %d", rs.Coverage.Total)
	}
	if rs.Coverage.Mapped != 3 {
		t.Errorf("mapped = %d, want 3", rs.Coverage.Mapped)
	}
	if rs.Coverage.Ratio < 0.7 {
		t.Errorf("coverage ratio %.2f below 0.7 floor", rs.Coverage.Ratio)
	}
	if len(rs.Gaps) != 1 || rs.Gaps[0].Key != "S9999" {
		t.Errorf("expected single gap S9999, got %+v", rs.Gaps)
	}
}

func TestParse_RequiresName(t *testing.T) {
	_, err := Parse(strings.NewReader(`<profile><rules></rules></profile>`))
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}
