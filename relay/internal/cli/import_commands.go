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

func cmdImportSonarQube(args []string) int {
	fs := flag.NewFlagSet("import sonarqube-profile", flag.ContinueOnError)
	out := fs.String("out", ".relay/rulesets", "output directory for the imported ruleset")
	quiet := fs.Bool("quiet", false, "suppress per-rule logging")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: relay import sonarqube-profile [--out=dir] <path.xml>")
		return 1
	}
	src := fs.Arg(0)
	profile, err := sonarqube.ParseFile(src)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	rs := sonarqube.Import(profile, src)
	if err := writeRuleset(rs, *out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if !*quiet {
		printImportSummary(os.Stdout, rs)
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
