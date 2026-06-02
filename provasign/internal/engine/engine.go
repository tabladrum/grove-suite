// Package engine orchestrates the certified-merge pipeline: policy gates,
// ICR computation, certificate signing, and admission of a ChangeSet onto
// the linear `relay-main` branch.
package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/provasign/provasign/internal/cert"
	"github.com/provasign/provasign/internal/cert/risk"
	"github.com/provasign/provasign/internal/config"
	"github.com/provasign/provasign/internal/core"
	"github.com/provasign/provasign/internal/enginestore"
	"github.com/provasign/provasign/internal/policy"
	"github.com/provasign/provasign/internal/signer"
)

// ICRProvider computes an ICR for a changeset. Implementations may call Grove
// or return a noop ICR (laptop mode without a running Grove).
type ICRProvider interface {
	ComputeICR(ctx context.Context, cs *core.ChangeSet) (core.ICR, error)
}

// BuildRunner executes the build + test stage in an isolated worktree.
// MVP-L2 ships a gotest runner; future stacks (node, python) plug in here.
type BuildRunner interface {
	Run(ctx context.Context, cs *core.ChangeSet) (cert.BuildResult, error)
}

// AnalysisRunner executes the standalone static-analysis stage (secrets, SAST,
// vulnerable-deps) in an isolated worktree and aggregates findings.
type AnalysisRunner interface {
	Run(ctx context.Context, cs *core.ChangeSet) (cert.AnalysisResult, error)
}

// Admitter applies a certified changeset to the repo and returns the resulting
// commit SHA on relay-main. Implementations own all git interaction.
type Admitter interface {
	Admit(ctx context.Context, cs *core.ChangeSet, cert *core.Certificate) (commitSHA string, err error)
}

// Engine wires the pieces together.
type Engine struct {
	Store    *enginestore.Store
	Policies *policy.Registry
	ICR      ICRProvider
	Signer   signer.Signer
	Admit    Admitter
	Config   *core.RelayConfig
	// Build is optional; when nil the build+test stage is skipped entirely
	// (laptop bootstrap mode). BuildGates still run with nil BuildResult
	// so the coverage gate yields ReviewRequired in that case.
	Build BuildRunner
	// BuildGates is the set of gate names that depend on BuildResult and
	// therefore run *after* Build. Defaults to ["coverage"].
	BuildGates []string
	// Analysis is optional; when nil the static-analysis stage is skipped and
	// Analysis-dependent gates (secrets, fileclass, deps) yield ReviewRequired.
	Analysis AnalysisRunner
	// AnalysisGates is the set of gate names that depend on AnalysisResult and
	// therefore run *after* Analysis. Defaults to ["secrets","fileclass","deps"].
	AnalysisGates []string
	Now         func() time.Time // injectable clock; defaults to time.Now

	buildMu     sync.Mutex
	buildResult *cert.BuildResult
	analysisMu     sync.Mutex
	analysisResult *cert.AnalysisResult
}

// Result is the structured outcome of a Check or Submit call.
type Result struct {
	ChangeSet   *core.ChangeSet
	Policies    []core.PolicyResult
	ICR         core.ICR
	Build      *cert.BuildResult // nil if Build was not run
	Analysis      *cert.AnalysisResult // nil if Analysis was not run
	Certificate *core.Certificate  // populated by Certify and Submit
	CommitSHA   string             // populated by Submit
	Allowed     bool               // true when no gate blocks
}

// BuildResult returns the most recent Build result captured during Certify
// or Submit. Used by gates (e.g. coverage) that need test evidence.
func (e *Engine) BuildResult() *cert.BuildResult {
	e.buildMu.Lock()
	defer e.buildMu.Unlock()
	return e.buildResult
}

func (e *Engine) setBuildResult(r *cert.BuildResult) {
	e.buildMu.Lock()
	e.buildResult = r
	e.buildMu.Unlock()
}

