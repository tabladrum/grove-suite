// Package stacks ships embedded `.provasign/` templates for common project
// shapes. `provasign init --stack=<name>` extracts the named tree into the
// caller's directory.
//
// Templates live under `templates/<name>/` next to this file and are
// compiled in via go:embed; no runtime asset fetching.
package stacks

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed all:templates
var templatesFS embed.FS

// Stack is a named template plus a short description for `--help`.
type Stack struct {
	Name        string
	Description string
}

// Known returns the list of available stacks, derived from the embedded FS.
func Known() ([]Stack, error) {
	entries, err := templatesFS.ReadDir("templates")
	if err != nil {
		return nil, err
	}
	var out []Stack
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		desc := descriptionOf(e.Name())
		out = append(out, Stack{Name: e.Name(), Description: desc})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// IsKnown reports whether name names an embedded stack.
func IsKnown(name string) bool {
	known, err := Known()
	if err != nil {
		return false
	}
	for _, s := range known {
		if s.Name == name {
			return true
		}
	}
	return false
}

// Apply extracts the named stack into destDir. Existing files are NOT
// overwritten: their relative paths are returned in the `skipped` slice so
// the caller can report them. Newly written files are returned in `written`.
func Apply(name, destDir string) (written, skipped []string, err error) {
	if !IsKnown(name) {
		return nil, nil, fmt.Errorf("stacks: unknown stack %q (run `provasign init --list-stacks`)", name)
	}
	root := "templates/" + name
	walkErr := fs.WalkDir(templatesFS, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == root {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		target := filepath.Join(destDir, filepath.FromSlash(rel))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if _, err := os.Stat(target); err == nil {
			skipped = append(skipped, rel)
			return nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
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

// descriptionOf returns a short human description for the named stack.
// Hard-coded for now (kept here, not in YAML, to avoid an extra read for the
// common `--help` path); add a line per new stack template.
func descriptionOf(name string) string {
	switch name {
	case "go-microservice":
		return "Go microservice with full Stage-1/Stage-2 defaults"
	case "node-api":
		return "Node.js API service (express-style); secrets+SAST+deps gates"
	case "python-service":
		return "Python service (FastAPI/Flask); secrets+SAST gates"
	case "java-spring":
		return "Java Spring Boot service; secrets+SAST gates"
	default:
		return strings.ReplaceAll(name, "-", " ")
	}
}
