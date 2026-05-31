// Package govulncheck adapts the govulncheck binary
// (https://golang.org/x/vuln/cmd/govulncheck) into the Stage-2 analyzer
// contract. Unavailable when the binary is not in PATH OR when the target
// directory has no go.mod.
package govulncheck

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/tabladrum/grove-suite/relay/internal/cert"
	"github.com/tabladrum/grove-suite/relay/internal/core"
	"github.com/tabladrum/grove-suite/relay/internal/tools"
)

// Analyzer wraps `govulncheck -json ./...`.
type Analyzer struct{}

// New returns a govulncheck adapter.
func New() *Analyzer { return &Analyzer{} }

// Name implements analyzers.Analyzer.
func (*Analyzer) Name() string { return "govulncheck" }

// Available reports whether govulncheck is in PATH.
func (*Analyzer) Available() bool {
	return tools.Available("govulncheck")
}

// govulncheckEvent is one stream event from `govulncheck -json`.
// We only consume the `finding` events that contain trace information at
// the call-site depth (the deepest, most actionable layer).
type govulncheckEvent struct {
	OSV     *osv     `json:"osv,omitempty"`
	Finding *finding `json:"finding,omitempty"`
}

type osv struct {
	ID      string `json:"id"`
	Summary string `json:"summary"`
}

type finding struct {
	OSV   string   `json:"osv"`
	Trace []traceL `json:"trace"`
}

type traceL struct {
	Module   string `json:"module"`
	Package  string `json:"package"`
	Function string `json:"function"`
	Position *struct {
		Filename string `json:"filename"`
		Line     int    `json:"line"`
	} `json:"position,omitempty"`
}

// Run executes govulncheck against dir and parses the stream.
func (a *Analyzer) Run(ctx context.Context, _ *core.ChangeSet, dir string) ([]cert.Finding, error) {
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
		return nil, nil // not a Go module: nothing to check
	}
	bin := tools.Locate("govulncheck")
	if bin == "" {
		return nil, nil
	}
	cmd := exec.CommandContext(ctx, bin, "-json", "./...")
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// govulncheck exits non-zero when vulns are found; tolerate that
		// (stderr will be empty). Only treat genuine execution failures
		// as an error.
		if _, ok := err.(*exec.ExitError); !ok {
			return nil, errors.New("govulncheck exec: " + err.Error() + ": " + stderr.String())
		}
	}

	osvSummaries := map[string]string{}
	type pending struct {
		osv string
		trc *traceL
	}
	var pendings []pending

	dec := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	for dec.More() {
		var ev govulncheckEvent
		if err := dec.Decode(&ev); err != nil {
			break
		}
		if ev.OSV != nil {
			osvSummaries[ev.OSV.ID] = ev.OSV.Summary
		}
		if ev.Finding != nil && len(ev.Finding.Trace) > 0 {
			// Keep the deepest frame that has a position (the actual call-site).
			var keep *traceL
			for i := range ev.Finding.Trace {
				if ev.Finding.Trace[i].Position != nil {
					keep = &ev.Finding.Trace[i]
				}
			}
			if keep == nil {
				keep = &ev.Finding.Trace[len(ev.Finding.Trace)-1]
			}
			pendings = append(pendings, pending{osv: ev.Finding.OSV, trc: keep})
		}
	}

	var out []cert.Finding
	for _, p := range pendings {
		path, line := "", 0
		if p.trc.Position != nil {
			path = p.trc.Position.Filename
			line = p.trc.Position.Line
		}
		summary := osvSummaries[p.osv]
		if summary == "" {
			summary = "vulnerability " + p.osv
		}
		f := cert.Finding{
			Schema:     cert.FindingsSchemaVersion,
			Analyzer:   "govulncheck",
			Category:   cert.CategoryVuln,
			RuleID:     p.osv,
			Severity:   cert.SeverityHigh,
			Path:       path,
			Line:       line,
			Message:    summary,
			NextAction: "upgrade the vulnerable dependency or constrain its imports",
			Extra:      map[string]any{"module": p.trc.Module, "package": p.trc.Package},
		}
		f.Fingerprint = cert.ComputeFingerprint(f.Analyzer, f.RuleID, f.Path, f.Line, f.Message)
		out = append(out, f)
	}
	return out, nil
}
