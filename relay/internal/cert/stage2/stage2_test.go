package stage2

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/tabladrum/grove-suite/relay/internal/cert"
	"github.com/tabladrum/grove-suite/relay/internal/core"
)

type fakeAnalyzer struct {
	name      string
	available bool
	findings  []cert.Finding
	err       error
	ran       bool
}

func (f *fakeAnalyzer) Name() string  { return f.name }
func (f *fakeAnalyzer) Available() bool { return f.available }
func (f *fakeAnalyzer) Run(_ context.Context, _ *core.ChangeSet, _ string) ([]cert.Finding, error) {
	f.ran = true
	return f.findings, f.err
}

func initRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not in PATH")
	}
	root := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "--initial-branch=main", "-q")
	run("config", "user.email", "t@e.com")
	run("config", "user.name", "T")
	run("config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(root, "f.txt"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-q", "-m", "seed")
	return root
}

func headSHA(t *testing.T, dir string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	return string(bytesTrim(out))
}

func bytesTrim(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == ' ') {
		b = b[:len(b)-1]
	}
	return b
}

func TestStage2_AggregatesAndSkipsUnavailable(t *testing.T) {
	repo := initRepo(t)
	cs := &core.ChangeSet{RepoRoot: repo, BaseSHA: headSHA(t, repo)}

	a1 := &fakeAnalyzer{name: "a1", available: true, findings: []cert.Finding{
		{Analyzer: "a1", Category: cert.CategorySecret, RuleID: "r1", Path: "x", Line: 1},
	}}
	a2 := &fakeAnalyzer{name: "a2", available: false}

	s := New(a1, a2)
	res, err := s.Run(context.Background(), cs)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !a1.ran {
		t.Error("a1 should have run")
	}
	if a2.ran {
		t.Error("a2 should NOT have run (unavailable)")
	}
	if len(res.Findings) != 1 {
		t.Fatalf("want 1 finding, got %d", len(res.Findings))
	}
	if len(res.Runs) != 2 {
		t.Fatalf("want 2 runs recorded, got %d", len(res.Runs))
	}
	if res.Skipped {
		t.Error("stage shouldn't be skipped when at least one analyzer is available")
	}
}

func TestStage2_AllUnavailableMarksSkipped(t *testing.T) {
	repo := initRepo(t)
	cs := &core.ChangeSet{RepoRoot: repo, BaseSHA: headSHA(t, repo)}

	s := New(&fakeAnalyzer{name: "a", available: false})
	res, err := s.Run(context.Background(), cs)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Skipped {
		t.Errorf("expected Skipped=true")
	}
	if res.SkipReason == "" {
		t.Error("expected SkipReason")
	}
}

func TestStage2_AnalyzerErrorIsRecordedNotFatal(t *testing.T) {
	repo := initRepo(t)
	cs := &core.ChangeSet{RepoRoot: repo, BaseSHA: headSHA(t, repo)}

	a := &fakeAnalyzer{name: "boom", available: true, err: errors.New("kaboom")}
	res, err := s2Run(t, New(a), cs)
	if err != nil {
		t.Fatalf("Run shouldn't fail on analyzer error: %v", err)
	}
	if res.Runs[0].ErrorMsg != "kaboom" {
		t.Errorf("expected ErrorMsg=kaboom, got %q", res.Runs[0].ErrorMsg)
	}
}

func TestStage2_NoAnalyzersConfigured(t *testing.T) {
	repo := initRepo(t)
	cs := &core.ChangeSet{RepoRoot: repo, BaseSHA: headSHA(t, repo)}
	res, err := New().Run(context.Background(), cs)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Skipped {
		t.Error("expected Skipped=true with no analyzers")
	}
}

func s2Run(t *testing.T, s *Stage2, cs *core.ChangeSet) (cert.Stage2Result, error) {
	t.Helper()
	return s.Run(context.Background(), cs)
}
