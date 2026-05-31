package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tabladrum/grove-suite/relay/internal/githook"
)

// initRepo creates a minimal git repo (just the .git/hooks dir is enough for
// the hook installers to consider it valid).
func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git", "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestLocalInit_WritesAgentInstructions(t *testing.T) {
	repo := initRepo(t)
	rc := cmdLocalInit([]string{
		"--repo", repo,
		"--skip-tools",
		"--skip-mcp",
	})
	if rc != 0 {
		t.Fatalf("rc %d", rc)
	}
	for _, rel := range agentInstructionFiles() {
		p := filepath.Join(repo, rel)
		body, err := os.ReadFile(p)
		if err != nil {
			t.Errorf("missing %s: %v", rel, err)
			continue
		}
		if !strings.Contains(string(body), relaySteeringMarker) {
			t.Errorf("%s missing relay marker", rel)
		}
	}
}

func TestLocalInit_InstallsHooks(t *testing.T) {
	repo := initRepo(t)
	rc := cmdLocalInit([]string{
		"--repo", repo,
		"--skip-tools",
		"--skip-mcp",
		"--skip-instructions",
	})
	if rc != 0 {
		t.Fatalf("rc %d", rc)
	}
	// Both hook files must exist and be relay-managed.
	for _, name := range []string{githook.HookFileName, githook.PreCommitFileName} {
		p := filepath.Join(repo, ".git", "hooks", name)
		managed, err := githook.IsManaged(p)
		if err != nil {
			t.Fatalf("IsManaged %s: %v", name, err)
		}
		if !managed {
			t.Errorf("%s not relay-managed", name)
		}
	}
}

func TestWriteAgentInstructions_Idempotent(t *testing.T) {
	repo := t.TempDir()
	first := writeAgentInstructions(repo)
	second := writeAgentInstructions(repo)
	if len(first) == 0 {
		t.Fatal("first call wrote nothing")
	}
	if len(second) != 0 {
		t.Errorf("second call should be no-op, wrote %v", second)
	}
}

func TestWriteAgentInstructions_PreservesExisting(t *testing.T) {
	repo := t.TempDir()
	existing := "# Existing project rules\n\nDo the thing.\n"
	p := filepath.Join(repo, "CLAUDE.md")
	if err := os.WriteFile(p, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	writeAgentInstructions(repo)
	body, _ := os.ReadFile(p)
	if !strings.Contains(string(body), "Existing project rules") {
		t.Error("existing content lost")
	}
	if !strings.Contains(string(body), relaySteeringMarker) {
		t.Error("relay block not appended")
	}
}

func TestWriteVSCodeMCP(t *testing.T) {
	repo := t.TempDir()
	p, err := writeVSCodeMCP(repo, "/x/relay")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(p)
	s := string(body)
	for _, want := range []string{`"servers"`, `"relay"`, `"stdio"`, `"/x/relay"`, `"mcp"`, `"serve"`} {
		if !strings.Contains(s, want) {
			t.Errorf("expected %q in %s", want, s)
		}
	}
}

func TestWriteVSCodeMCP_MergesExisting(t *testing.T) {
	repo := t.TempDir()
	dir := filepath.Join(repo, ".vscode")
	_ = os.MkdirAll(dir, 0o755)
	pre := `{"servers": {"prism": {"type":"stdio","command":"/u/prism","args":["mcp"]}}}`
	_ = os.WriteFile(filepath.Join(dir, "mcp.json"), []byte(pre), 0o644)
	if _, err := writeVSCodeMCP(repo, "/x/relay"); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(filepath.Join(dir, "mcp.json"))
	s := string(body)
	if !strings.Contains(s, `"prism"`) || !strings.Contains(s, `"relay"`) {
		t.Errorf("merge lost an entry: %s", s)
	}
}

func TestLocalStatus(t *testing.T) {
	repo := initRepo(t)
	if rc := cmdLocalStatus([]string{"--repo", repo}); rc != 0 {
		t.Errorf("rc %d", rc)
	}
}

func TestRunLocal_Dispatcher(t *testing.T) {
	if rc := RunLocal(nil); rc == 0 {
		t.Error("expected non-zero for empty args")
	}
	if rc := RunLocal([]string{"weird"}); rc == 0 {
		t.Error("expected non-zero for unknown subcommand")
	}
}

func TestSplitCSV(t *testing.T) {
	got := splitCSV(" a, b ,, c")
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("got[%d]=%q, want %q", i, got[i], want[i])
		}
	}
}
