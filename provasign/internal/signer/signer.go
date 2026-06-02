// Package signer provides certificate signing. MVP-L1 ships a single local
// Ed25519 implementation; team mode (MVP-T4) adds a KMS-backed Signer behind
// the same interface.
package signer

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/provasign/provasign/internal/core"
)

// Signer abstracts certificate signing.
type Signer interface {
	// KeyID is a stable identifier for the signing key, recorded on the certificate.
	KeyID() string
	// PublicKey returns the raw public-key bytes (Ed25519: 32 bytes).
	PublicKey() []byte
	// Sign produces a signature over the canonical bytes of cert.
	// The cert is treated as input only; the caller assigns the signature.
	Sign(cert *core.Certificate) ([]byte, error)
}

// LocalSigner holds an Ed25519 keypair stored under a directory.
type LocalSigner struct {
	keyID string
	pub   ed25519.PublicKey
	priv  ed25519.PrivateKey
}

// LoadOrCreateLocal loads an Ed25519 keypair from dir; generates a fresh pair
// if no key file exists. The private key is written with 0600 permissions.
// dir is created if missing.
func LoadOrCreateLocal(dir string) (*LocalSigner, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("mkdir keydir: %w", err)
	}
	privPath := filepath.Join(dir, "signing.ed25519.key")
	pubPath := filepath.Join(dir, "signing.ed25519.pub")

	if priv, err := os.ReadFile(privPath); err == nil {
		if len(priv) != ed25519.PrivateKeySize {
			return nil, fmt.Errorf("malformed private key at %s", privPath)
		}
		pub, _ := os.ReadFile(pubPath)
		if len(pub) != ed25519.PublicKeySize {
			pub = ed25519.PrivateKey(priv).Public().(ed25519.PublicKey)
		}
		return &LocalSigner{
			keyID: keyIDFor(pub),
			pub:   ed25519.PublicKey(pub),
			priv:  ed25519.PrivateKey(priv),
		}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read private key: %w", err)
	}

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate ed25519: %w", err)
	}
	if err := os.WriteFile(privPath, priv, 0o600); err != nil {
		return nil, fmt.Errorf("write private key: %w", err)
	}
	if err := os.WriteFile(pubPath, pub, 0o644); err != nil {
		return nil, fmt.Errorf("write public key: %w", err)
	}
	return &LocalSigner{keyID: keyIDFor(pub), pub: pub, priv: priv}, nil
}

func keyIDFor(pub []byte) string {
	sum := sha256.Sum256(pub)
	return "local-ed25519:" + hex.EncodeToString(sum[:8])
}

// KeyID implements Signer.
func (s *LocalSigner) KeyID() string { return s.keyID }

// PublicKey implements Signer.
func (s *LocalSigner) PublicKey() []byte { return append([]byte(nil), s.pub...) }

// Sign implements Signer. It hashes the canonical-JSON encoding of the
// certificate's signed fields and signs the hash.
func (s *LocalSigner) Sign(cert *core.Certificate) ([]byte, error) {
	msg, err := CanonicalBytes(cert)
	if err != nil {
		return nil, err
	}
	return ed25519.Sign(s.priv, msg), nil
}

// Verify checks sig against the public key. Returns nil when valid.
func Verify(pub []byte, cert *core.Certificate, sig []byte) error {
	if len(pub) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid public key size %d", len(pub))
	}
	msg, err := CanonicalBytes(cert)
	if err != nil {
		return err
	}
	if !ed25519.Verify(ed25519.PublicKey(pub), msg, sig) {
		return errors.New("signature verification failed")
	}
	return nil
}

// CanonicalBytes returns the deterministic byte representation of the
// signed portion of a certificate. Signature itself is excluded so the
// produced bytes are what Sign signs and Verify checks.
func CanonicalBytes(cert *core.Certificate) ([]byte, error) {
	// Build an ordered map by hand to avoid struct-tag traps.
	// AdmittedCommitSHA is deliberately excluded — the certificate is signed
	// before admission produces the commit, so the SHA cannot be part of the
	// signed message. The (sha → cert) mapping lives in the engine store.
	view := map[string]any{
		"id":                    cert.ID,
		"changeset_id":          cert.ChangeSetID,
		"intent_id":             cert.IntentID,
		"base_sha":              cert.BaseSHA,
		"icr":                   cert.ICR,
		"policies":              cert.Policies,
		"effective_config_hash": cert.EffectiveConfigHash,
		"policy_version":        cert.PolicyVersion,
		"toolchain_image":       cert.ToolchainImage,
		"signed_by":             cert.SignedBy,
		"created_at":            cert.CreatedAt.UTC().Format("2006-01-02T15:04:05.000000000Z"),
	}
	// json.Marshal sorts map[string]X keys alphabetically — canonical by construction.
	return canonicalMarshal(view)
}

func canonicalMarshal(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	// Round-trip to normalize nested map ordering.
	var any1 any
	if err := json.Unmarshal(raw, &any1); err != nil {
		return nil, err
	}
	return json.Marshal(sortKeys(any1))
}

// sortKeys recursively returns the input with map keys in sorted order.
// json.Marshal already sorts top-level map[string]any, but a recursive walk
// makes the canonical form explicit and unit-testable.
func sortKeys(v any) any {
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make(map[string]any, len(x))
		for _, k := range keys {
			out[k] = sortKeys(x[k])
		}
		return out
	case []any:
		for i, e := range x {
			x[i] = sortKeys(e)
		}
		return x
	default:
		return v
	}
}
