package cert

import (
	"testing"
	"time"
)

func TestTestRunOk(t *testing.T) {
	cases := []struct {
		name string
		run  TestRun
		want bool
	}{
		{"pass only", TestRun{Passed: 3}, true},
		{"pass with skip", TestRun{Passed: 1, Skipped: 2}, true},
		{"zero cases", TestRun{}, false},
		{"any failure", TestRun{Passed: 5, Failed: 1}, false},
		{"only skipped", TestRun{Skipped: 4}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.run.Ok(); got != tc.want {
				t.Errorf("Ok() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestBuildResultOk(t *testing.T) {
	cases := []struct {
		name string
		r    BuildResult
		want bool
	}{
		{"build fail", BuildResult{BuildOk: false, Tests: TestRun{Passed: 1}}, false},
		{"tests fail", BuildResult{BuildOk: true, Tests: TestRun{Failed: 1}}, false},
		{"all ok", BuildResult{BuildOk: true, Tests: TestRun{Passed: 2}}, true},
		{"no tests", BuildResult{BuildOk: true, Tests: TestRun{}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.r.Ok(); got != tc.want {
				t.Errorf("Ok() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTestCaseFieldsCopyable(t *testing.T) {
	tc := TestCase{Package: "p", Name: "TestX", Status: TestPassed, Duration: time.Millisecond, Output: "ok"}
	if tc.Status != TestPassed {
		t.Fatal("status round-trip")
	}
}
