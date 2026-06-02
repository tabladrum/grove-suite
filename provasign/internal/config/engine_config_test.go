package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/provasign/provasign/internal/core"
)

func TestDiscoverRelayRoot_FindsInParent(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".provasign"), 0o755); err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := DiscoverRelayRoot(deep)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	// Resolve symlinks on macOS (/var → /private/var) so the comparison is portable.
	wantResolved, _ := filepath.EvalSymlinks(root)
	gotResolved, _ := filepath.EvalSymlinks(got)
	if wantResolved != gotResolved {
		t.Errorf("got %q want %q", gotResolved, wantResolved)
	}
}

func TestDiscoverRelayRoot_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := DiscoverRelayRoot(dir)
	if !errors.Is(err, ErrRelayRootNotFound) {
		t.Fatalf("want ErrRelayRootNotFound, got %v", err)
	}
}

func TestLoadRelayConfig_DefaultsWhenNoFile(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".provasign"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadRelayConfig(root)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.ProvasignVersion == "" {
		t.Error("expected default provasign_version")
	}
	if _, ok := cfg.Policies["path"]; !ok {
		t.Error("expected default path policy")
	}
}

func TestLoadRelayConfig_ParsesYAML(t *testing.T) {
	root := t.TempDir()
	relayDir := filepath.Join(root, ".provasign")
	if err := os.MkdirAll(relayDir, 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := `provasign_version: "0.2"
stack: go-microservice
policies:
  path:
    enabled: true
    options:
      deny: [".env", "secrets/"]
  size:
    enabled: false
`
	if err := os.WriteFile(filepath.Join(relayDir, "provasign.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadRelayConfig(root)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Stack != "go-microservice" {
		t.Errorf("stack: got %q", cfg.Stack)
	}
	if cfg.Policies["size"].Enabled {
		t.Error("size should be disabled")
	}
	if !cfg.Policies["path"].Enabled {
		t.Error("path should be enabled")
	}
}

func TestEffectiveConfigHash_StableAcrossSourcePath(t *testing.T) {
	cfg1 := &core.RelayConfig{
		ProvasignVersion: "0.1",
		Stack:        "go",
		Policies: map[string]core.PolicyBlock{
			"path": {Enabled: true, Options: map[string]any{"deny": []any{".git/"}}},
		},
		SourcePath: "/tmp/repo-a",
	}
	cfg2 := *cfg1
	cfg2.SourcePath = "/different/path/repo-b"

	h1, err := EffectiveConfigHash(cfg1)
	if err != nil {
		t.Fatal(err)
	}
	h2, err := EffectiveConfigHash(&cfg2)
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Errorf("hash should ignore SourcePath: %q vs %q", h1, h2)
	}
	if len(h1) != 64 {
		t.Errorf("expected 64-char sha256 hex, got %d", len(h1))
	}
}

func TestEffectiveConfigHash_ChangesOnPolicyEdit(t *testing.T) {
	cfg := &core.RelayConfig{
		ProvasignVersion: "0.1",
		Policies: map[string]core.PolicyBlock{
			"size": {Enabled: true, Options: map[string]any{"max_files": 100}},
		},
	}
	h1, _ := EffectiveConfigHash(cfg)

	cfg.Policies["size"] = core.PolicyBlock{Enabled: true, Options: map[string]any{"max_files": 50}}
	h2, _ := EffectiveConfigHash(cfg)

	if h1 == h2 {
		t.Error("hash should change when policy options change")
	}
}

func TestEffectiveConfigHash_MapKeyOrderingIsCanonical(t *testing.T) {
	// Same logical config, different declaration order — JSON canonicalization should produce identical hash.
	a := &core.RelayConfig{
		ProvasignVersion: "0.1",
		Policies: map[string]core.PolicyBlock{
			"path": {Enabled: true},
			"size": {Enabled: true},
		},
	}
	b := &core.RelayConfig{
		ProvasignVersion: "0.1",
		Policies: map[string]core.PolicyBlock{
			"size": {Enabled: true},
			"path": {Enabled: true},
		},
	}
	h1, _ := EffectiveConfigHash(a)
	h2, _ := EffectiveConfigHash(b)
	if h1 != h2 {
		t.Errorf("map key order leaked into hash: %q vs %q", h1, h2)
	}
}
