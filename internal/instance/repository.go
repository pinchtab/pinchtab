package instance

import (
	"sync"

	"github.com/pinchtab/pinchtab/internal/bridge"
)

// Repository is the in-memory store of the instances the orchestrator runs.
type Repository struct {
	mu        sync.RWMutex
	instances map[string]*bridge.Instance
}

func newRepository() *Repository {
	return &Repository{instances: make(map[string]*bridge.Instance)}
}

// store snapshots inst so the repository owns every struct it serves. Callers
// keep mutating the instance they handed over — the orchestrator does so under
// its own lock — and that would otherwise be two unrelated mutexes guarding the
// same memory. Caller must hold r.mu.
func (r *Repository) store(inst *bridge.Instance) {
	snapshot := *inst
	r.instances[inst.ID] = &snapshot
}

func (r *Repository) running() []bridge.Instance {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]bridge.Instance, 0, len(r.instances))
	for _, inst := range r.instances {
		if inst.Status == bridge.InstanceRunning {
			out = append(out, *inst)
		}
	}
	return out
}

func (r *Repository) get(id string) (*bridge.Instance, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	inst, ok := r.instances[id]
	if !ok {
		return nil, false
	}
	copy := *inst
	return &copy, true
}

func (r *Repository) add(inst *bridge.Instance) {
	if inst == nil {
		return
	}
	r.mu.Lock()
	r.store(inst)
	r.mu.Unlock()
}

func (r *Repository) remove(id string) {
	r.mu.Lock()
	delete(r.instances, id)
	r.mu.Unlock()
}
