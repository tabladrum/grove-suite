package signer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tabladrum/grove-suite/relay/internal/core"
)

func sampleCert() *core.Certificate {
	return &core.Certificate{
		ID:                "cert-1",
		ChangeSetID:       "cs-1",
		IntentID:          "intent-1",
		AdmittedCommitSHA: "deadbeef",
		BaseSHA:           "abc",
		ICR: core.ICR{
			Symbols:    []string{"pkg.Foo"},
			Files:      []string{"a.go"},
			Edges:      2,
			Confidence: 0.5,
			Hash:       "icrhash",
		},
		Policies: []core.PolicyResult{
			{Gate: "path", Verdict: core.VerdictAllow, Message: "ok"},
		},
		EffectiveConfigHash: "cfg",
		PolicyVersion:       "v1",
		CreatedAt:           time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
	}
}

func TestLoadOrCreateLocal_GeneratesThenReuses(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "keys")
	s1, err := LoadOrCreateLocal(dir)
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	if s1.KeyID() == "" || !strings.HasPrefix(s1.KeyID(), "local-ed25519:") {
		t.Errorf("unexpected key id: %s", s1.KeyID())
	}
	// Private key file must exist and be 0600.
	privPath := filepath.Join(dir, "signing.ed25519.key")
	info, err := os.Stat(privPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("private key perms: %v", info.Mode().Perm())
	}

	s2, err := LoadOrCreateLocal(dir)
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	if s1.KeyID() != s2.KeyID() {
		t.Errorf("key id changed across loads: %s vs %s", s1.KeyID(), s2.KeyID())
	}
}

func TestSignAndVerify_RoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "keys")
	s, err := LoadOrCreateLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	cert := sampleCert()
	cert.SignedBy = s.KeyID()

	sig, err := s.Sign(cert)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if err := Verify(s.PublicKey(), cert, sig); err != nil {
		t.Errorf("verify clean cert: %v", err)
	}

	// Tampering with any signed field must invalidate the signature.
	tampered := *cert
	tampered.EffectiveConfigHash = "tampered"
	if err := Verify(s.PublicKey(), &tampered, sig); err == nil {
		t.Errorf("expected verification to fail on tampered config hash")
	}
}

func TestVerify_BadKeySize(t *testing.T) {
	if err := Verify([]byte{0x01, 0x02}, sampleCert(), nil); err == nil {
		t.Errorf("expected error for bad pubkey size")
	}
}

func TestCanonicalBytes_StableAcrossPolicyOrder(t *testing.T) {
	c1 := sampleCert()
	c1.Policies = []core.PolicyResult{
		{Gate: "a", Verdict: core.VerdictAllow},
		{Gate: "b", Verdict: core.VerdictAllow},
	}
	c2 := *c1
	// Same policies, same order — canonical bytes must match exactly.
	b1, err := CanonicalBytes(c1)
	if err != nil {
		t.Fatal(err)
	}
	b2, err := CanonicalBytes(&c2)
	if err != nil {
		t.Fatal(err)
	}
	if string(b1) != string(b2) {
		t.Errorf("canonical bytes differ:\n%s\nvs\n%s", b1, b2)
	}
}

func TestLoadOrCreateLocal_RejectsMalformedKey(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "keys")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	// Write a truncated "private key".
	if err := os.WriteFile(filepath.Join(dir, "signing.ed25519.key"), []byte("short"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadOrCreateLocal(dir)
	if err == nil || !strings.Contains(err.Error(), "malformed") {
		t.Errorf("want malformed error, got %v", err)
	}
}
