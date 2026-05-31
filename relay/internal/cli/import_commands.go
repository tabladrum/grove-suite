package cli

// `relay import <sub>` — import third-party rule configurations.

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/tabladrum/grove-suite/relay/internal/sonarqube"
)

// RunImport dispatches `relay import <sub>`.
func RunImport(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: relay import <sonarqube-profile> [args]")
		return 1
	}
	switch args[0] {
	case "sonarqube-profile":
		return cmdImportSonarQube(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown import subcommand: %s\n", args[0])
		return 1
	}
}

// cmdImportSonarQube implements `relay import sonarqube-profile <path.xml>`.
//
// The Phase 2A surface preserves the SonarQube quality-profile XML verbatim
// at .relay/rulesets/<name>.xml so the Phase 2B SonarLint Core engine can
// consume it unchanged (see docs/sonarqube-no-server-investigation.md). The
// older lossy SQ-key → semgrep-rule-ID mapping is no longer the default; pass
// --legacy-mapping to fall back to it for migrations from the pre-audit flow.
func cmdImportSonarQube(args []string) int {
	fs := flag.NewFlagSet("import sonarqube-profile", flag.ContinueOnError)
	outDir := fs.String("out", ".relay/rulesets", "output directory for the imported XML")
	name := fs.String("name", "", "destination filename stem (defaults to slugified profile name)")
	quiet := fs.Bool("quiet", false, "suppress informational output")
	legacy := fs.Bool("legacy-mapping", false, "use the pre-audit lossy SQ→semgrep mapping (deprecated)")
	withSonar := fs.Bool("with-sonar-installed", false, "skip the install hint (declare the engine is available)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: relay import sonarqube-profile [--out=dir] [--name=stem] [--legacy-mapping] <path.xml>")
		return 1
	}
	src := fs.Arg(0)

	if *legacy {
		// Legacy path retained for back-compat; emit a one-line deprecation notice.
		fmt.Fprintln(os.Stderr, "warning: --legacy-mapping is deprecated; preserves the lossy SQ-key → semgrep mapping. See docs/sonarqube-no-server-investigation.md.")
		profile, err := sonarqube.ParseFile(src)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		rs := sonarqube.Import(profile, src)
		if err := writeRuleset(rs, *outDir); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if !*quiet {
			printImportSummary(os.Stdout, rs)
		}
		return 0
	}

	// Verbatim path (the Phase 2A default).
	profile, err := sonarqube.ParseFile(src)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	stem := *name
	if stem == "" {
		stem = sanitize(profile.Name)
	}
	if stem == "" {
		stem = "profile"
	}
	destPath := filepath.Join(*outDir, stem+".xml")
	summary, err := sonarqube.ImportVerbatim(src, destPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if !*quiet {
		fmt.Printf("imported sonarqube profile %q (lang=%s, %d rules)\n",
			summary.Name, summary.Language, summary.RuleCount)
		fmt.Printf("  written verbatim to %s (travels with the repo)\n", summary.DestinationPath)
		// Print the install hint unless the caller declared --with-sonar-installed.
		// The engine integration ships in Phase 2B; until then the file is durable
		// and the importer is enough for the audit/admission story.
		if !*withSonar {
			fmt.Println()
			fmt.Println("next: run `relay tools install --with-sonar` (Phase 2B) to enable")
			fmt.Println("      evaluation of this profile against your code at relay_check time.")
		}
		fmt.Println()
		fmt.Println("Add this to .relay/relay.yaml so it's picked up on every invocation:")
		fmt.Println("  sonar:")
		fmt.Printf("    profile: rulesets/%s.xml\n", stem)
	}
	return 0
}

func writeRuleset(rs *sonarqube.Ruleset, outDir string) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	dst := filepath.Join(outDir, sanitize(rs.Name)+".json")
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(rs)
}

func printImportSummary(w io.Writer, rs *sonarqube.Ruleset) {
	fmt.Fprintf(w, "imported sonarqube profile: %s (lang=%s)\n", rs.Name, rs.Language)
	fmt.Fprintf(w, "  mapped: %d / %d (%.0f%%)\n",
		rs.Coverage.Mapped, rs.Coverage.Total, rs.Coverage.Ratio*100)
	if len(rs.Gaps) > 0 {
		fmt.Fprintf(w, "  gaps (%d):\n", len(rs.Gaps))
		for _, g := range rs.Gaps {
			fmt.Fprintf(w, "    - %s:%s (priority=%s)\n", g.RepositoryKey, g.Key, g.Priority)
		}
	}
}

func sanitize(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			out = append(out, r)
		case r == ' ':
			out = append(out, '-')
		}
	}
	if len(out) == 0 {
		return "profile"
	}
	return string(out)
}
