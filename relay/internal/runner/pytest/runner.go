// Package pytest is the Python pytest runner. It detects Python projects
// by looking for pyproject.toml / setup.py / pytest.ini / setup.cfg,
// invokes pytest with the jest-junit-compatible junitxml reporter, and
// returns a normalized cert.TestRun parsed from the JUnit output.
package pytest

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/tabladrum/grove-suite/relay/internal/cert"
	"github.com/tabladrum/grove-suite/relay/internal/runner/junit"
)

// Runner is the pytest adapter for Stage 1.
type Runner struct {
	// Cmd overrides the executable. Empty string falls back to "pytest"
	// then "python3 -m pytest" then "python -m pytest".
	Cmd string
	// ExtraArgs are appended after the junit-xml report flag.
	ExtraArgs []string
}

// New returns a default Runner.
func New() *Runner { return &Runner{} }

// Name implements the multilang LangRunner contract.
func (r *Runner) Name() string { return "pytest" }

// Detect returns true if dir looks like a Python project.
func (r *Runner) Detect(dir string) bool {
	for _, name := range []string{"pyproject.toml", "setup.py", "setup.cfg", "pytest.ini", "tox.ini"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	// Fallback: any *.py file at the root.
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".py") {
			return true
		}
	}
	return false
}

// Run executes pytest in dir and returns a normalized TestRun.
func (r *Runner) Run(ctx context.Context, dir string) (cert.TestRun, error) {
	reportPath := filepath.Join(dir, ".relay-pytest-junit.xml")
	defer os.Remove(reportPath)

	cmdLine := r.resolveCmd()
	if len(cmdLine) == 0 {
		return cert.TestRun{
			Runner:    r.Name(),
			StartedAt: time.Now().UTC(),
		}, fmt.Errorf("pytest: no python/pytest executable found on PATH")
	}
	args := append([]string{}, cmdLine[1:]...)
	args = append(args, "--junitxml="+reportPath, "-q")
	args = append(args, r.ExtraArgs...)

	cmd := exec.CommandContext(ctx, cmdLine[0], args...)
	cmd.Dir = dir
	start := time.Now()
	out, runErr := cmd.CombinedOutput()
	_ = runErr // pytest exits non-zero on failures; we read the junit report regardless.

	tr := cert.TestRun{Runner: r.Name(), StartedAt: start.UTC()}
	if _, err := os.Stat(reportPath); err != nil {
		// pytest never wrote a report (collection error etc.).
		tr.Duration = time.Since(start)
		tr.RawOutput = truncate(string(out), 64<<10)
		return tr, nil
	}
	parsed, perr := junit.ParseFile(reportPath, r.Name())
	if perr != nil {
		tr.Duration = time.Since(start)
		tr.RawOutput = truncate(string(out), 64<<10)
		return tr, fmt.Errorf("pytest: parse junit: %w", perr)
	}
	parsed.StartedAt = start.UTC()
	parsed.Duration = time.Since(start)
	parsed.RawOutput = truncate(string(out), 64<<10)
	return parsed, nil
}

// resolveCmd returns the argv prefix used to invoke pytest. Empty slice
// means no usable executable.
func (r *Runner) resolveCmd() []string {
	if r.Cmd != "" {
		return strings.Fields(r.Cmd)
	}
	if p, err := exec.LookPath("pytest"); err == nil {
		return []string{p}
	}
	if p, err := exec.LookPath("python3"); err == nil {
		return []string{p, "-m", "pytest"}
	}
	if p, err := exec.LookPath("python"); err == nil {
		return []string{p, "-m", "pytest"}
	}
	return nil
}

func truncate(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "\n... [truncated]"
}
