package instance

import (
	"fmt"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/instance/allocation"
)

// Allocator selects an instance using the configured AllocationPolicy.
// It reads candidates from the Repository and delegates selection to the policy.
type Allocator struct {
	repo   *Repository
	policy allocation.Policy
}

// NewAllocator creates an Allocator with the given policy.
func NewAllocator(repo *Repository, policy allocation.Policy) *Allocator {
	return &Allocator{repo: repo, policy: policy}
}

// Allocate selects a running instance using the configured policy.
func (a *Allocator) Allocate() (*bridge.Instance, error) {
	candidates := a.repo.Running()
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no running instances available")
	}
	selected, err := a.policy.Select(candidates)
	if err != nil {
		return nil, fmt.Errorf("allocation policy %q failed: %w", a.policy.Name(), err)
	}
	return &selected, nil
}

// Policy returns the current allocation policy.
func (a *Allocator) Policy() allocation.Policy {
	return a.policy
}

// SetPolicy swaps the allocation policy at runtime.
func (a *Allocator) SetPolicy(p allocation.Policy) {
	a.policy = p
}
