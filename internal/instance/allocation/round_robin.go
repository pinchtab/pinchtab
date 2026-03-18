package allocation

import (
	"sync/atomic"

	"github.com/pinchtab/pinchtab/internal/bridge"
)

// RoundRobin cycles through candidates in order.
// Thread-safe via atomic counter.
type RoundRobin struct {
	counter atomic.Uint64
}

// NewRoundRobin creates a new RoundRobin policy.
func NewRoundRobin() *RoundRobin {
	return &RoundRobin{}
}

func (rr *RoundRobin) Name() string { return "round_robin" }

func (rr *RoundRobin) Select(candidates []bridge.Instance) (bridge.Instance, error) {
	if len(candidates) == 0 {
		return bridge.Instance{}, ErrNoCandidates
	}
	idx := rr.counter.Add(1) - 1
	return candidates[idx%uint64(len(candidates))], nil
}