func (e *Engine) buildGateSet() map[string]bool {
	names := e.BuildGates
	if names == nil {
		names = []string{"coverage"}
	}
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}

// AnalysisResult returns the most recent Analysis result captured during Certify
// or Submit. Used by gates (secrets, deps) that consume static-analysis findings.
func (e *Engine) AnalysisResult() *cert.AnalysisResult {
	e.analysisMu.Lock()
	defer e.analysisMu.Unlock()
	return e.analysisResult
}

func (e *Engine) setAnalysisResult(r *cert.AnalysisResult) {
	e.analysisMu.Lock()
	e.analysisResult = r
	e.analysisMu.Unlock()
}

func (e *Engine) analysisGateSet() map[string]bool {
	names := e.AnalysisGates
	if names == nil {
		names = []string{"secrets", "fileclass", "deps"}
	}
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}

// postStageGateSet is the union of Build and Analysis gates; these run after
// both stages have produced their results.
func (e *Engine) postStageGateSet() map[string]bool {
	m := e.buildGateSet()
	for n := range e.analysisGateSet() {
		m[n] = true
	}
	return m
}

// Check runs the pre-Build policy gates only. Cheap, no build/test.
// Build-dependent gates (coverage) are deferred to Certify.
func (e *Engine) Check(ctx context.Context, cs *core.ChangeSet) (*Result, error) {
	if e.Config == nil {
		return nil, errors.New("engine: Config is nil")
	}
	postSet := e.postStageGateSet()
	results := evaluateFiltered(ctx, e.Policies, cs, e.Config, func(n string) bool { return !postSet[n] })
	return &Result{
		ChangeSet: cs,
		Policies:  results,
		Allowed:   policy.Allowed(results),
	}, nil
}

// Certify runs Check, the Build build+test stage (if configured), the
// Build-dependent gates (coverage), then computes ICR and produces a signed
// (but not admitted) Certificate. Returns Allowed=false if any gate blocks.
func (e *Engine) Certify(ctx context.Context, cs *core.ChangeSet) (*Result, error) {
	res, err := e.Check(ctx, cs)
	if err != nil {
		return nil, err
	}
	if !res.Allowed {
		return res, nil
	}

	// Stage 1: build + test in an isolated worktree.
	e.setBuildResult(nil)
	if e.Build != nil {
		s1, err := e.Build.Run(ctx, cs)
		if err != nil {
			return res, fmt.Errorf("build: %w", err)
		}
		res.Build = &s1
		e.setBuildResult(&s1)
		if !s1.Ok() {
			res.Policies = append(res.Policies, buildPolicyResult(&s1))
			res.Allowed = false
			return res, nil
		}
		res.Policies = append(res.Policies, buildPolicyResult(&s1))
	}

	// Stage 2: standalone static analysis (secrets, SAST, vulnerable deps).
	e.setAnalysisResult(nil)
	if e.Analysis != nil {
		s2, err := e.Analysis.Run(ctx, cs)
		if err != nil {
			return res, fmt.Errorf("analysis: %w", err)
		}
		res.Analysis = &s2
		e.setAnalysisResult(&s2)
		res.Policies = append(res.Policies, analysisPolicyResult(&s2))
	}

	// Post-stage gates: Build (coverage) and Analysis (secrets, fileclass, deps).
	postSet := e.postStageGateSet()
	postResults := evaluateFiltered(ctx, e.Policies, cs, e.Config, func(n string) bool { return postSet[n] })
	res.Policies = append(res.Policies, postResults...)
	res.Allowed = policy.Allowed(res.Policies)
	if !res.Allowed {
		return res, nil
	}

	icr, err := e.ICR.ComputeICR(ctx, cs)
	if err != nil {
		return res, fmt.Errorf("compute icr: %w", err)
	}
	res.ICR = icr

	cfgHash, err := config.EffectiveConfigHash(e.Config)
	if err != nil {
		return res, fmt.Errorf("config hash: %w", err)
	}
	crt := &core.Certificate{
		ID:                  "cert-" + uuid.NewString(),
		ChangeSetID:         cs.ID,
		IntentID:            cs.IntentID,
		BaseSHA:             cs.BaseSHA,
		ICR:                 icr,
		Policies:            res.Policies,
		EffectiveConfigHash: cfgHash,
		PolicyVersion:       "v1",
		SignedBy:            e.Signer.KeyID(),
		CreatedAt:           e.now(),
	}
	crt.Payload = map[string]any{
		"risk_heatmap": risk.Compute(risk.Inputs{
			ICR:    icr,
			Build: res.Build,
			Analysis: res.Analysis,
		}),
		"risk_model_version": risk.ModelVersion,
	}
	sig, err := e.Signer.Sign(crt)
	if err != nil {
		return res, fmt.Errorf("sign cert: %w", err)
	}
	crt.Signature = sig
	res.Certificate = crt
	return res, nil
}

