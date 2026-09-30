package autosolver

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// Registry manages solver instances with priority-based ordering.
// Unlike the global solver.Registry, this is instance-level to support
// multiple AutoSolver instances with different solver sets.
type Registry struct {
	mu      sync.RWMutex
	solvers map[string]Solver
	order   []string // maintained in priority order
}

// NewRegistry creates an empty solver registry.
func NewRegistry() *Registry {
	return &Registry{
		solvers: make(map[string]Solver),
	}
}

// Register adds a solver to the registry. Returns an error if a solver
// with the same name is already registered.
func (r *Registry) Register(s Solver) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := s.Name()
	if _, exists := r.solvers[name]; exists {
		return fmt.Errorf("autosolver: solver %q already registered", name)
	}

	r.solvers[name] = s
	r.order = append(r.order, name)
	r.sortOrderLocked()
	return nil
}

// MustRegister is like Register but panics on error.
func (r *Registry) MustRegister(s Solver) {
	if err := r.Register(s); err != nil {
		panic(err)
	}
}

// Get returns a solver by name.
func (r *Registry) Get(name string) (Solver, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.solvers[name]
	return s, ok
}

// Names returns all registered solver names in priority order.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, len(r.order))
	copy(out, r.order)
	return out
}

// All returns all registered solvers in priority order.
func (r *Registry) All() []Solver {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Solver, 0, len(r.order))
	for _, name := range r.order {
		if s, ok := r.solvers[name]; ok {
			out = append(out, s)
		}
	}
	return out
}

// MatchingSolvers returns solvers that can handle the given page,
// sorted by priority. CanHandle is called for each registered solver.
func (r *Registry) MatchingSolvers(ctx context.Context, page Page) []Solver {
	r.mu.RLock()
	names := make([]string, len(r.order))
	copy(names, r.order)
	r.mu.RUnlock()

	var matched []Solver
	for _, name := range names {
		r.mu.RLock()
		s, ok := r.solvers[name]
		r.mu.RUnlock()
		if !ok {
			continue
		}

		can, err := s.CanHandle(ctx, page)
		if err != nil || !can {
			continue
		}
		matched = append(matched, s)
	}
	return matched
}

// sortOrderLocked re-sorts the order slice by solver priority.
// Caller must hold r.mu write lock.
func (r *Registry) sortOrderLocked() {
	sort.SliceStable(r.order, func(i, j int) bool {
		si, oki := r.solvers[r.order[i]]
		sj, okj := r.solvers[r.order[j]]
		if !oki || !okj {
			return oki
		}
		return si.Priority() < sj.Priority()
	})
}
