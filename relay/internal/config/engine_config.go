package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/tabladrum/grove-suite/relay/internal/core"
)

// ErrRelayRootNotFound is returned when no .relay/ directory exists at or above the start path.
var ErrRelayRootNotFound = errors.New("no .relay/ directory found in this path or any parent")

// DiscoverRelayRoot walks upward from start looking for a directory containing `.relay/`.
// Returns the directory containing `.relay/` (NOT the .relay path itself).
func DiscoverRelayRoot(start string) (string, error) {
	abs, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	cur := abs
	for {
		candidate := filepath.Join(cur, ".relay")
		info, statErr := os.Stat(candidate)
		if statErr == nil && info.IsDir() {
			return cur, nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", ErrRelayRootNotFound
		}
		cur = parent
	}
}

// LoadRelayConfig discovers `.relay/relay.yaml` upward from start and loads it.
// Missing optional fields are filled with safe defaults.
func LoadRelayConfig(start string) (*core.RelayConfig, error) {
	root, err := DiscoverRelayRoot(start)
	if err != nil {
		return nil, err
	}
	cfgPath := filepath.Join(root, ".relay", "relay.yaml")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// .relay/ exists but no relay.yaml — return defaults.
			cfg := defaultRelayConfig()
			cfg.SourcePath = root
			return cfg, nil
		}
		return nil, fmt.Errorf("read %s: %w", cfgPath, err)
	}
	cfg := defaultRelayConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", cfgPath, err)
	}
	cfg.SourcePath = root
	return cfg, nil
}

func defaultRelayConfig() *core.RelayConfig {
	return &core.RelayConfig{
		RelayVersion: "0.1",
		Policies: map[string]core.PolicyBlock{
			"path": {Enabled: true, Options: map[string]any{
				"deny": []any{".git/", "vendor/", "node_modules/"},
			}},
			"size": {Enabled: true, Options: map[string]any{
				"max_files":       100,
				"max_added_lines": 5000,
			}},
		},
	}
}

// EffectiveConfigHash returns the sha256 over the canonical JSON encoding of cfg.
// Field ordering is enforced by encoding/json's struct field order plus sorted map keys.
// SourcePath is excluded (json:"-") so the same logical config in two checkouts hashes identically.
func EffectiveConfigHash(cfg *core.RelayConfig) (string, error) {
	canonical, err := canonicalJSON(cfg)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

// canonicalJSON encodes v with sorted map keys at every level.
// encoding/json already sorts map[string]X keys, but nested map[string]any with
// further nested maps does the same via marshal recursion. We round-trip through
// json.Marshal then re-decode-encode to guarantee canonical map ordering for any
// any-typed maps that the yaml decoder produced as `map[string]any` (which
// json.Marshal does sort) — versus `map[any]any` (which is not JSON-marshalable).
// LoadRelayConfig guarantees yaml decodes into `map[string]any` because the
// target type is `map[string]any` (see PolicyBlock.Options).
func canonicalJSON(v any) ([]byte, error) {
	// First pass: marshal. json.Marshal sorts map[string]X keys.
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("canonical marshal: %w", err)
	}
	// Second pass: decode into a generic structure then re-encode so any
	// non-string-keyed maps surfacing from third-party yaml decoders are caught.
	var any1 any
	if err := json.Unmarshal(raw, &any1); err != nil {
		return nil, fmt.Errorf("canonical unmarshal: %w", err)
	}
	return json.Marshal(any1)
}
