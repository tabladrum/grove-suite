package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/provasign/provasign/internal/core"
	"github.com/provasign/provasign/internal/engine"
	"github.com/provasign/provasign/internal/enginestore"
	"github.com/provasign/provasign/internal/policy"
	"github.com/provasign/provasign/internal/signer"
)

// fakeGate is a deterministic Gate used to assert the MCP server delegates
// to the engine and surfaces verdicts.
type fakeGate struct {
	name    string
	verdict core.PolicyVerdict
	message string
}

func (g *fakeGate) Name() string { return g.name }
func (g *fakeGate) Evaluate(_ context.Context, _ *core.ChangeSet, _ map[string]any) core.PolicyResult {
	return core.PolicyResult{Gate: g.name, Verdict: g.verdict, Message: g.message}
}

// buildFakeEngine returns an EngineBuilder that yields an Engine pre-wired
// with the supplied gate, all enabled. Each call gets a fresh tempdir-backed
// SQLite store and Ed25519 signer so Submit can persist state.
func buildFakeEngine(g *fakeGate) EngineBuilder {
	return func(root string) (*engine.Engine, func(), error) {
		reg := policy.NewRegistry()
		reg.Register(g)
		cfg := &core.RelayConfig{Policies: map[string]core.PolicyBlock{
			g.name: {Enabled: true},
		}}
		dir, err := os.MkdirTemp("", "mcp-engine-")
		if err != nil {
			return nil, nil, err
		}
		store, err := enginestore.Open(filepath.Join(dir, "engine.db"))
		if err != nil {
			_ = os.RemoveAll(dir)
			return nil, nil, err
		}
		sgnr, err := signer.LoadOrCreateLocal(filepath.Join(dir, "keys"))
		if err != nil {
			_ = store.Close()
			_ = os.RemoveAll(dir)
			return nil, nil, err
		}
		e := &engine.Engine{
			Store:       store,
			Policies:    reg,
			Config:      cfg,
			Signer:      sgnr,
			BuildGates: []string{"coverage"},
			AnalysisGates: []string{"secrets", "deps"},
		}
		cleanup := func() {
			_ = store.Close()
			_ = os.RemoveAll(dir)
		}
		_ = root
		return e, cleanup, nil
	}
}

// framedRequest renders a single JSON-RPC message with Content-Length framing.
func framedRequest(t *testing.T, id any, method string, params any) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": id, "method": method, "params": params,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return []byte(fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body))
}

// readFrames decodes every response off the wire. The MCP stdio transport
// emits newline-delimited compact JSON (one object per line).
func readFrames(t *testing.T, raw []byte) []map[string]any {
	t.Helper()
	out := []map[string]any{}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var msg map[string]any
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			t.Fatalf("decode %q: %v", line, err)
		}
		out = append(out, msg)
	}
	return out
}

func serveOnce(t *testing.T, s *Server, in []byte) []map[string]any {
	t.Helper()
	var buf bytes.Buffer
	if err := s.Serve(io.NopCloser(bytes.NewReader(in)), &buf); err != nil {
		t.Fatalf("serve: %v", err)
	}
	return readFrames(t, buf.Bytes())
}

// TestInitializeReturnsServerInfo asserts the MCP handshake works.
func TestInitializeReturnsServerInfo(t *testing.T) {
	s := NewServer(t.TempDir(), buildFakeEngine(&fakeGate{name: "x", verdict: core.VerdictAllow}))
	frames := serveOnce(t, s, framedRequest(t, 1, "initialize", map[string]any{}))
	if len(frames) != 1 {
		t.Fatalf("want 1 frame, got %d", len(frames))
	}
	result, _ := frames[0]["result"].(map[string]any)
	info, _ := result["serverInfo"].(map[string]any)
	if info["name"] != "relay" {
		t.Fatalf("serverInfo.name = %v", info["name"])
	}
	if _, ok := result["protocolVersion"]; !ok {
		t.Fatalf("missing protocolVersion")
	}
}

