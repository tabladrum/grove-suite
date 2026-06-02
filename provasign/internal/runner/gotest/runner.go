package gotest

// Package gotest is the Go test runner. It executes `go test -json -cover`
// in a target directory and normalizes the streaming JSON event output into
// a cert.TestRun.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/provasign/provasign/internal/cert"
)

func osStatFile(dir, name string) (os.FileInfo, error) {
	return os.Stat(filepath.Join(dir, name))
}

// Runner runs `go test` and produces a normalized TestRun.
type Runner struct {
	// Packages is the go test target. Defaults to "./...".
	Packages string
	// Timeout caps a single test (passed via -timeout). Zero means no override.
	Timeout time.Duration
	// MaxOutputBytes truncates per-case Output and TestRun.RawOutput. Zero
	// disables truncation (not recommended).
	MaxOutputBytes int
}

// New returns a Runner with default settings.
func New() *Runner {
	return &Runner{
		Packages:       "./...",
		MaxOutputBytes: 64 << 10, // 64 KiB
	}
}

// Detect returns true if dir looks like a Go module (has go.mod at root).
// Implements the multilang dispatch contract.
func (r *Runner) Detect(dir string) bool {
	_, err := osStatFile(dir, "go.mod")
	return err == nil
}

// Name identifies this runner in TestRun.Runner output and logs.
func (r *Runner) Name() string { return "gotest" }

// goTestEvent matches the schema of `go test -json` events.
//
// Action values used: run, pass, fail, skip, output, start.
type goTestEvent struct {
	Time    time.Time `json:"Time"`
	Action  string    `json:"Action"`
	Package string    `json:"Package"`
	Test    string    `json:"Test"`
	Elapsed float64   `json:"Elapsed"` // seconds
	Output  string    `json:"Output"`
}

// Run executes the test suite in dir and returns a normalized TestRun.
// A non-nil error indicates the command could not be executed at all; a
// failed test suite is reported as TestRun.Failed > 0 with err == nil.
func (r *Runner) Run(ctx context.Context, dir string) (cert.TestRun, error) {
	pkgs := r.Packages
	if pkgs == "" {
		pkgs = "./..."
	}
	args := []string{"test", "-json", "-cover", pkgs}
	if r.Timeout > 0 {
		args = append(args, "-timeout", r.Timeout.String())
	}
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	start := time.Now()
	runErr := cmd.Run()
	dur := time.Since(start)

	run := cert.TestRun{
		Runner:    "gotest",
		StartedAt: start.UTC(),
		Duration:  dur,
	}

	// Output of `go test -json` is one JSON object per line on stdout.
	// Stderr typically contains tool-level errors (e.g. build failures).
	// We must snapshot stdout BEFORE scanning, because bufio.Scanner reads
	// (and consumes) from the same bytes.Buffer.
	rawStdout := stdout.String()
	caseOutput := map[string]*strings.Builder{}
	caseStatus := map[string]*cert.TestCase{}

	scanner := bufio.NewScanner(strings.NewReader(rawStdout))
	scanner.Buffer(make([]byte, 1<<20), 8<<20)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		var ev goTestEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		switch ev.Action {
		case "output":
			if ev.Test == "" {
				continue
			}
			key := ev.Package + "::" + ev.Test
			b := caseOutput[key]
			if b == nil {
				b = &strings.Builder{}
				caseOutput[key] = b
			}
			if r.MaxOutputBytes == 0 || b.Len() < r.MaxOutputBytes {
				b.WriteString(ev.Output)
			}
		case "pass", "fail", "skip":
			if ev.Test == "" {
				continue
			}
			key := ev.Package + "::" + ev.Test
			tc := &cert.TestCase{
				Package:  ev.Package,
				Name:     ev.Test,
				Duration: time.Duration(ev.Elapsed * float64(time.Second)),
			}
			switch ev.Action {
			case "pass":
				tc.Status = cert.TestPassed
				run.Passed++
			case "fail":
				tc.Status = cert.TestFailed
				run.Failed++
			case "skip":
				tc.Status = cert.TestSkipped
				run.Skipped++
			}
			caseStatus[key] = tc
		}
	}

	for key, tc := range caseStatus {
		if b, ok := caseOutput[key]; ok {
			tc.Output = truncate(b.String(), r.MaxOutputBytes)
		}
		run.Cases = append(run.Cases, *tc)
	}

	run.CoveragePct = parseAggregateCoverage(rawStdout)
	run.RawOutput = truncate(stderr.String(), r.MaxOutputBytes)

	// If the test command itself failed (build error, exit !=0 with no parsed
	// failures), record at least one synthetic failure so Ok() reports false.
	if runErr != nil && run.Failed == 0 && run.Passed == 0 {
		run.Failed = 1
		run.Cases = append(run.Cases, cert.TestCase{
			Package: pkgs,
			Name:    "go_test",
			Status:  cert.TestFailed,
			Output:  truncate(fmt.Sprintf("go test failed: %v\n%s", runErr, stderr.String()), r.MaxOutputBytes),
		})
	}
	return run, nil
}

var coverageLineRE = regexp.MustCompile(`coverage:\s+([0-9.]+)%\s+of\s+statements`)

// parseAggregateCoverage averages the per-package coverage lines emitted by
// `go test -cover`. Returns 0 if no coverage lines were seen.
func parseAggregateCoverage(out string) float64 {
	var total float64
	var n int
	matches := coverageLineRE.FindAllStringSubmatch(out, -1)
	for _, m := range matches {
		v, err := strconv.ParseFloat(m[1], 64)
		if err != nil {
			continue
		}
		total += v
		n++
	}
	if n == 0 {
		return 0
	}
	return total / float64(n)
}

func truncate(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "\n...(truncated)"
}
