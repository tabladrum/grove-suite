// Package nodetest is the Node.js test runner. It detects projects by
// looking for package.json, then invokes either jest with the
// jest-junit reporter or vitest with --reporter=junit, and returns a
// normalized cert.TestRun parsed from the JUnit output.
package nodetest

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/tabladrum/grove-suite/relay/internal/cert"
	"github.com/tabladrum/grove-suite/relay/internal/runner/junit"
)

// Runner is the Node.js adapter for Stage 1.
type Runner struct {
	// Cmd overrides the executable. Empty falls back to autodetection.
	Cmd string
}

// New returns a default Runner.
func New() *Runner { return &Runner{} }

// Name identifies this runner.
func (r *Runner) Name() string { return "nodetest" }

// Detect returns true if dir has a package.json.
func (r *Runner) Detect(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "package.json"))
	return err == nil
}

type packageJSON struct {
	Scripts         map[string]string `json:"scripts"`
	DevDependencies map[string]string `json:"devDependencies"`
	Dependencies    map[string]string `json:"dependencies"`
}

// Run executes the project's test command in dir and returns a normalized TestRun.
func (r *Runner) Run(ctx context.Context, dir string) (cert.TestRun, error) {
	reportPath := filepath.Join(dir, ".relay-node-junit.xml")
	defer os.Remove(reportPath)

	argv, err := r.resolveCmd(dir, reportPath)
	if err != nil {
		return cert.TestRun{Runner: r.Name(), StartedAt: time.Now().UTC()}, err
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	// jest-junit takes its output path from an env var.
	cmd.Env = append(os.Environ(), "JEST_JUNIT_OUTPUT_FILE="+reportPath)

	start := time.Now()
	out, runErr := cmd.CombinedOutput()
	_ = runErr // non-zero on failures; junit report is still authoritative

	tr := cert.TestRun{Runner: r.Name(), StartedAt: start.UTC()}
	if _, statErr := os.Stat(reportPath); statErr != nil {
		tr.Duration = time.Since(start)
		tr.RawOutput = truncate(string(out), 64<<10)
		return tr, nil
	}
	parsed, perr := junit.ParseFile(reportPath, r.Name())
	if perr != nil {
		tr.Duration = time.Since(start)
		tr.RawOutput = truncate(string(out), 64<<10)
		return tr, fmt.Errorf("nodetest: parse junit: %w", perr)
	}
	parsed.StartedAt = start.UTC()
	parsed.Duration = time.Since(start)
	parsed.RawOutput = truncate(string(out), 64<<10)
	return parsed, nil
}

// resolveCmd inspects package.json and picks a sensible command. It
// prefers `npx jest` (or `npx vitest`) so the project's local
// dependency wins; falls back to invoking the global binary; falls
// back to `npm test`.
func (r *Runner) resolveCmd(dir, report string) ([]string, error) {
	if r.Cmd != "" {
		return strings.Fields(r.Cmd), nil
	}
	body, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return nil, fmt.Errorf("nodetest: read package.json: %w", err)
	}
	var pj packageJSON
	if err := json.Unmarshal(body, &pj); err != nil {
		return nil, fmt.Errorf("nodetest: parse package.json: %w", err)
	}
	deps := map[string]string{}
	for k, v := range pj.DevDependencies {
		deps[k] = v
	}
	for k, v := range pj.Dependencies {
		deps[k] = v
	}

	npx, _ := exec.LookPath("npx")
	switch {
	case deps["vitest"] != "" && npx != "":
		return []string{npx, "vitest", "run", "--reporter=junit", "--outputFile=" + report}, nil
	case deps["jest"] != "" && npx != "":
		return []string{npx, "jest", "--reporters=default", "--reporters=jest-junit"}, nil
	}
	if pj.Scripts["test"] != "" {
		npm, err := exec.LookPath("npm")
		if err != nil {
			return nil, fmt.Errorf("nodetest: package.json declares a test script but npm is not on PATH")
		}
		return []string{npm, "test", "--silent"}, nil
	}
	return nil, fmt.Errorf("nodetest: no recognized test setup (jest/vitest/npm test) in package.json")
}

func truncate(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "\n... [truncated]"
}
