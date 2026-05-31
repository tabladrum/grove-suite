package intent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fixedClock returns a deterministic time for slug stability.
func fixedClock(t *testing.T) func() time.Time {
	t.Helper()
	ts, _ := time.Parse(time.RFC3339, "2026-05-30T14:33:09Z")
	return func() time.Time { return ts }
}

func newService(t *testing.T) *Service {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".relay"), 0o755); err != nil {
		t.Fatal(err)
	}
	s := New(dir)
	s.Now = fixedClock(t)
	return s
}

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Add rate limiting to /api/auth/* endpoints": "add-rate-limiting-to-api-auth-endpoints",
		"  Fix bug #42  ":                     "fix-bug-42",
		"":                                    "",
		"//////":                              "",
		"Use UPPERCASE And Mixed_Underscores": "use-uppercase-and-mixed-underscores",
	}
	for in, want := range cases {
		got := Slugify(in)
		if got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNewID_FormatStableAcrossClock(t *testing.T) {
	ts, _ := time.Parse(time.RFC3339, "2026-05-30T14:33:09Z")
	id := NewID(ts, "Add rate limiting to /api/auth")
	want := "INT-2026-05-30-add-rate-limiting-to-api-auth"
	if id != want {
		t.Errorf("NewID = %q, want %q", id, want)
	}
}

func TestService_OpenWritesDraft(t *testing.T) {
	s := newService(t)
	i, path, err := s.Open(
		"Add rate limiting to /api/auth/*",
		"please add rate limiting, 100 req/min per IP",
		OriginatedFrom{Agent: "claude-code:1.4.2", Model: "claude-sonnet-4-6:2026-04-15"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if i.Status != StatusDraft {
		t.Errorf("Status = %s, want draft", i.Status)
	}
	if i.ID != "INT-2026-05-30-add-rate-limiting-to-api-auth" {
		t.Errorf("ID = %q", i.ID)
	}
	if i.OriginatedFrom.PromptHash == "" {
		t.Error("PromptHash should be auto-populated")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "schema: "+SchemaVersion) {
		t.Errorf("draft missing schema header: %s", data)
	}
	if !strings.Contains(string(data), "please add rate limiting") {
		t.Errorf("draft missing verbatim prompt")
	}
}

func TestService_OpenRequiresTitleAndDescription(t *testing.T) {
	s := newService(t)
	if _, _, err := s.Open("", "x", OriginatedFrom{}); err == nil {
		t.Error("expected error for empty title")
	}
	if _, _, err := s.Open("x", "", OriginatedFrom{}); err == nil {
		t.Error("expected error for empty description")
	}
}

func TestService_UniqueIDOnSameDay(t *testing.T) {
	s := newService(t)
	a, _, _ := s.Open("dup title", "first", OriginatedFrom{})
	b, _, _ := s.Open("dup title", "second", OriginatedFrom{})
	if a.ID == b.ID {
		t.Errorf("expected different IDs, got %q twice", a.ID)
	}
	if !strings.HasSuffix(b.ID, "-2") {
		t.Errorf("second draft should have -2 suffix, got %q", b.ID)
	}
}

func TestService_UpdatePatchesDraft(t *testing.T) {
	s := newService(t)
	i, _, _ := s.Open("orig title", "orig desc", OriginatedFrom{})
	patched, err := s.Update(i.ID, map[string]any{
		"title":         "new title",
		"allowed_paths": []any{"internal/auth/**", "tests/auth/**"},
		"risk_level":    "low",
	})
	if err != nil {
		t.Fatal(err)
	}
	if patched.Title != "new title" {
		t.Errorf("title not patched: %q", patched.Title)
	}
	if len(patched.AllowedPaths) != 2 {
		t.Errorf("allowed_paths = %v", patched.AllowedPaths)
	}
	if patched.RiskLevel != "low" {
		t.Errorf("risk_level = %q", patched.RiskLevel)
	}
}

func TestService_UpdateNotFound(t *testing.T) {
	s := newService(t)
	_, err := s.Update("nonexistent-id", map[string]any{"title": "x"})
	if err == nil {
		t.Fatal("expected error for unknown intent ID, got nil")
	}
}

func TestService_CloseNotFound(t *testing.T) {
	s := newService(t)
	_, _, _, err := s.Close("nonexistent-id", "agent")
	if err == nil {
		t.Fatal("expected error for unknown intent ID, got nil")
	}
}

func TestService_ClosePromotes(t *testing.T) {
	s := newService(t)
	i, draftPath, _ := s.Open("ship feature", "the verbatim prompt", OriginatedFrom{
		Agent: "claude-code", Model: "claude-sonnet-4-6",
	})
	committedPath, hash, trailer, err := s.Close(i.ID, "claude-code:1.4.2")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(committedPath); err != nil {
		t.Errorf("committed file missing: %v", err)
	}
	if _, err := os.Stat(draftPath); !os.IsNotExist(err) {
		t.Errorf("draft should be removed after promotion: %v", err)
	}
	if hash == "" {
		t.Error("hash empty")
	}
	if !strings.Contains(trailer, "Intent-ID: "+i.ID) {
		t.Errorf("trailer missing Intent-ID: %s", trailer)
	}
	if !strings.Contains(trailer, "Intent-Hash: sha256:") {
		t.Errorf("trailer missing Intent-Hash: %s", trailer)
	}
	loaded, err := s.LoadCommitted(i.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != StatusCommitted {
		t.Errorf("loaded status = %s, want committed", loaded.Status)
	}
	if loaded.CommittedAt == nil {
		t.Error("CommittedAt should be set")
	}
	if loaded.CommittedByAgent != "claude-code:1.4.2" {
		t.Errorf("CommittedByAgent = %q", loaded.CommittedByAgent)
	}
}

func TestService_HashStableForSameContent(t *testing.T) {
	s := newService(t)
	i, _, _ := s.Open("stable", "same prompt", OriginatedFrom{Agent: "x", Model: "y"})
	h1, _ := i.Hash()
	h2, _ := i.Hash()
	if h1 != h2 {
		t.Errorf("hash unstable: %s != %s", h1, h2)
	}
	if len(h1) != 64 {
		t.Errorf("hash should be 64 hex chars, got %d", len(h1))
	}
}

func TestService_ListShowsDraftsAndCommitted(t *testing.T) {
	s := newService(t)
	d, _, _ := s.Open("draft intent", "x", OriginatedFrom{})
	c, _, _ := s.Open("committed intent", "y", OriginatedFrom{})
	if _, _, _, err := s.Close(c.ID, "claude-code"); err != nil {
		t.Fatal(err)
	}
	all, err := s.List("all")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(all))
	}
	drafts, _ := s.List("draft")
	if len(drafts) != 1 || drafts[0].ID != d.ID {
		t.Errorf("drafts = %+v", drafts)
	}
	committed, _ := s.List("committed")
	if len(committed) != 1 || committed[0].ID != c.ID {
		t.Errorf("committed = %+v", committed)
	}
}

func TestService_UnmarshalRequiresSchema(t *testing.T) {
	if _, err := Unmarshal([]byte("id: x\ntitle: y\n")); err == nil {
		t.Error("expected error for missing schema")
	}
}