// Submit runs the full pipeline: persist changeset → Certify → Admit →
// persist certificate → mark changeset admitted.
func (e *Engine) Submit(ctx context.Context, cs *core.ChangeSet) (*Result, error) {
	if cs.ID == "" {
		cs.ID = "cs-" + uuid.NewString()
	}
	if err := e.Store.InsertChangeSet(ctx, cs); err != nil {
		return nil, err
	}
	res, err := e.Certify(ctx, cs)
	if err != nil {
		_ = e.Store.UpdateChangeSetStatus(ctx, cs.ID, core.ChangeSetRejected)
		return res, err
	}
	if !res.Allowed {
		_ = e.Store.UpdateChangeSetStatus(ctx, cs.ID, core.ChangeSetRejected)
		return res, nil
	}
	sha, err := e.Admit.Admit(ctx, cs, res.Certificate)
	if err != nil {
		_ = e.Store.UpdateChangeSetStatus(ctx, cs.ID, core.ChangeSetRejected)
		return res, fmt.Errorf("admit: %w", err)
	}
	res.Certificate.AdmittedCommitSHA = sha
	res.CommitSHA = sha
	if err := e.Store.InsertCertificate(ctx, res.Certificate); err != nil {
		return res, fmt.Errorf("persist cert: %w", err)
	}
	if err := e.Store.UpdateChangeSetStatus(ctx, cs.ID, core.ChangeSetAdmitted); err != nil {
		return res, fmt.Errorf("mark admitted: %w", err)
	}
	return res, nil
}

func (e *Engine) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now().UTC()
}

// evaluateFiltered runs the subset of registered gates from cfg.Policies for
// which include(name) is true. Unknown gates fail closed (deny). Results are
// returned in deterministic order.
func evaluateFiltered(ctx context.Context, reg *policy.Registry, cs *core.ChangeSet, cfg *core.RelayConfig, include func(string) bool) []core.PolicyResult {
	names := make([]string, 0, len(cfg.Policies))
	for n, block := range cfg.Policies {
		if !block.Enabled {
			continue
		}
		if !include(n) {
			continue
		}
		names = append(names, n)
	}
	sort.Strings(names)

	out := make([]core.PolicyResult, 0, len(names))
	for _, n := range names {
		g, ok := reg.Get(n)
		if !ok {
			out = append(out, core.PolicyResult{
				Gate:    n,
				Verdict: core.VerdictDeny,
				Message: "gate enabled in config but not registered",
			})
			continue
		}
		out = append(out, g.Evaluate(ctx, cs, cfg.Policies[n].Options))
	}
	return out
}

