// Package sonarqube imports SonarQube quality profile XMLs into Relay
// rulesets.
//
// The XML schema we accept is the standard SonarQube quality-profile export:
//
//   <profile>
//     <name>Sonar way</name>
//     <language>java</language>
//     <rules>
//       <rule>
//         <repositoryKey>java</repositoryKey>
//         <key>S2068</key>
//         <priority>BLOCKER</priority>
//       </rule>
//       ...
//     </rules>
//   </profile>
//
// Mapping table (see mapping.go) translates a subset of SonarQube rule keys
// into Relay-native gate identifiers (`secrets.<id>`, `fileclass.<id>`,
// `semgrep:<config>`, etc.). Unmapped rules are returned in the Gap report so
// adopters can see exactly what's missing.
package sonarqube

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

// Profile is the parsed SonarQube quality profile.
type Profile struct {
	XMLName  xml.Name `xml:"profile"`
	Name     string   `xml:"name"`
	Language string   `xml:"language"`
	Rules    struct {
		Rules []ProfileRule `xml:"rule"`
	} `xml:"rules"`
}

// ProfileRule is a single rule entry in the profile.
type ProfileRule struct {
	RepositoryKey string `xml:"repositoryKey"`
	Key           string `xml:"key"`
	Priority      string `xml:"priority"`
}

// Ruleset is the Relay-side import result.
type Ruleset struct {
	Source   string         `json:"source"`              // path of the imported XML
	Name     string         `json:"name"`                // profile name
	Language string         `json:"language"`            // profile language
	Mapped   []MappedRule   `json:"mapped"`              // sonar rule → relay gate
	Gaps     []ProfileRule  `json:"gaps"`                // unmapped rules
	Coverage CoverageReport `json:"coverage"`            // mapped/total ratio
}

// MappedRule is one rule with its Relay-side translation.
type MappedRule struct {
	Sonar ProfileRule `json:"sonar"`
	Relay string      `json:"relay"`
}

// CoverageReport summarises the import.
type CoverageReport struct {
	Total  int     `json:"total"`
	Mapped int     `json:"mapped"`
	Ratio  float64 `json:"ratio"`
}

// Parse parses a SonarQube profile XML from r.
func Parse(r io.Reader) (*Profile, error) {
	dec := xml.NewDecoder(r)
	dec.Strict = false
	var p Profile
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("sonarqube: parse: %w", err)
	}
	if p.Name == "" {
		return nil, fmt.Errorf("sonarqube: profile missing <name>")
	}
	return &p, nil
}

// ParseFile parses a SonarQube profile XML from path.
func ParseFile(path string) (*Profile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Parse(f)
}

// Import applies the mapping table and returns a Ruleset describing the
// translated profile plus any unmapped rules.
func Import(p *Profile, source string) *Ruleset {
	rs := &Ruleset{
		Source:   source,
		Name:     p.Name,
		Language: p.Language,
	}
	for _, r := range p.Rules.Rules {
		k := lookupKey(r)
		if relay, ok := Mapping[k]; ok {
			rs.Mapped = append(rs.Mapped, MappedRule{Sonar: r, Relay: relay})
			continue
		}
		rs.Gaps = append(rs.Gaps, r)
	}
	sort.Slice(rs.Mapped, func(i, j int) bool {
		return lookupKey(rs.Mapped[i].Sonar) < lookupKey(rs.Mapped[j].Sonar)
	})
	sort.Slice(rs.Gaps, func(i, j int) bool {
		return lookupKey(rs.Gaps[i]) < lookupKey(rs.Gaps[j])
	})
	total := len(rs.Mapped) + len(rs.Gaps)
	rs.Coverage.Total = total
	rs.Coverage.Mapped = len(rs.Mapped)
	if total > 0 {
		rs.Coverage.Ratio = float64(len(rs.Mapped)) / float64(total)
	}
	return rs
}

// lookupKey is the canonical "repo:key" form used by Mapping.
func lookupKey(r ProfileRule) string {
	if r.RepositoryKey == "" {
		return r.Key
	}
	return r.RepositoryKey + ":" + r.Key
}

// VerbatimSummary describes the result of a verbatim XML import (the
// Phase 2A surface). The XML payload is preserved byte-for-byte at
// DestinationPath; only a validation parse runs in-process. The actual
// rule evaluation happens at Stage 2 via the SonarLint Core engine, which
// reads the same XML directly (Phase 2B).
type VerbatimSummary struct {
	SourcePath      string `json:"source_path"`
	DestinationPath string `json:"destination_path"`
	Name            string `json:"profile_name"`
	Language        string `json:"language"`
	RuleCount       int    `json:"rule_count"`
}

// ImportVerbatim is the Phase 2A SonarQube import path. It validates the XML
// is a recognized SonarQube profile backup, writes the XML *verbatim* to
// destPath (typically `.relay/rulesets/<name>.xml` in the repo), and returns
// a summary the CLI uses to print a "next step" hint.
//
// Verbatim preservation is the contract that lets the Phase 2B SonarLint
// Core engine consume the same XML SonarQube Server itself produces, without
// Relay imposing a lossy SQ-key → semgrep mapping in between.
func ImportVerbatim(srcPath, destPath string) (*VerbatimSummary, error) {
	raw, err := os.ReadFile(srcPath)
	if err != nil {
		return nil, fmt.Errorf("read source xml: %w", err)
	}
	// Validate parse so we fail before writing junk into .relay/rulesets/.
	p, err := Parse(stringsReader(raw))
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepathDir(destPath), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir dest: %w", err)
	}
	if err := os.WriteFile(destPath, raw, 0o644); err != nil {
		return nil, fmt.Errorf("write verbatim xml: %w", err)
	}
	return &VerbatimSummary{
		SourcePath:      srcPath,
		DestinationPath: destPath,
		Name:            p.Name,
		Language:        p.Language,
		RuleCount:       len(p.Rules.Rules),
	}, nil
}

func stringsReader(b []byte) io.Reader { return bytes.NewReader(b) }
func filepathDir(p string) string      { return filepath.Dir(p) }
