// Package policy contains the gate framework and built-in gates.
// A Gate consumes a ChangeSet (plus optional engine context) and returns a
// PolicyResult. Gates must be deterministic given identical input + config so
// that EffectiveConfigHash actually means something.
package policy

import (
	"context"
	"sort"

	"github.com/tabladrum/grove-suite/relay/internal/core"
)

// Gate is the contract every policy check implements.
type Gate interface {
	// Name is the gate identifier matched against RelayConfig.Policies key.
	Name() string

	// Evaluate runs the gate against the changeset. It should respect ctx
	// cancellation. opts is the raw options map from .relay/relay.yaml.
	Evaluate(ctx context.Context, cs *core.ChangeSet, opts map[string]any) core.PolicyResult
}

// Registry holds the gates available to the engine. Lookup is by Gate.Name().
type Registry struct {
	gates map[string]Gate
}

// NewRegistry constructs an empty registry.
func NewRegistry() *Registry {
	return &Registry{gates: map[string]Gate{}}
}

// Register adds a gate. Re-registering the same name overwrites.
func (r *Registry) Register(g Gate) { r.gates[g.Name()] = g }

// Get returns the gate by name; the second return is false when unknown.
func (r *Registry) Get(name string) (Gate, bool) {
	g, ok := r.gates[name]
	return g, ok
}

// Names returns registered gate names in sorted order (deterministic output).
func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.gates))
	for n := range r.gates {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// EvaluateAll runs every gate enabled in cfg. Gates referenced in cfg but not
// registered yield a deny result so missing gates fail closed.
// Results are returned in deterministic (sorted by gate name) order.
func EvaluateAll(ctx context.Context, reg *Registry, cs *core.ChangeSet, cfg *core.RelayConfig) []core.PolicyResult {
	names := make([]string, 0, len(cfg.Policies))
	for n, block := range cfg.Policies {
		if block.Enabled {
			names = append(names, n)
		}
	}
	sort.Strings(names)

	out := make([]core.PolicyResult, 0, len(names))
	for _, n := range names {
		g, ok := reg.Get(n)
		if !ok {
			out = append(out, core.PolicyResult{
				Gate:    n,
				Verdict: core.VerdictDeny,
				Message: "gate enabled in config but not registered",
			})
			continue
		}
		out = append(out, g.Evaluate(ctx, cs, cfg.Policies[n].Options))
	}
	return out
}

// Allowed reports whether all results in rs are non-blocking.
func Allowed(rs []core.PolicyResult) bool {
	for _, r := range rs {
		if !r.Allowed() {
			return false
		}
	}
	return true
}
