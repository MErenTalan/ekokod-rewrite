package integration

import "fmt"

// Registry looks up the Adapter for a Provider. It is built once, from the
// fixed set of adapters wired at startup, and is otherwise read-only.
type Registry struct {
	adapters map[Provider]Adapter
}

// NewRegistry builds a Registry from adapters. It refuses two adapters that
// report the same Provider() — that would make Source's result depend on
// slice order, silently swallowing one adapter.
func NewRegistry(adapters ...Adapter) (*Registry, error) {
	m := make(map[Provider]Adapter, len(adapters))
	for _, a := range adapters {
		p := a.Provider()
		if _, exists := m[p]; exists {
			return nil, fmt.Errorf("integration: duplicate provider %q registered", p)
		}
		m[p] = a
	}
	return &Registry{adapters: m}, nil
}

// Source returns the Adapter registered for p. An unregistered provider
// reports an error that wraps ErrNotFound, so callers can use
// errors.Is(err, integration.ErrNotFound) uniformly with adapter-call
// failures.
func (r *Registry) Source(p Provider) (Adapter, error) {
	a, ok := r.adapters[p]
	if !ok {
		return nil, &Error{Kind: ErrNotFound, Provider: p, Op: "registry.source"}
	}
	return a, nil
}
