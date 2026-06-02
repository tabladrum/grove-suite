package integration

import (
	"fmt"

	"github.com/provasign/provasign/internal/core"
)

type Registry struct {
	connectors map[string]core.Connector
}

func NewRegistry() *Registry {
	return &Registry{connectors: map[string]core.Connector{}}
}

func (r *Registry) Register(c core.Connector) {
	r.connectors[c.Name()] = c
}

func (r *Registry) Get(name string) (core.Connector, error) {
	c, ok := r.connectors[name]
	if !ok {
		return nil, fmt.Errorf("no connector registered for %q", name)
	}
	return c, nil
}

func (r *Registry) Has(name string) bool {
	_, ok := r.connectors[name]
	return ok
}

func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.connectors))
	for k := range r.connectors {
		names = append(names, k)
	}
	return names
}
