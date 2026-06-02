package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/provasign/provasign/internal/core"
)

// callTool calls the named MCP tool with the given arguments and returns the
// parsed result payload. Fails the test on any RPC error.
func callTool(t *testing.T, s *Server, name string, args map[string]any) map[string]any {
	t.Helper()
	in := framedRequest(t, 1, "tools/call", map[string]any{"name": name, "arguments": args})
	frames := serveOnce(t, s, in)
	if len(frames) == 0 {
		t.Fatalf("no frames returned for %s", name)
	}
	if errObj, _ := frames[0]["error"].(map[string]any); errObj != nil {
		t.Fatalf("rpc error from %s: %v", name, errObj)
	}
	result, _ := frames[0]["result"].(map[string]any)
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		t.Fatalf("no content from %s", name)
	}
	first, _ := content[0].(map[string]any)
	text, _ := first["text"].(string)
	var payload map[string]any
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("decode payload: %v (raw: %s)", err, text)
	}
	return payload
}

// TestIntentCaptureRoundTrip runs the open → update → close → list flow
// against the MCP server and asserts the on-disk artifacts match the docs.
//
// This is the laptop-MVP exit criterion that says: the prompt becomes a
// committed YAML pointed at by an Intent-ID commit trailer, with verbatim
// preservation of the user's request.
func TestIntentCaptureRoundTrip(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".provasign"), 0o755); err != nil {
		t.Fatal(err)
	}
	s := NewServer(repo, buildFakeEngine(&fakeGate{name: "x", verdict: core.VerdictAllow}))

	openResult := callTool(t, s, "provasign_intent_open", map[string]any{
		"title":           "Add rate limiting to /api/auth/* endpoints",
		"description":     "Please add rate limiting, 100 req/min per IP, on the /api/auth/* endpoints.",
		"agent":           "claude-code:1.4.2",
		"model":           "claude-sonnet-4-6:2026-04-15",
		"conversation_ts": "2026-05-30T14:33:09Z",
	})
	intentID, _ := openResult["intent_id"].(string)
	if intentID == "" {
		t.Fatalf("intent_id missing from open: %+v", openResult)
	}
	if !strings.HasPrefix(intentID, "INT-") {
		t.Errorf("intent_id should start with INT-, got %q", intentID)
	}
	draftPath, _ := openResult["draft_path"].(string)
	if _, err := os.Stat(draftPath); err != nil {
		t.Fatalf("draft file missing: %v", err)
	}

	// Update: refine acceptance criteria.
	upd := callTool(t, s, "provasign_intent_update", map[string]any{
		"intent_id": intentID,
		"patch": map[string]any{
			"risk_level":          "low",
			"allowed_paths":       []any{"internal/auth/**", "tests/auth/**"},
			"acceptance_criteria": []any{"POST /api/auth/login returns 429 after 100 reqs"},
		},
	})
	if upd["title"] == nil {
		t.Errorf("update result missing title: %+v", upd)
	}

	// Close: promotion.
	close := callTool(t, s, "provasign_intent_close", map[string]any{
		"intent_id": intentID,
		"agent":     "claude-code:1.4.2",
	})
	committed, _ := close["committed_path"].(string)
	if committed == "" {
		t.Fatalf("committed_path missing: %+v", close)
	}
	if _, err := os.Stat(committed); err != nil {
		t.Fatalf("committed file missing: %v", err)
	}
	if _, err := os.Stat(draftPath); !os.IsNotExist(err) {
		t.Errorf("draft should be removed after promotion: %v", err)
	}
	trailer, _ := close["trailer_block"].(string)
	if !strings.Contains(trailer, "Intent-ID: "+intentID) {
		t.Errorf("trailer missing Intent-ID: %s", trailer)
	}
	if !strings.Contains(trailer, "Intent-Hash: sha256:") {
		t.Errorf("trailer missing Intent-Hash: %s", trailer)
	}

	// The committed YAML must contain the verbatim user prompt — this is
	// the durability invariant that distinguishes Relay from session-only tools.
	data, err := os.ReadFile(committed)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Please add rate limiting") {
		t.Errorf("committed intent missing verbatim prompt: %s", data)
	}
	if !strings.Contains(string(data), "claude-sonnet-4-6") {
		t.Errorf("committed intent missing model: %s", data)
	}

	// List.
	listResult := callTool(t, s, "provasign_intent_list", map[string]any{"status": "committed"})
	intents, _ := listResult["intents"].([]any)
	if len(intents) != 1 {
		t.Fatalf("expected 1 committed intent, got %d", len(intents))
	}
	first, _ := intents[0].(map[string]any)
	if first["id"] != intentID {
		t.Errorf("listed id = %v, want %s", first["id"], intentID)
	}
}

// TestIntentOpenRejectsMissingFields covers the validation in relay_intent_open.
func TestIntentOpenRejectsMissingFields(t *testing.T) {
	s := NewServer(t.TempDir(), buildFakeEngine(&fakeGate{name: "x", verdict: core.VerdictAllow}))
	for _, args := range []map[string]any{
		{"description": "x"}, // missing title
		{"title": "x"},       // missing description
		{},                   // both missing
	} {
		in := framedRequest(t, 1, "tools/call", map[string]any{"name": "provasign_intent_open", "arguments": args})
		frames := serveOnce(t, s, in)
		if errObj, _ := frames[0]["error"].(map[string]any); errObj == nil {
			t.Errorf("expected error for args %v, got %+v", args, frames[0])
		}
	}
}
