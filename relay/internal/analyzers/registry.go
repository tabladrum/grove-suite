package analyzers

import (
	"fmt"
	"sort"
	"sync"

	"github.com/tabladrum/grove-suite/relay/internal/core"
)

// Factory constructs a configured Analyzer for the given name and config.
// Factories must be deterministic and side-effect-free at construction
// time — defer all I/O to Available()/Run(). The name parameter equals
// the registry key the user wrote in relay.yaml.
type Factory func(name string, cfg core.AnalyzerConfig) (Analyzer, error)

// Registry maps analyzer name → factory. Built-in adapters Register
// themselves in init(); the external plugin shim is looked up by Type
// rather than Name (see ExternalFactory).
type Registry struct {
	mu        sync.RWMutex
	factories map[string]Factory
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry { return &Registry{factories: map[string]Factory{}} }

// Register installs a factory for name. Subsequent calls overwrite,
// supporting user-supplied analyzer replacements that shadow a built-in.
func (r *Registry) Register(name string, f Factory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[name] = f
}

// Lookup returns the factory for name, or (nil, false) when absent.
func (r *Registry) Lookup(name string) (Factory, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	f, ok := r.factories[name]
	return f, ok
}

// Names returns registered factory names in lexicographic order.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.factories))
	for n := range r.factories {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// DefaultRegistry is the package-level registry built-in adapters register
// against. CLI loaders call this unless they pass an explicit override
// (test seam).
var DefaultRegistry = NewRegistry()

// Build resolves a config map into ordered analyzer instances. Disabled
// entries are skipped. External (Type=="external") entries are handed to
// ExternalFactory which must be set by the external package's init().
// Unknown built-in names return an error so typos do not silently no-op.
func Build(cfgs map[string]core.AnalyzerConfig, reg *Registry) ([]Analyzer, error) {
	if reg == nil {
		reg = DefaultRegistry
	}
	names := make([]string, 0, len(cfgs))
	for n := range cfgs {
		names = append(names, n)
	}
	sort.Strings(names)

	var out []Analyzer
	for _, name := range names {
		cfg := cfgs[name]
		if cfg.Type == "external" {
			if !cfg.IsEnabled(false) {
				continue
			}
			if ExternalFactory == nil {
				return nil, fmt.Errorf("analyzer %q: external type requested but external factory not registered", name)
			}
			a, err := ExternalFactory(name, cfg)
			if err != nil {
				return nil, fmt.Errorf("analyzer %q: %w", name, err)
			}
			out = append(out, a)
			continue
		}
		if !cfg.IsEnabled(true) {
			continue
		}
		f, ok := reg.Lookup(name)
		if !ok {
			return nil, fmt.Errorf("analyzer %q: no built-in factory registered (set type: external for custom)", name)
		}
		a, err := f(name, cfg)
		if err != nil {
			return nil, fmt.Errorf("analyzer %q: %w", name, err)
		}
		out = append(out, a)
	}
	return out, nil
}

// ExternalFactory is the factory for Type=="external" entries. The
// internal/analyzers/external package sets this in init() to avoid an
// import cycle with this package.
var ExternalFactory Factory
