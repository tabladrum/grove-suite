package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/tabladrum/grove-suite/relay/internal/cert"
	"github.com/tabladrum/grove-suite/relay/internal/core"
	"github.com/tabladrum/grove-suite/relay/internal/policy"
)

// fakeStage1 is a Stage1Runner for tests.
type fakeStage1 struct {
	result cert.Stage1Result
	err    error
	called int
}

func (f *fakeStage1) Run(_ context.Context, _ *core.ChangeSet) (cert.Stage1Result, error) {
	f.called++
	return f.result, f.err
}

func TestCertify_Stage1PassAttachesAllowResult(t *testing.T) {
	e := newTestEngine(t, &fakeAdmitter{sha: "sha1"})
	e.Stage1 = &fakeStage1{result: cert.Stage1Result{
		BuildOk: true,
		Tests:   cert.TestRun{Passed: 5, CoveragePct: 90},
	}}
	res, err := e.Certify(context.Background(), sampleChangeSet())
	if err != nil {
		t.Fatalf("certify: %v", err)
	}
	if !res.Allowed {
		t.Fatalf("expected allowed, got results=%+v", res.Policies)
	}
	if res.Certificate == nil {
		t.Fatal("expected cert")
	}
	if res.Stage1 == nil || !res.Stage1.Ok() {
		t.Errorf("expected ok stage1, got %+v", res.Stage1)
	}
	found := false
	for _, p := range res.Policies {
		if p.Gate == "stage1" && p.Verdict == core.VerdictAllow {
			found = true
		}
	}
	if !found {
		t.Errorf("expected allow stage1 policy in %+v", res.Policies)
	}
}

func TestCertify_Stage1FailBlocksCertificate(t *testing.T) {
	e := newTestEngine(t, &fakeAdmitter{sha: "sha1"})
	e.Stage1 = &fakeStage1{result: cert.Stage1Result{
		BuildOk: true,
		Tests:   cert.TestRun{Passed: 2, Failed: 1},
	}}
	res, err := e.Certify(context.Background(), sampleChangeSet())
	if err != nil {
		t.Fatalf("certify: %v", err)
	}
	if res.Allowed {
		t.Error("expected denied due to stage1 failure")
	}
	if res.Certificate != nil {
		t.Error("expected nil cert when stage1 fails")
	}
	// stage1 deny result must be present.
	hasDeny := false
	for _, p := range res.Policies {
		if p.Gate == "stage1" && p.Verdict == core.VerdictDeny {
			hasDeny = true
		}
	}
	if !hasDeny {
		t.Errorf("expected stage1 deny in %+v", res.Policies)
	}
}

func TestCertify_Stage1BuildFailNextActionSet(t *testing.T) {
	e := newTestEngine(t, &fakeAdmitter{sha: "sha1"})
	e.Stage1 = &fakeStage1{result: cert.Stage1Result{BuildOk: false}}
	res, _ := e.Certify(context.Background(), sampleChangeSet())
	for _, p := range res.Policies {
		if p.Gate == "stage1" {
			if p.NextAction == "" {
				t.Error("expected NextAction on stage1 failure")
			}
			if p.Message != "build failed" {
				t.Errorf("expected build failed message, got %q", p.Message)
			}
		}
	}
}

func TestCertify_Stage1SkippedAllows(t *testing.T) {
	e := newTestEngine(t, &fakeAdmitter{sha: "sha1"})
	e.Stage1 = &fakeStage1{result: cert.Stage1Result{
		Skipped: true, SkipReason: "no go.mod", BuildOk: true,
	}}
	res, err := e.Certify(context.Background(), sampleChangeSet())
	if err != nil {
		t.Fatalf("certify: %v", err)
	}
	if !res.Allowed {
		t.Error("expected skipped stage1 to be allowed")
	}
	if res.Certificate == nil {
		t.Error("expected certificate for skipped stage1")
	}
}

func TestCertify_Stage1RunnerErrorPropagates(t *testing.T) {
	e := newTestEngine(t, &fakeAdmitter{sha: "sha1"})
	e.Stage1 = &fakeStage1{err: errors.New("boom")}
	_, err := e.Certify(context.Background(), sampleChangeSet())
	if err == nil {
		t.Error("expected error")
	}
}

func TestStage1Result_AccessorIsConcurrencySafe(t *testing.T) {
	e := &Engine{}
	if e.Stage1Result() != nil {
		t.Error("initially nil")
	}
	want := &cert.Stage1Result{BuildOk: true}
	e.setStage1Result(want)
	if got := e.Stage1Result(); got != want {
		t.Errorf("got %p want %p", got, want)
	}
}

func TestStage1GateSet_Default(t *testing.T) {
	e := &Engine{}
	s := e.stage1GateSet()
	if !s["coverage"] {
		t.Error("default should include coverage")
	}
	if s["size"] {
		t.Error("default should not include size")
	}
}

func TestStage1GateSet_Custom(t *testing.T) {
	e := &Engine{Stage1Gates: []string{"custom"}}
	s := e.stage1GateSet()
	if !s["custom"] {
		t.Error("custom not in set")
	}
	if s["coverage"] {
		t.Error("default not used when custom set")
	}
}

// coverageHolderGate is a minimal Gate that reads from a Stage1 holder.
// Used to prove the engine flushes stage1Result before post-stage gates fire.
type coverageHolderGate struct {
	get func() *cert.Stage1Result
	saw *cert.Stage1Result
}

func (g *coverageHolderGate) Name() string { return "coverage" }
func (g *coverageHolderGate) Evaluate(_ context.Context, _ *core.ChangeSet, _ map[string]any) core.PolicyResult {
	g.saw = g.get()
	return core.PolicyResult{Gate: "coverage", Verdict: core.VerdictAllow}
}

func TestCertify_StageOrdering_PostGateSeesStage1Result(t *testing.T) {
	e := newTestEngine(t, &fakeAdmitter{sha: "sha1"})
	s1 := cert.Stage1Result{BuildOk: true, Tests: cert.TestRun{Passed: 1, CoveragePct: 100}}
	e.Stage1 = &fakeStage1{result: s1}

	g := &coverageHolderGate{get: e.Stage1Result}
	e.Policies.Register(g)
	// Enable coverage gate in config.
	e.Config.Policies["coverage"] = core.PolicyBlock{Enabled: true}

	if _, err := e.Certify(context.Background(), sampleChangeSet()); err != nil {
		t.Fatalf("certify: %v", err)
	}
	if g.saw == nil || !g.saw.Ok() {
		t.Errorf("post-gate did not see stage1 result, saw=%+v", g.saw)
	}
}

func TestCheck_DoesNotRunStage1Gates(t *testing.T) {
	e := newTestEngine(t, &fakeAdmitter{sha: "sha1"})
	g := &coverageHolderGate{get: e.Stage1Result}
	e.Policies.Register(g)
	e.Config.Policies["coverage"] = core.PolicyBlock{Enabled: true}

	res, err := e.Check(context.Background(), sampleChangeSet())
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range res.Policies {
		if p.Gate == "coverage" {
			t.Errorf("coverage gate should be deferred to Certify, got %+v", p)
		}
	}
	_ = policy.Allowed(res.Policies)
}
