package junit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tabladrum/grove-suite/relay/internal/cert"
)

const sampleWrapped = `<?xml version="1.0" encoding="UTF-8"?>
<testsuites>
  <testsuite name="suite-a" tests="3" failures="1" errors="0" skipped="1" time="0.5">
    <testcase classname="suite_a" name="test_ok" time="0.1"/>
    <testcase classname="suite_a" name="test_bad" time="0.2">
      <failure message="boom" type="AssertionError">stack trace</failure>
    </testcase>
    <testcase classname="suite_a" name="test_skipped" time="0.0">
      <skipped/>
    </testcase>
  </testsuite>
</testsuites>`

const sampleBare = `<?xml version="1.0" encoding="UTF-8"?>
<testsuite name="suite-b" tests="2" failures="0" errors="1" skipped="0" time="0.3">
  <testcase classname="suite_b" name="test_one" time="0.1"/>
  <testcase classname="suite_b" name="test_two" time="0.2">
    <error message="kaboom" type="RuntimeError">trace</error>
  </testcase>
</testsuite>`

func TestParse_Wrapped(t *testing.T) {
	tr, err := Parse(strings.NewReader(sampleWrapped), "pytest")
	if err != nil {
		t.Fatal(err)
	}
	if tr.Runner != "pytest" {
		t.Errorf("runner: %s", tr.Runner)
	}
	if tr.Passed != 1 || tr.Failed != 1 || tr.Skipped != 1 {
		t.Errorf("counts P/F/S: %d/%d/%d", tr.Passed, tr.Failed, tr.Skipped)
	}
	if len(tr.Cases) != 3 {
		t.Fatalf("cases: %d", len(tr.Cases))
	}
	// Find the failed case and inspect it.
	var failed *cert.TestCase
	for i := range tr.Cases {
		if tr.Cases[i].Status == cert.TestFailed {
			failed = &tr.Cases[i]
		}
	}
	if failed == nil {
		t.Fatal("no failed case")
	}
	if !strings.Contains(failed.Output, "boom") {
		t.Errorf("output missing message: %q", failed.Output)
	}
}

func TestParse_Bare(t *testing.T) {
	tr, err := Parse(strings.NewReader(sampleBare), "jest")
	if err != nil {
		t.Fatal(err)
	}
	if tr.Passed != 1 || tr.Failed != 1 {
		t.Errorf("counts: P=%d F=%d", tr.Passed, tr.Failed)
	}
}

func TestParse_Garbage(t *testing.T) {
	_, err := Parse(strings.NewReader("not xml at all"), "x")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParse_WrongRoot(t *testing.T) {
	_, err := Parse(strings.NewReader(`<other/>`), "x")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "report.xml")
	if err := writeFile(p, sampleWrapped); err != nil {
		t.Fatal(err)
	}
	tr, err := ParseFile(p, "pytest")
	if err != nil {
		t.Fatal(err)
	}
	if tr.Passed+tr.Failed+tr.Skipped != 3 {
		t.Errorf("expected 3 cases total")
	}
}

func TestParseFile_Missing(t *testing.T) {
	_, err := ParseFile("/nonexistent/path/relay-junit.xml", "x")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseDir_MergesMultipleFiles(t *testing.T) {
	dir := t.TempDir()
	if err := writeFile(filepath.Join(dir, "a.xml"), sampleWrapped); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(filepath.Join(dir, "b.xml"), sampleBare); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(filepath.Join(dir, "junk.xml"), "<noise/>"); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(filepath.Join(dir, "ignore.txt"), "not xml"); err != nil {
		t.Fatal(err)
	}
	tr, err := ParseDir(dir, "surefire")
	if err != nil {
		t.Fatal(err)
	}
	// wrapped: 1P 1F 1S ; bare: 1P 1F
	if tr.Passed != 2 || tr.Failed != 2 || tr.Skipped != 1 {
		t.Errorf("merge counts: P=%d F=%d S=%d", tr.Passed, tr.Failed, tr.Skipped)
	}
	if len(tr.Cases) != 5 {
		t.Errorf("expected 5 cases, got %d", len(tr.Cases))
	}
}

func TestParseDir_Empty(t *testing.T) {
	dir := t.TempDir()
	tr, err := ParseDir(dir, "surefire")
	if err != nil {
		t.Fatal(err)
	}
	if tr.Passed+tr.Failed+tr.Skipped != 0 {
		t.Errorf("expected empty run")
	}
}

func writeFile(path, body string) error {
	return os.WriteFile(path, []byte(body), 0o644)
}
