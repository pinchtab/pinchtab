// Package allocation defines the AllocationPolicy interface and built-in implementations.
// An AllocationPolicy decides which running instance should handle the next request.
// It is independent from Strategy (API style) — both are swappable independently.
package allocation

import (
	"fmt"

	"github.com/pinchtab/pinchtab/internal/bridge"
)

// Policy selects an instance from a list of running candidates.
// Implementations must be safe for concurrent use.
type Policy interface {
	// Name returns the policy identifier (for config/logging).
	Name() string

	// Select picks the best instance from the given candidates.
	// Returns an error if candidates is empty or no suitable instance exists.
	Select(candidates []bridge.Instance) (bridge.Instance, error)
}

// ErrNoCandidates is returned when Select receives an empty slice.
var ErrNoCandidates = fmt.Errorf("no candidate instances available")

// New creates a Policy by name. Returns an error for unknown names.
func New(name string) (Policy, error) {
	switch name {
	case "fcfs", "":
		return &FCFS{}, nil
	case "round_robin":
		return NewRoundRobin(), nil
	case "random":
		return &Random{}, nil
	default:
		return nil, fmt.Errorf("unknown allocation policy: %q (available: fcfs, round_robin, random)", name)
	}
}
