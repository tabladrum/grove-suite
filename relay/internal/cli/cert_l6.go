package cli

// L6 helpers: certificate lookup-by-ref, JSON-LD projection, and evidence replay.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/tabladrum/grove-suite/relay/internal/config"
	"github.com/tabladrum/grove-suite/relay/internal/core"
	"github.com/tabladrum/grove-suite/relay/internal/engine"
	"github.com/tabladrum/grove-suite/relay/internal/enginestore"
)

// lookupCert resolves a certificate by ID first, then falls back to
// admitted commit SHA. Returning enginestore.ErrNotFound from both is
// surfaced verbatim so callers can distinguish "no such ref" from real
// I/O errors.
func lookupCert(store *enginestore.Store, ref string) (*core.Certificate, error) {
	ctx := context.Background()
	if c, err := store.GetCertificate(ctx, ref); err == nil {
		return c, nil
	} else if !errors.Is(err, enginestore.ErrNotFound) {
		return nil, err
	}
	return store.GetCertificateByCommit(ctx, ref)
}

// toJSONLD returns a JSON-LD projection of a certificate suitable for
// publication as an "AI code passport". The mapping is intentionally
// minimal — we use a Relay-defined @context URI rather than overloading
// schema.org vocabulary that doesn't quite fit.
func toJSONLD(c *core.Certificate) map[string]any {
	out := map[string]any{
		"@context":              "https://relay.dev/cert/v1",
		"@type":                 "CodeCertificate",
		"id":                    c.ID,
		"changeset_id":          c.ChangeSetID,
		"intent_id":             c.IntentID,
		"admitted_commit_sha":   c.AdmittedCommitSHA,
		"base_sha":              c.BaseSHA,
		"icr":                   c.ICR,
		"policies":              c.Policies,
		"effective_config_hash": c.EffectiveConfigHash,
		"policy_version":        c.PolicyVersion,
		"toolchain_image":       c.ToolchainImage,
		"signed_by":             c.SignedBy,
		"created_at":            c.CreatedAt.UTC().Format(time.RFC3339Nano),
		"signature_alg":         "Ed25519",
	}
	if len(c.Signature) > 0 {
		out["signature"] = c.Signature
	}
	if len(c.Payload) > 0 {
		out["payload"] = c.Payload
	}
	return out
}

// ReplayVerdict enumerates the possible outcomes of a cert replay.
type ReplayVerdict string

const (
	// ReplayByteReproducible: re-running Check against the recorded changeset
	// produced the same Allow/Deny pattern under the same effective config hash.
	ReplayByteReproducible ReplayVerdict = "byte_reproducible"
	// ReplayToolDrift: same config hash but different verdicts —
	// analyzer/tool behaviour changed.
	ReplayToolDrift ReplayVerdict = "tool_drift"
	// ReplayConfigDrift: effective config hash differs from the one
	// recorded in the certificate.
	ReplayConfigDrift ReplayVerdict = "config_drift"
	// ReplayUnrecoverable: the recorded changeset or supporting state
	// cannot be reconstructed.
	ReplayUnrecoverable ReplayVerdict = "unrecoverable"
)

// ReplayReport is emitted by `relay cert replay <ref>` as a JSON document.
type ReplayReport struct {
	CertID                 string        `json:"cert_id"`
	ChangeSetID            string        `json:"changeset_id"`
	Verdict                ReplayVerdict `json:"verdict"`
	OriginalConfigHash     string        `json:"original_config_hash"`
	CurrentConfigHash      string        `json:"current_config_hash"`
	OriginalPolicyVerdicts []GatePair    `json:"original_policy_verdicts"`
	ReplayedPolicyVerdicts []GatePair    `json:"replayed_policy_verdicts,omitempty"`
	DiffingGates           []string      `json:"diffing_gates,omitempty"`
	Note                   string        `json:"note,omitempty"`
}

// GatePair is one gate's verdict in a replay comparison.
type GatePair struct {
	Gate    string `json:"gate"`
	Verdict string `json:"verdict"`
}

// replayCert re-runs the Check pipeline against the changeset that
// originally produced the certificate, then classifies the divergence.
// It does NOT re-run Stage 1 or Stage 2 (those re-execute external
// toolchains and are out of scope for laptop replay); the cert's
// recorded policy results are the authoritative ground truth.
func replayCert(ctx context.Context, store *enginestore.Store, repoRoot string, cert *core.Certificate) (*ReplayReport, error) {
	report := &ReplayReport{
		CertID:                 cert.ID,
		ChangeSetID:            cert.ChangeSetID,
		OriginalConfigHash:     cert.EffectiveConfigHash,
		OriginalPolicyVerdicts: gatePairs(cert.Policies),
	}

	cs, err := store.GetChangeSet(ctx, cert.ChangeSetID)
	if err != nil {
		report.Verdict = ReplayUnrecoverable
		report.Note = fmt.Sprintf("load changeset: %v", err)
		return report, nil
	}

	// Build a fresh engine against the current repo state.
	e, closeEngine, err := BuildEngine(repoRoot)
	if err != nil {
		report.Verdict = ReplayUnrecoverable
		report.Note = fmt.Sprintf("build engine: %v", err)
		return report, nil
	}
	defer closeEngine()

	currentHash, err := config.EffectiveConfigHash(e.Config)
	if err != nil {
		report.Verdict = ReplayUnrecoverable
		report.Note = fmt.Sprintf("config hash: %v", err)
		return report, nil
	}
	report.CurrentConfigHash = currentHash
	if currentHash != cert.EffectiveConfigHash {
		report.Verdict = ReplayConfigDrift
		return report, nil
	}

	res, err := e.Check(ctx, cs)
	if err != nil {
		report.Verdict = ReplayUnrecoverable
		report.Note = fmt.Sprintf("re-check: %v", err)
		return report, nil
	}
	replayed := gatePairs(res.Policies)
	report.ReplayedPolicyVerdicts = replayed
	if diff := diffGates(report.OriginalPolicyVerdicts, replayed); len(diff) > 0 {
		report.Verdict = ReplayToolDrift
		report.DiffingGates = diff
		return report, nil
	}
	report.Verdict = ReplayByteReproducible
	return report, nil
}

// Compile-time check that BuildEngine signature stays compatible.
var _ func(string) (*engine.Engine, func(), error) = BuildEngine

func gatePairs(ps []core.PolicyResult) []GatePair {
	out := make([]GatePair, 0, len(ps))
	for _, p := range ps {
		out = append(out, GatePair{Gate: p.Gate, Verdict: string(p.Verdict)})
	}
	return out
}

// diffGates returns the gate names whose verdict differs between a and b.
// Gates present in only one side are NOT counted — a recheck via Check
// legitimately produces a subset of the certificate's gates (it omits
// Stage 1 / Stage 2 results, which require running external toolchains).
// This conservative comparison reports drift only when a gate evaluated
// on both sides disagrees.
func diffGates(a, b []GatePair) []string {
	bMap := map[string]string{}
	for _, p := range b {
		bMap[p.Gate] = p.Verdict
	}
	diff := []string{}
	for _, p := range a {
		bv, ok := bMap[p.Gate]
		if !ok {
			continue
		}
		if bv != p.Verdict {
			diff = append(diff, p.Gate)
		}
	}
	return diff
}

// Compile-time check that BuildEngine signature stays compatible.
var _ func(string) (*engine.Engine, func(), error) = BuildEngine