// buildPolicyResult folds a BuildResult into a synthetic PolicyResult that
// is appended to res.Policies (and persisted on the certificate). Verdict is
// allow when both build and tests pass, deny otherwise.
func buildPolicyResult(s *cert.BuildResult) core.PolicyResult {
	if s.Skipped {
		return core.PolicyResult{
			Gate: "build",
			Verdict: core.VerdictAllow,
			Message: "build skipped: " + s.SkipReason,
		}
	}
	if s.Ok() {
		return core.PolicyResult{
			Gate: "build",
			Verdict: core.VerdictAllow,
			Message: fmt.Sprintf("build ok, %d passed / %d failed / %d skipped, coverage %.1f%%",
				s.Tests.Passed, s.Tests.Failed, s.Tests.Skipped, s.Tests.CoveragePct),
		}
	}
	msg := "build failed"
	if !s.BuildOk {
		msg = "build failed"
	} else if s.Tests.Failed > 0 {
		msg = fmt.Sprintf("%d test failure(s)", s.Tests.Failed)
	} else if s.Tests.Passed == 0 {
		msg = "no tests executed"
	}
	return core.PolicyResult{
		Gate: "build",
		Verdict:    core.VerdictDeny,
		Message:    msg,
		NextAction: "fix build or test failures and re-submit",
	}
}

// analysisPolicyResult folds a AnalysisResult into a synthetic PolicyResult that
// records which analyzers ran and how many findings each produced. The
// verdict is informational (allow) — the secrets/fileclass/deps gates apply
// the actual blocking semantics on the findings.
func analysisPolicyResult(s *cert.AnalysisResult) core.PolicyResult {
	if s.Skipped {
		return core.PolicyResult{
			Gate: "analysis",
			Verdict: core.VerdictAllow,
			Message: "analysis skipped: " + s.SkipReason,
		}
	}
	available, unavailable, totalFindings := 0, 0, 0
	for _, r := range s.Runs {
		if r.Available {
			available++
			totalFindings += r.NumFindings
		} else {
			unavailable++
		}
	}
	return core.PolicyResult{
		Gate: "analysis",
		Verdict: core.VerdictAllow,
		Message: fmt.Sprintf("analysis: %d analyzer(s) ran, %d unavailable, %d total finding(s)",
			available, unavailable, totalFindings),
		Details: map[string]any{
			"runs":           s.Runs,
			"findings_total": totalFindings,
		},
	}
}

// ComputeICRHash returns the canonical hash of the (symbols, files) tuple.
// Used by ICR providers that need to fill in core.ICR.Hash.
func ComputeICRHash(symbols, files []string) string {
	s := append([]string(nil), symbols...)
	f := append([]string(nil), files...)
	sort.Strings(s)
	sort.Strings(f)
	h := sha256.New()
	for _, x := range s {
		h.Write([]byte("S:"))
		h.Write([]byte(x))
		h.Write([]byte{0})
	}
	for _, x := range f {
		h.Write([]byte("F:"))
		h.Write([]byte(x))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// NoopICRProvider returns an empty (zero-confidence) ICR. Used when Grove is
// not running (pure-laptop mode without indexing).
type NoopICRProvider struct{}

// ComputeICR implements ICRProvider.
func (NoopICRProvider) ComputeICR(_ context.Context, cs *core.ChangeSet) (core.ICR, error) {
	files := touchedPaths(cs.Diff)
	return core.ICR{
		Files:      files,
		Confidence: 0,
		Hash:       ComputeICRHash(nil, files),
	}, nil
}

// touchedPaths is a local copy of policy.touchedPaths to avoid an import cycle
// (engine already imports policy, but exposing the helper would leak internals).
func touchedPaths(diff string) []string {
	var out []string
	for _, line := range splitLines(diff) {
		if len(line) < 4 || line[:4] != "+++ " {
			continue
		}
		path := line[4:]
		// trim leading b/, optional CR, trailing tab+meta
		if len(path) > 2 && path[:2] == "b/" {
			path = path[2:]
		}
		if i := indexByte(path, '\t'); i >= 0 {
			path = path[:i]
		}
		path = trimSpace(path)
		if path == "" || path == "/dev/null" {
			continue
		}
		out = append(out, path)
	}
	return out
}

func splitLines(s string) []string {
	out := []string{}
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

func trimSpace(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t' || s[0] == '\r') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}
