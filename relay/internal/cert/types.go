package cert

// Package cert holds the cross-stage result types of the certification
// pipeline. Stage runners and gates write into these structs; the engine
// folds them into the final core.Certificate / core.PolicyResult outputs.

import "time"

// TestStatus is the normalized outcome of a single test case.
type TestStatus string

const (
	// TestPassed indicates the test succeeded.
	TestPassed TestStatus = "passed"
	// TestFailed indicates the test failed (assertion, panic, or build error).
	TestFailed TestStatus = "failed"
	// TestSkipped indicates the test was skipped at runtime.
	TestSkipped TestStatus = "skipped"
)

// TestCase is a single test result, normalized across languages.
type TestCase struct {
	Package  string        `json:"package"`
	Name     string        `json:"name"`
	Status   TestStatus    `json:"status"`
	Duration time.Duration `json:"duration_ns"`
	// Output is the captured stdout/stderr for the case (truncated by runners).
	Output string `json:"output,omitempty"`
}

// TestRun is the normalized result of executing a project's test suite.
// Runners (Go, Node, Python, ...) produce this struct so downstream gates
// and persistence are language-agnostic.
type TestRun struct {
	Runner      string        `json:"runner"` // e.g. "gotest"
	StartedAt   time.Time     `json:"started_at"`
	Duration    time.Duration `json:"duration_ns"`
	Passed      int           `json:"passed"`
	Failed      int           `json:"failed"`
	Skipped     int           `json:"skipped"`
	CoveragePct float64       `json:"coverage_pct"` // 0..100
	Cases       []TestCase    `json:"cases,omitempty"`
	// Raw stdout/stderr from the test command, truncated to a sane bound.
	RawOutput string `json:"raw_output,omitempty"`
}

// Ok reports whether the run had zero failures and at least one passed test.
// A run with zero cases (no tests) returns false: we require evidence that
// something executed.
func (r TestRun) Ok() bool {
	return r.Failed == 0 && r.Passed > 0
}

// Stage1Result is the output of the build + test stage.
type Stage1Result struct {
	// Skipped is set when no applicable runner matched the project (e.g. no
	// go.mod for the Go runner). SkipReason explains why. A skipped result
	// is treated as non-blocking by the engine.
	Skipped       bool          `json:"skipped,omitempty"`
	SkipReason    string        `json:"skip_reason,omitempty"`
	BuildOk       bool          `json:"build_ok"`
	BuildOutput   string        `json:"build_output,omitempty"`
	BuildDuration time.Duration `json:"build_duration_ns"`
	Tests         TestRun       `json:"tests"`
}

// Ok reports whether both build and tests succeeded, or the stage was skipped
// for a structural reason (no applicable runner).
func (s Stage1Result) Ok() bool {
	if s.Skipped {
		return true
	}
	return s.BuildOk && s.Tests.Ok()
}
