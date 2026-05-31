package analyzers

import (
	"context"
	"strings"
	"testing"

	"github.com/tabladrum/grove-suite/relay/internal/cert"
	"github.com/tabladrum/grove-suite/relay/internal/core"
)

type stub struct{ name string }

func (s *stub) Name() string                                                         { return s.name }
func (s *stub) Available() bool                                                      { return true }
func (s *stub) Run(context.Context, *core.ChangeSet, string) ([]cert.Finding, error) { return nil, nil }

func TestBuild_OrdersAndSkipsDisabled(t *testing.T) {
	reg := NewRegistry()
	for _, n := range []string{"alpha", "bravo", "charlie"} {
		n := n
		reg.Register(n, func(name string, _ core.AnalyzerConfig) (Analyzer, error) {
			return &stub{name: name}, nil
		})
	}
	disabled := false
	cfgs := map[string]core.AnalyzerConfig{
		"alpha":   {},                   // default enabled
		"bravo":   {Enabled: &disabled}, // explicitly disabled
		"charlie": {},
	}
	got, err := Build(cfgs, reg)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name() != "alpha" || got[1].Name() != "charlie" {
		t.Fatalf("expected [alpha, charlie] in order, got %v", names(got))
	}
}

func TestBuild_UnknownNameFailsLoudly(t *testing.T) {
	reg := NewRegistry()
	_, err := Build(map[string]core.AnalyzerConfig{"semgrep": {}}, reg)
	if err == nil || !strings.Contains(err.Error(), "no built-in factory") {
		t.Fatalf("expected unknown-name error, got %v", err)
	}
}

func TestBuild_ExternalRequiresFactory(t *testing.T) {
	saved := ExternalFactory
	ExternalFactory = nil
	defer func() { ExternalFactory = saved }()

	enabled := true
	_, err := Build(map[string]core.AnalyzerConfig{
		"mything": {Type: "external", Enabled: &enabled, Command: "echo"},
	}, NewRegistry())
	if err == nil || !strings.Contains(err.Error(), "external factory not registered") {
		t.Fatalf("expected external-factory error, got %v", err)
	}
}

func TestBuild_ExternalDispatch(t *testing.T) {
	saved := ExternalFactory
	defer func() { ExternalFactory = saved }()
	ExternalFactory = func(name string, _ core.AnalyzerConfig) (Analyzer, error) {
		return &stub{name: "ext:" + name}, nil
	}

	enabled := true
	got, err := Build(map[string]core.AnalyzerConfig{
		"mything": {Type: "external", Enabled: &enabled, Command: "echo"},
	}, NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name() != "ext:mything" {
		t.Fatalf("unexpected: %v", names(got))
	}
}

func names(as []Analyzer) []string {
	out := make([]string, len(as))
	for i, a := range as {
		out[i] = a.Name()
	}
	return out
}
