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
	"time"

	"github.com/google/uuid"

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
	Now      func() time.Time // injectable clock; defaults to time.Now
}

// Result is the structured outcome of a Check or Submit call.
type Result struct {
	ChangeSet   *core.ChangeSet
	Policies    []core.PolicyResult
	ICR         core.ICR
	Certificate *core.Certificate // populated by Certify and Submit
	CommitSHA   string            // populated by Submit
	Allowed     bool              // true when no gate blocks
}

// Check runs the policy gates only. Does not compute ICR or sign anything.
// This is the cheapest operation an agent can call before attempting Submit.
func (e *Engine) Check(ctx context.Context, cs *core.ChangeSet) (*Result, error) {
	if e.Config == nil {
		return nil, errors.New("engine: Config is nil")
	}
	results := policy.EvaluateAll(ctx, e.Policies, cs, e.Config)
	return &Result{
		ChangeSet: cs,
		Policies:  results,
		Allowed:   policy.Allowed(results),
	}, nil
}

// Certify runs Check, computes ICR, and produces a signed (but not admitted)
// Certificate. Returns the result with Allowed=false if any gate blocks; in
// that case Certificate is nil.
func (e *Engine) Certify(ctx context.Context, cs *core.ChangeSet) (*Result, error) {
	res, err := e.Check(ctx, cs)
	if err != nil {
		return nil, err
	}
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
	cert := &core.Certificate{
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
	sig, err := e.Signer.Sign(cert)
	if err != nil {
		return res, fmt.Errorf("sign cert: %w", err)
	}
	cert.Signature = sig
	res.Certificate = cert
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
