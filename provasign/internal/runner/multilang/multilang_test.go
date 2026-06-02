package multilang

import (
	"context"
	"testing"

	"github.com/provasign/provasign/internal/cert"
)

type fakeLang struct {
	name    string
	detect  bool
	run     cert.TestRun
	runErr  error
	dirSeen string
}

func (f *fakeLang) Name() string         { return f.name }
func (f *fakeLang) Detect(dir string) bool { f.dirSeen = dir; return f.detect }
func (f *fakeLang) Run(ctx context.Context, dir string) (cert.TestRun, error) {
	return f.run, f.runErr
}

func TestRun_PicksFirstMatch(t *testing.T) {
	a := &fakeLang{name: "go", detect: false}
	b := &fakeLang{name: "py", detect: true, run: cert.TestRun{Runner: "py", Passed: 3}}
	c := &fakeLang{name: "node", detect: true, run: cert.TestRun{Runner: "node", Passed: 9}}
	r := New(a, b, c)
	tr, err := r.Run(context.Background(), "/some/dir")
	if err != nil {
		t.Fatal(err)
	}
	if tr.Runner != "py" || tr.Passed != 3 {
		t.Fatalf("expected first matching (py), got %+v", tr)
	}
	if a.dirSeen != "/some/dir" {
		t.Errorf("first runner not consulted: %q", a.dirSeen)
	}
}

func TestRun_NoMatchReturnsSentinel(t *testing.T) {
	a := &fakeLang{name: "go", detect: false}
	r := New(a)
	tr, err := r.Run(context.Background(), "/x")
	if err != nil {
		t.Fatal(err)
	}
	if tr.Runner != "multilang" {
		t.Errorf("expected multilang sentinel, got %q", tr.Runner)
	}
	if tr.Passed+tr.Failed+tr.Skipped != 0 {
		t.Errorf("sentinel should have zero counts: %+v", tr)
	}
}

func TestMatchedName(t *testing.T) {
	a := &fakeLang{name: "go", detect: false}
	b := &fakeLang{name: "py", detect: true}
	r := New(a, b)
	if got := r.MatchedName("/x"); got != "py" {
		t.Errorf("matched: %q want py", got)
	}
	r2 := New(a)
	if got := r2.MatchedName("/x"); got != "" {
		t.Errorf("expected empty for no match, got %q", got)
	}
}
