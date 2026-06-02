// Package profiles ships embedded compliance / stack-specific `.provasign/`
// profile templates. Profiles are a thin wrapper around the same
// extraction mechanism `stacks` uses, but they target compliance
// regimes (SOC2 baseline, PCI-DSS baseline, ...) and "strict" variants
// of the four shipped stacks. Wiring `--profile=<name>` to `provasign init`
// is what turns this package into the L6 "policy marketplace bootstrap"
// deliverable.
package profiles

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

//go:embed all:templates
var templatesFS embed.FS

// Profile is one named template.
type Profile struct {
	Name        string
	Description string
}

// Known returns every embedded profile, sorted by name.
func Known() ([]Profile, error) {
	entries, err := templatesFS.ReadDir("templates")
	if err != nil {
		return nil, err
	}
	out := make([]Profile, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		out = append(out, Profile{Name: e.Name(), Description: descriptionOf(e.Name())})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// IsKnown reports whether name matches a shipped profile.
func IsKnown(name string) bool {
	known, err := Known()
	if err != nil {
		return false
	}
	for _, p := range known {
		if p.Name == name {
			return true
		}
	}
	return false
}

// Apply extracts the named profile into destDir using the same "never
// overwrite" semantics as stacks.Apply.
func Apply(name, destDir string) (written, skipped []string, err error) {
	if !IsKnown(name) {
		return nil, nil, fmt.Errorf("profiles: unknown profile %q (run `provasign init --list-profiles`)", name)
	}
	root := "templates/" + name
	walkErr := fs.WalkDir(templatesFS, root, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if p == root {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		target := filepath.Join(destDir, filepath.FromSlash(rel))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if _, statErr := os.Stat(target); statErr == nil {
			skipped = append(skipped, rel)
			return nil
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return statErr
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		data, err := templatesFS.ReadFile(p)
		if err != nil {
			return err
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return err
		}
		written = append(written, rel)
		return nil
	})
	if walkErr != nil {
		return written, skipped, walkErr
	}
	sort.Strings(written)
	sort.Strings(skipped)
	return written, skipped, nil
}

// descriptionOf is the short human label used by `provasign init --list-profiles`.
func descriptionOf(name string) string {
	switch name {
	case "soc2-baseline":
		return "SOC2 baseline: secrets/SAST required, coverage>=70%, signed certs"
	case "pci-dss-baseline":
		return "PCI-DSS baseline: strict secrets, coverage>=80%, fileclass deny on PAN paths"
	case "go-microservice-strict":
		return "Go microservice with hardened gates: coverage>=85%, deny critical CVEs"
	case "node-api-strict":
		return "Node API with hardened gates: coverage>=80%, deny critical CVEs"
	case "python-service-strict":
		return "Python service with hardened gates: coverage>=80%, deny critical CVEs"
	case "java-spring-strict":
		return "Java Spring service with hardened gates: coverage>=80%, deny critical CVEs"
	default:
		return ""
	}
}
