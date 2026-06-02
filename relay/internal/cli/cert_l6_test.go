package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tabladrum/grove-suite/relay/internal/core"
)

func TestToJSONLD_IncludesContextAndType(t *testing.T) {
	c := &core.Certificate{
		ID:                  "cert-123",
		ChangeSetID:         "cs-1",
		IntentID:            "intent-1",
		AdmittedCommitSHA:   "abc",
		EffectiveConfigHash: "h",
		PolicyVersion:       "v1",
		SignedBy:            "key-1",
		Signature:           []byte("sig"),
		CreatedAt:           time.Now().UTC(),
		Payload:             map[string]any{"risk_model_version": "v1"},
	}
	doc := toJSONLD(c)
	if doc["@context"] != "https://provora.dev/cert/v1" {
		t.Fatalf("@context: %v", doc["@context"])
	}
	if doc["@type"] != "CodeCertificate" {
		t.Fatalf("@type: %v", doc["@type"])
	}
	if doc["signature_alg"] != "Ed25519" {
		t.Fatalf("signature_alg: %v", doc["signature_alg"])
	}
	if doc["id"] != "cert-123" {
		t.Fatalf("id: %v", doc["id"])
	}
	if _, ok := doc["payload"]; !ok {
		t.Fatal("missing payload")
	}
	// Round-trip through encoding/json to confirm marshalability.
	if _, err := json.Marshal(doc); err != nil {
		t.Fatalf("marshal: %v", err)
	}
}

func TestToJSONLD_NoPayloadOmits(t *testing.T) {
	c := &core.Certificate{ID: "x"}
	doc := toJSONLD(c)
	if _, ok := doc["payload"]; ok {
		t.Fatal("payload key should be absent when payload is nil")
	}
}

func TestGatePairsAndDiff(t *testing.T) {
	a := []core.PolicyResult{
		{Gate: "secrets", Verdict: core.VerdictAllow},
		{Gate: "size", Verdict: core.VerdictAllow},
	}
	b := []core.PolicyResult{
		{Gate: "secrets", Verdict: core.VerdictDeny},
		{Gate: "size", Verdict: core.VerdictAllow},
		{Gate: "coverage", Verdict: core.VerdictWarn},
	}
	diff := diffGates(gatePairs(a), gatePairs(b))
	// Intersection semantics: only "secrets" appears in both and disagrees.
	// "coverage" exists only in b and is intentionally ignored.
	if len(diff) != 1 || diff[0] != "secrets" {
		t.Fatalf("got %v want [secrets]", diff)
	}
}

func TestGatePairsAndDiff_Identical(t *testing.T) {
	x := []core.PolicyResult{{Gate: "a", Verdict: core.VerdictAllow}}
	if d := diffGates(gatePairs(x), gatePairs(x)); len(d) != 0 {
		t.Fatalf("identical gates should have no diff, got %v", d)
	}
}

func TestE2E_CertShowJSONLD(t *testing.T) {
	repo := initGitRepo(t)
	chdir(t, repo)

	if code := RunEngine([]string{"init"}); code != 0 {
		t.Fatal("init failed")
	}
	diff := writeDiffFile(t, repo, "f.txt", "hello")
	subOut := captureStdout(t, func() {
		code := RunEngine([]string{"submit", "--diff", diff, "--intent", "i-1", "--brief", "b", "--author", "a"})
		if code != 0 {
			t.Fatalf("submit exit=%d", code)
		}
	})
	sha := extractCommit(t, subOut)

	out := captureStdout(t, func() {
		if code := RunEngine([]string{"cert", "show", "--jsonld", sha}); code != 0 {
			t.Fatalf("cert show --jsonld exit=%d", code)
		}
	})
	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if doc["@context"] != "https://provora.dev/cert/v1" {
		t.Fatalf("missing @context in JSON-LD output: %s", out)
	}
	if _, ok := doc["payload"]; !ok {
		t.Fatalf("expected payload (risk heatmap) in cert output: %s", out)
	}

	headOut := captureStdout(t, func() {
		if code := RunEngine([]string{"cert", "show", "--jsonld", "HEAD"}); code != 0 {
			t.Fatalf("cert show --jsonld HEAD exit=%d", code)
		}
	})
	var headDoc map[string]any
	if err := json.Unmarshal([]byte(headOut), &headDoc); err != nil {
		t.Fatalf("HEAD output is not valid JSON: %v\n%s", err, headOut)
	}
	if headDoc["admitted_commit_sha"] != sha {
		t.Fatalf("HEAD resolved commit = %v, want %s", headDoc["admitted_commit_sha"], sha)
	}
}

