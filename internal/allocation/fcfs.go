package allocation

import "github.com/pinchtab/pinchtab/internal/bridge"

// FCFS (First Come First Served) returns the first running candidate.
// This is the default policy — simple, predictable, deterministic.
type FCFS struct{}

func (f *FCFS) Name() string { return "fcfs" }

func (f *FCFS) Select(candidates []bridge.Instance) (bridge.Instance, error) {
	if len(candidates) == 0 {
		return bridge.Instance{}, ErrNoCandidates
	}
	return candidates[0], nil
}
