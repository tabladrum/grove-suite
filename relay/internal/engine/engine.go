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

	"github.com/tabladrum/grove-suite/relay/internal/cert"
	"github.com/tabladrum/grove-suite/relay/internal/config"
	"github.com/tabladrum/grove-suite/relay/internal/core"
	"github.com/tabladrum/grove-suite/relay/internal/enginestore"
	"github.com/tabladrum/grove-suite/relay/internal/policy"
	"github.com/tabladrum/grove-suite/relay/internal/signer"
)

// ICRProvider computes an ICR for a changeset. Implementations may call Grove
// or return a noop ICR (laptop mode without a running Grove).
type ICRProvider interface {
	ComputeICR(ctx context.Context, cs *core.ChangeSet) (core.ICR, error)
}

// Stage1Runner executes the build + test stage in an isolated worktree.
// MVP-L2 ships a gotest runner; future stacks (node, python) plug in here.
type Stage1Runner interface {
	Run(ctx context.Context, cs *core.ChangeSet) (cert.Stage1Result, error)
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
	// Stage1 is optional; when nil the build+test stage is skipped entirely
	// (laptop bootstrap mode). Stage1Gates still run with nil Stage1Result
	// so the coverage gate yields ReviewRequired in that case.
	Stage1 Stage1Runner
	// Stage1Gates is the set of gate names that depend on Stage1Result and
	// therefore run *after* Stage1. Defaults to ["coverage"].
	Stage1Gates []string
	Now         func() time.Time // injectable clock; defaults to time.Now

	stage1Mu     sync.Mutex
	stage1Result *cert.Stage1Result
}

// Result is the structured outcome of a Check or Submit call.
type Result struct {
	ChangeSet   *core.ChangeSet
	Policies    []core.PolicyResult
	ICR         core.ICR
	Stage1      *cert.Stage1Result // nil if Stage1 was not run
	Certificate *core.Certificate  // populated by Certify and Submit
	CommitSHA   string             // populated by Submit
	Allowed     bool               // true when no gate blocks
}

// Stage1Result returns the most recent Stage1 result captured during Certify
// or Submit. Used by gates (e.g. coverage) that need test evidence.
func (e *Engine) Stage1Result() *cert.Stage1Result {
	e.stage1Mu.Lock()
	defer e.stage1Mu.Unlock()
	return e.stage1Result
}

func (e *Engine) setStage1Result(r *cert.Stage1Result) {
	e.stage1Mu.Lock()
	e.stage1Result = r
	e.stage1Mu.Unlock()
}

func (e *Engine) stage1GateSet() map[string]bool {
	names := e.Stage1Gates
	if names == nil {
		names = []string{"coverage"}
	}
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
}

// Check runs the pre-Stage1 policy gates only. Cheap, no build/test.
// Stage1-dependent gates (coverage) are deferred to Certify.
func (e *Engine) Check(ctx context.Context, cs *core.ChangeSet) (*Result, error) {
	if e.Config == nil {
		return nil, errors.New("engine: Config is nil")
	}
	stage1Set := e.stage1GateSet()
	results := evaluateFiltered(ctx, e.Policies, cs, e.Config, func(n string) bool { return !stage1Set[n] })
	return &Result{
		ChangeSet: cs,
		Policies:  results,
		Allowed:   policy.Allowed(results),
	}, nil
}

// Certify runs Check, the Stage1 build+test stage (if configured), the
// Stage1-dependent gates (coverage), then computes ICR and produces a signed
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
	e.setStage1Result(nil)
	if e.Stage1 != nil {
		s1, err := e.Stage1.Run(ctx, cs)
		if err != nil {
			return res, fmt.Errorf("stage1: %w", err)
		}
		res.Stage1 = &s1
		e.setStage1Result(&s1)
		if !s1.Ok() {
			res.Policies = append(res.Policies, stage1PolicyResult(&s1))
			res.Allowed = false
			return res, nil
		}
		res.Policies = append(res.Policies, stage1PolicyResult(&s1))
	}

	// Post-Stage1 gates (coverage).
	stage1Set := e.stage1GateSet()
	postResults := evaluateFiltered(ctx, e.Policies, cs, e.Config, func(n string) bool { return stage1Set[n] })
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

// stage1PolicyResult folds a Stage1Result into a synthetic PolicyResult that
// is appended to res.Policies (and persisted on the certificate). Verdict is
// allow when both build and tests pass, deny otherwise.
func stage1PolicyResult(s *cert.Stage1Result) core.PolicyResult {
	if s.Skipped {
		return core.PolicyResult{
			Gate:    "stage1",
			Verdict: core.VerdictAllow,
			Message: "stage1 skipped: " + s.SkipReason,
		}
	}
	if s.Ok() {
		return core.PolicyResult{
			Gate:    "stage1",
			Verdict: core.VerdictAllow,
			Message: fmt.Sprintf("build ok, %d passed / %d failed / %d skipped, coverage %.1f%%",
				s.Tests.Passed, s.Tests.Failed, s.Tests.Skipped, s.Tests.CoveragePct),
		}
	}
	msg := "stage1 failed"
	if !s.BuildOk {
		msg = "build failed"
	} else if s.Tests.Failed > 0 {
		msg = fmt.Sprintf("%d test failure(s)", s.Tests.Failed)
	} else if s.Tests.Passed == 0 {
		msg = "no tests executed"
	}
	return core.PolicyResult{
		Gate:       "stage1",
		Verdict:    core.VerdictDeny,
		Message:    msg,
		NextAction: "fix build or test failures and re-submit",
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
