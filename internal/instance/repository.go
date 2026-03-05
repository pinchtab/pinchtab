package instance

import (
	"fmt"
	"sync"

	"github.com/pinchtab/pinchtab/internal/bridge"
)

// Repository manages the in-memory store of instances and delegates
// process lifecycle to an InstanceLauncher (typically the Orchestrator).
type Repository struct {
	mu        sync.RWMutex
	instances map[string]*bridge.Instance
	launcher  InstanceLauncher
}

// NewRepository creates a Repository backed by the given launcher.
func NewRepository(launcher InstanceLauncher) *Repository {
	return &Repository{
		instances: make(map[string]*bridge.Instance),
		launcher:  launcher,
	}
}

// Launch starts a new instance and adds it to the store.
func (r *Repository) Launch(name, port string, headless bool) (*bridge.Instance, error) {
	inst, err := r.launcher.Launch(name, port, headless)
	if err != nil {
		return nil, fmt.Errorf("launch instance: %w", err)
	}
	r.mu.Lock()
	r.instances[inst.ID] = inst
	r.mu.Unlock()
	return inst, nil
}

// Stop terminates an instance and removes it from the store.
func (r *Repository) Stop(id string) error {
	if err := r.launcher.Stop(id); err != nil {
		return fmt.Errorf("stop instance %q: %w", id, err)
	}
	r.mu.Lock()
	delete(r.instances, id)
	r.mu.Unlock()
	return nil
}

// List returns all instances. The returned slice is a snapshot.
func (r *Repository) List() []bridge.Instance {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]bridge.Instance, 0, len(r.instances))
	for _, inst := range r.instances {
		out = append(out, *inst)
	}
	return out
}

// Running returns only instances with status "running".
func (r *Repository) Running() []bridge.Instance {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]bridge.Instance, 0, len(r.instances))
	for _, inst := range r.instances {
		if inst.Status == "running" {
			out = append(out, *inst)
		}
	}
	return out
}

// Get returns an instance by ID.
func (r *Repository) Get(id string) (*bridge.Instance, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	inst, ok := r.instances[id]
	if !ok {
		return nil, false
	}
	copy := *inst
	return &copy, true
}

// Add registers an existing instance (used for sync with external state).
func (r *Repository) Add(inst *bridge.Instance) {
	r.mu.Lock()
	r.instances[inst.ID] = inst
	r.mu.Unlock()
}

// Remove deletes an instance from the store without stopping it.
func (r *Repository) Remove(id string) {
	r.mu.Lock()
	delete(r.instances, id)
	r.mu.Unlock()
}

// Count returns the number of tracked instances.
func (r *Repository) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.instances)
}
