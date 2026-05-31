// Package junit parses JUnit-format XML produced by pytest, jest's
// jest-junit reporter, Maven surefire, Gradle Jupiter, and many others.
// Returns a normalized cert.TestRun the engine can consume directly.
package junit

import (
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tabladrum/grove-suite/relay/internal/cert"
)

// Suite is one <testsuite> element. Multiple suites are common.
type Suite struct {
	XMLName  xml.Name   `xml:"testsuite"`
	Name     string     `xml:"name,attr"`
	Tests    int        `xml:"tests,attr"`
	Failures int        `xml:"failures,attr"`
	Errors   int        `xml:"errors,attr"`
	Skipped  int        `xml:"skipped,attr"`
	Time     float64    `xml:"time,attr"`
	Cases    []TestCase `xml:"testcase"`
}

// Suites is the optional <testsuites> wrapper.
type Suites struct {
	XMLName xml.Name `xml:"testsuites"`
	Suites  []Suite  `xml:"testsuite"`
}

// TestCase is one <testcase> element.
type TestCase struct {
	XMLName   xml.Name `xml:"testcase"`
	Name      string   `xml:"name,attr"`
	Classname string   `xml:"classname,attr"`
	Time      float64  `xml:"time,attr"`
	Failure   *Failure `xml:"failure"`
	Errorr    *Failure `xml:"error"`
	Skipped   *Failure `xml:"skipped"`
	SystemOut string   `xml:"system-out"`
	SystemErr string   `xml:"system-err"`
}

// Failure / Error / Skipped all share the same shape.
type Failure struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Body    string `xml:",chardata"`
}

// Parse reads a single JUnit XML document and returns the merged TestRun.
// It accepts either a top-level <testsuites> wrapper or a bare <testsuite>.
func Parse(r io.Reader, runner string) (cert.TestRun, error) {
	body, err := io.ReadAll(r)
	if err != nil {
		return cert.TestRun{}, fmt.Errorf("junit: read: %w", err)
	}
	suites, err := decode(body)
	if err != nil {
		return cert.TestRun{}, fmt.Errorf("junit: decode: %w", err)
	}
	return aggregate(suites, runner), nil
}

// ParseFile is a convenience wrapper around Parse.
func ParseFile(path, runner string) (cert.TestRun, error) {
	f, err := os.Open(path)
	if err != nil {
		return cert.TestRun{}, fmt.Errorf("junit: open: %w", err)
	}
	defer f.Close()
	return Parse(f, runner)
}

// ParseDir walks dir and parses every *.xml file, merging the results.
// Used for surefire/jupiter where each test class emits its own file.
func ParseDir(dir, runner string) (cert.TestRun, error) {
	merged := cert.TestRun{Runner: runner, StartedAt: time.Now().UTC()}
	any := false
	walkErr := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(p), ".xml") {
			return nil
		}
		body, rerr := os.ReadFile(p)
		if rerr != nil {
			return fmt.Errorf("junit: read %s: %w", p, rerr)
		}
		suites, derr := decode(body)
		if derr != nil {
			// Skip non-JUnit XML files quietly.
			return nil
		}
		tr := aggregate(suites, runner)
		if tr.Passed+tr.Failed+tr.Skipped == 0 {
			return nil
		}
		any = true
		merged.Passed += tr.Passed
		merged.Failed += tr.Failed
		merged.Skipped += tr.Skipped
		merged.Duration += tr.Duration
		merged.Cases = append(merged.Cases, tr.Cases...)
		return nil
	})
	if walkErr != nil {
		return cert.TestRun{}, walkErr
	}
	if !any {
		return cert.TestRun{Runner: runner, StartedAt: merged.StartedAt}, nil
	}
	return merged, nil
}

// decode handles both wrapping styles.
func decode(body []byte) ([]Suite, error) {
	// Try <testsuites> wrapper first.
	var wrap Suites
	if err := xml.Unmarshal(body, &wrap); err == nil && len(wrap.Suites) > 0 {
		return wrap.Suites, nil
	}
	var single Suite
	if err := xml.Unmarshal(body, &single); err != nil {
		return nil, err
	}
	if single.XMLName.Local != "testsuite" {
		return nil, fmt.Errorf("junit: root element is %q, expected testsuite(s)", single.XMLName.Local)
	}
	return []Suite{single}, nil
}

// aggregate merges suites into a single normalized TestRun.
func aggregate(suites []Suite, runner string) cert.TestRun {
	tr := cert.TestRun{
		Runner:    runner,
		StartedAt: time.Now().UTC(),
	}
	totalSec := 0.0
	for _, s := range suites {
		totalSec += s.Time
		for _, c := range s.Cases {
			tc := cert.TestCase{
				Package:  s.Name,
				Name:     caseName(c),
				Duration: time.Duration(c.Time * float64(time.Second)),
			}
			switch {
			case c.Failure != nil:
				tr.Failed++
				tc.Status = cert.TestFailed
				tc.Output = c.Failure.Message + "\n" + c.Failure.Body
			case c.Errorr != nil:
				tr.Failed++
				tc.Status = cert.TestFailed
				tc.Output = c.Errorr.Message + "\n" + c.Errorr.Body
			case c.Skipped != nil:
				tr.Skipped++
				tc.Status = cert.TestSkipped
			default:
				tr.Passed++
				tc.Status = cert.TestPassed
			}
			tr.Cases = append(tr.Cases, tc)
		}
	}
	tr.Duration = time.Duration(totalSec * float64(time.Second))
	return tr
}

func caseName(c TestCase) string {
	if c.Classname == "" {
		return c.Name
	}
	return c.Classname + "." + c.Name
}