// TestToolsListAdvertisesAll ensures all MCP tools — the 5 core engine tools
// and the 4 Auto-Intent Capture tools — are advertised.
func TestToolsListAdvertisesAll(t *testing.T) {
	s := NewServer("", buildFakeEngine(&fakeGate{name: "x", verdict: core.VerdictAllow}))
	frames := serveOnce(t, s, framedRequest(t, 1, "tools/list", map[string]any{}))
	result, _ := frames[0]["result"].(map[string]any)
	tools, _ := result["tools"].([]any)
	want := map[string]bool{
		"provasign_check": true, "provasign_certify": true, "provasign_submit": true,
		"provasign_policy": true, "provasign_explain": true,
		"provasign_intent_open": true, "provasign_intent_update": true,
		"provasign_intent_close": true, "provasign_intent_list": true,
	}
	if len(tools) != len(want) {
		t.Fatalf("want %d tools, got %d", len(want), len(tools))
	}
	for _, raw := range tools {
		m, _ := raw.(map[string]any)
		delete(want, m["name"].(string))
	}
	if len(want) != 0 {
		t.Fatalf("missing tools: %v", want)
	}
}

// TestUnknownMethodReturnsRPCError covers the default branch of handle().
func TestUnknownMethodReturnsRPCError(t *testing.T) {
	s := NewServer("", buildFakeEngine(&fakeGate{name: "x", verdict: core.VerdictAllow}))
	frames := serveOnce(t, s, framedRequest(t, 1, "tools/unknown", map[string]any{}))
	e, _ := frames[0]["error"].(map[string]any)
	if e == nil || e["code"].(float64) != -32601 {
		t.Fatalf("want -32601, got %v", frames[0])
	}
}

// TestNotificationIsIgnored verifies that requests without an id are dropped.
func TestNotificationIsIgnored(t *testing.T) {
	s := NewServer("", buildFakeEngine(&fakeGate{name: "x", verdict: core.VerdictAllow}))
	body := []byte(`{"jsonrpc":"2.0","method":"initialized"}`)
	in := []byte(fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body))
	frames := serveOnce(t, s, in)
	if len(frames) != 0 {
		t.Fatalf("want no response, got %d", len(frames))
	}
}

// TestRelayCheckAllowed asserts an allowing gate yields allowed=true.
func TestRelayCheckAllowed(t *testing.T) {
	s := NewServer(t.TempDir(), buildFakeEngine(&fakeGate{name: "ok", verdict: core.VerdictAllow}))
	req := framedRequest(t, 1, "tools/call", map[string]any{
		"name":      "provasign_check",
		"arguments": map[string]any{"diff": "diff --git a/x b/x\n", "intent": "I-1", "brief": "test"},
	})
	frames := serveOnce(t, s, req)
	result, _ := frames[0]["result"].(map[string]any)
	content, _ := result["content"].([]any)
	text := content[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, `"Allowed": true`) {
		t.Fatalf("expected Allowed:true, got %s", text)
	}
}

// TestRelayCheckDenied asserts a denying gate yields allowed=false.
func TestRelayCheckDenied(t *testing.T) {
	s := NewServer(t.TempDir(), buildFakeEngine(&fakeGate{name: "no", verdict: core.VerdictDeny, message: "nope"}))
	req := framedRequest(t, 1, "tools/call", map[string]any{
		"name":      "provasign_check",
		"arguments": map[string]any{"diff": "x", "intent": "I-1"},
	})
	frames := serveOnce(t, s, req)
	result, _ := frames[0]["result"].(map[string]any)
	text := result["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, `"Allowed": false`) || !strings.Contains(text, "nope") {
		t.Fatalf("expected denial with message, got %s", text)
	}
}

// TestRelayCheckMissingDiff returns an error when no diff is given.
func TestRelayCheckMissingDiff(t *testing.T) {
	s := NewServer("", buildFakeEngine(&fakeGate{name: "x", verdict: core.VerdictAllow}))
	req := framedRequest(t, 1, "tools/call", map[string]any{
		"name":      "provasign_check",
		"arguments": map[string]any{"intent": "I-1"},
	})
	frames := serveOnce(t, s, req)
	if _, ok := frames[0]["error"]; !ok {
		t.Fatalf("want error, got %v", frames[0])
	}
}