func TestE2E_CertReplay_ByteReproducible(t *testing.T) {
	repo := initGitRepo(t)
	chdir(t, repo)
	if code := RunEngine([]string{"init"}); code != 0 {
		t.Fatal("init")
	}
	diff := writeDiffFile(t, repo, "f.txt", "hello")
	subOut := captureStdout(t, func() {
		if code := RunEngine([]string{"submit", "--diff", diff, "--intent", "i-1", "--brief", "b", "--author", "a"}); code != 0 {
			t.Fatalf("submit exit=%d", code)
		}
	})
	sha := extractCommit(t, subOut)

	out := captureStdout(t, func() {
		if code := RunEngine([]string{"cert", "replay", sha}); code != 0 {
			t.Fatalf("replay exit=%d for byte_reproducible run", code)
		}
	})
	var report ReplayReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("output not JSON: %v\n%s", err, out)
	}
	if report.Verdict != ReplayByteReproducible {
		t.Errorf("verdict: got %q want %q\n%s", report.Verdict, ReplayByteReproducible, out)
	}
	if report.OriginalConfigHash != report.CurrentConfigHash {
		t.Errorf("config hash mismatch: %s vs %s", report.OriginalConfigHash, report.CurrentConfigHash)
	}
}

func TestE2E_CertReplay_ConfigDrift(t *testing.T) {
	repo := initGitRepo(t)
	chdir(t, repo)
	if code := RunEngine([]string{"init"}); code != 0 {
		t.Fatal("init")
	}
	diff := writeDiffFile(t, repo, "f.txt", "hello")
	subOut := captureStdout(t, func() {
		if code := RunEngine([]string{"submit", "--diff", diff, "--intent", "i-1", "--brief", "b", "--author", "a"}); code != 0 {
			t.Fatalf("submit exit=%d", code)
		}
	})
	sha := extractCommit(t, subOut)

	// Mutate .relay/relay.yaml so the current effective config hash differs.
	// Add a real policy block (comments don't affect the parsed config hash).
	cfgPath := filepath.Join(repo, ".relay", "relay.yaml")
	drifted := []byte(`relay_version: "0.1"
policies:
  size:
    enabled: true
    options:
      max_files: 99
      max_added_lines: 4242
`)
	if err := os.WriteFile(cfgPath, drifted, 0o644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		code := RunEngine([]string{"cert", "replay", sha})
		if code != 2 {
			t.Fatalf("expected exit 2 for non-byte-reproducible replay, got %d", code)
		}
	})
	var report ReplayReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("output not JSON: %v\n%s", err, out)
	}
	if report.Verdict != ReplayConfigDrift {
		t.Errorf("verdict: got %q want %q", report.Verdict, ReplayConfigDrift)
	}
}

func TestCmdCert_UnknownSubcommand(t *testing.T) {
	repo := initGitRepo(t)
	chdir(t, repo)
	_ = RunEngine([]string{"init"})
	if code := RunEngine([]string{"cert", "frobnicate", "abc"}); code == 0 {
		t.Fatal("expected non-zero exit for unknown cert subcommand")
	}
}

func TestCmdCert_NoArgs(t *testing.T) {
	if code := RunEngine([]string{"cert"}); code == 0 {
		t.Fatal("expected non-zero exit for `cert` with no subcommand")
	}
}
