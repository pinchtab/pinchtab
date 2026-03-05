package allocation

import (
	"math/rand/v2"

	"github.com/pinchtab/pinchtab/internal/bridge"
)

// Random selects a random candidate.
type Random struct{}

func (r *Random) Name() string { return "random" }

func (r *Random) Select(candidates []bridge.Instance) (bridge.Instance, error) {
	if len(candidates) == 0 {
		return bridge.Instance{}, ErrNoCandidates
	}
	return candidates[rand.IntN(len(candidates))], nil
}