// TestRelayCheckBadDiffPath surfaces read errors as RPC errors.
func TestRelayCheckBadDiffPath(t *testing.T) {
	s := NewServer("", buildFakeEngine(&fakeGate{name: "x", verdict: core.VerdictAllow}))
	req := framedRequest(t, 1, "tools/call", map[string]any{
		"name":      "provasign_check",
		"arguments": map[string]any{"diff_path": "/nonexistent/path/here.patch"},
	})
	frames := serveOnce(t, s, req)
	if _, ok := frames[0]["error"]; !ok {
		t.Fatalf("want error, got %v", frames[0])
	}
}

// TestRelaySubmitDeniedDoesNotAdmit runs relay_submit against a denying gate.
// The gate denial short-circuits before admission, so this safely exercises
// runSubmit without needing a real git repo.
func TestRelaySubmitDeniedDoesNotAdmit(t *testing.T) {
	s := NewServer(t.TempDir(), buildFakeEngine(&fakeGate{name: "no", verdict: core.VerdictDeny, message: "blocked"}))
	req := framedRequest(t, 1, "tools/call", map[string]any{
		"name":      "provasign_submit",
		"arguments": map[string]any{"diff": "x", "intent": "I-1"},
	})
	frames := serveOnce(t, s, req)
	result, _ := frames[0]["result"].(map[string]any)
	if result == nil {
		t.Fatalf("expected result, got %v", frames[0])
	}
	text := result["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, `"Allowed": false`) {
		t.Fatalf("expected Allowed:false, got %s", text)
	}
}

// TestRelayPolicyDefaultRoot uses the server's default root when no `repo` is
// supplied — covers the wd-fallback branch of runPolicy.
func TestRelayPolicyDefaultRoot(t *testing.T) {
	s := NewServer("", buildFakeEngine(&fakeGate{name: "secrets", verdict: core.VerdictAllow}))
	req := framedRequest(t, 1, "tools/call", map[string]any{
		"name": "provasign_policy", "arguments": map[string]any{},
	})
	frames := serveOnce(t, s, req)
	if _, ok := frames[0]["result"]; !ok {
		t.Fatalf("want result, got %v", frames[0])
	}
}

// TestRelayCheckDiffPath reads the diff from disk.
func TestRelayCheckDiffPath(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/d.patch"
	if err := writeFile(path, "diff --git a/x b/x\n"); err != nil {
		t.Fatal(err)
	}
	s := NewServer(dir, buildFakeEngine(&fakeGate{name: "ok", verdict: core.VerdictAllow}))
	req := framedRequest(t, 1, "tools/call", map[string]any{
		"name":      "provasign_check",
		"arguments": map[string]any{"diff_path": path},
	})
	frames := serveOnce(t, s, req)
	if _, ok := frames[0]["result"]; !ok {
		t.Fatalf("expected result, got %v", frames[0])
	}
}

// TestRelayPolicyListsGates ensures relay_policy reports registry + config.
func TestRelayPolicyListsGates(t *testing.T) {
	s := NewServer(t.TempDir(), buildFakeEngine(&fakeGate{name: "secrets", verdict: core.VerdictAllow}))
	req := framedRequest(t, 1, "tools/call", map[string]any{
		"name": "provasign_policy", "arguments": map[string]any{},
	})
	frames := serveOnce(t, s, req)
	result := frames[0]["result"].(map[string]any)
	text := result["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "secrets") || !strings.Contains(text, "registered_gates") {
		t.Fatalf("policy output missing fields: %s", text)
	}
}

// TestRelayExplainKnown and TestRelayExplainUnknown cover both help branches.
func TestRelayExplainKnown(t *testing.T) {
	s := NewServer("", buildFakeEngine(&fakeGate{name: "x", verdict: core.VerdictAllow}))
	req := framedRequest(t, 1, "tools/call", map[string]any{
		"name":      "provasign_explain",
		"arguments": map[string]any{"gate": "secrets", "rule": "aws-access-key-id"},
	})
	frames := serveOnce(t, s, req)
	text := frames[0]["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "AWS access key") {
		t.Fatalf("missing specific help: %s", text)
	}
}

func TestRelayExplainFallback(t *testing.T) {
	s := NewServer("", buildFakeEngine(&fakeGate{name: "x", verdict: core.VerdictAllow}))
	req := framedRequest(t, 1, "tools/call", map[string]any{
		"name":      "provasign_explain",
		"arguments": map[string]any{"gate": "secrets", "rule": "nonexistent"},
	})
	frames := serveOnce(t, s, req)
	text := frames[0]["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "fallback to gate-level") {
		t.Fatalf("missing gate-level fallback: %s", text)
	}
}

func TestRelayExplainUnknown(t *testing.T) {
	s := NewServer("", buildFakeEngine(&fakeGate{name: "x", verdict: core.VerdictAllow}))
	req := framedRequest(t, 1, "tools/call", map[string]any{
		"name":      "provasign_explain",
		"arguments": map[string]any{"gate": "mystery"},
	})
	frames := serveOnce(t, s, req)
	text := frames[0]["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "No builtin explanation") {
		t.Fatalf("missing fallback: %s", text)
	}
}

func TestRelayExplainMissingGate(t *testing.T) {
	s := NewServer("", buildFakeEngine(&fakeGate{name: "x", verdict: core.VerdictAllow}))
	req := framedRequest(t, 1, "tools/call", map[string]any{
		"name": "provasign_explain", "arguments": map[string]any{},
	})
	frames := serveOnce(t, s, req)
	if _, ok := frames[0]["error"]; !ok {
		t.Fatalf("want error, got %v", frames[0])
	}
}

// TestUnknownToolReturnsError exercises the default branch of callTool.
func TestUnknownToolReturnsError(t *testing.T) {
	s := NewServer("", buildFakeEngine(&fakeGate{name: "x", verdict: core.VerdictAllow}))
	req := framedRequest(t, 1, "tools/call", map[string]any{"name": "relay_nope", "arguments": map[string]any{}})
	frames := serveOnce(t, s, req)
	if _, ok := frames[0]["error"]; !ok {
		t.Fatalf("want error for unknown tool, got %v", frames[0])
	}
}

// TestEngineBuilderError surfaces builder failures as RPC errors.
func TestEngineBuilderError(t *testing.T) {
	builder := func(string) (*engine.Engine, func(), error) {
		return nil, func() {}, fmt.Errorf("boom")
	}
	s := NewServer(t.TempDir(), builder)
	req := framedRequest(t, 1, "tools/call", map[string]any{
		"name": "provasign_check", "arguments": map[string]any{"diff": "x"},
	})
	frames := serveOnce(t, s, req)
	if _, ok := frames[0]["error"]; !ok {
		t.Fatalf("want error, got %v", frames[0])
	}
}

// TestUnframedLineIsParsedAsMessage covers the no-framing fallback in readMessage.
func TestUnframedLineIsParsedAsMessage(t *testing.T) {
	s := NewServer("", buildFakeEngine(&fakeGate{name: "x", verdict: core.VerdictAllow}))
	in := []byte(`{"jsonrpc":"2.0","id":7,"method":"initialize","params":{}}` + "\n")
	frames := serveOnce(t, s, in)
	if len(frames) != 1 || frames[0]["id"].(float64) != 7 {
		t.Fatalf("want id=7, got %v", frames)
	}
}

// TestInvalidJSONIsSkipped covers the unmarshal-fail branch of Serve.
func TestInvalidJSONIsSkipped(t *testing.T) {
	s := NewServer("", buildFakeEngine(&fakeGate{name: "x", verdict: core.VerdictAllow}))
	body := []byte("not-json")
	in := []byte(fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body))
	frames := serveOnce(t, s, in)
	if len(frames) != 0 {
		t.Fatalf("expected no frames, got %d", len(frames))
	}
}

// TestToolsCallBadParams covers the invalidParams branch when arguments is
// not an object.
func TestToolsCallBadParams(t *testing.T) {
	s := NewServer("", buildFakeEngine(&fakeGate{name: "x", verdict: core.VerdictAllow}))
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":"not-an-object"}`)
	in := []byte(fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body))
	frames := serveOnce(t, s, in)
	if _, ok := frames[0]["error"]; !ok {
		t.Fatalf("want error, got %v", frames[0])
	}
}

// writeFile is a tiny helper.
func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}
