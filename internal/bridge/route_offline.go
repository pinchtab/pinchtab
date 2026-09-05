package bridge

import (
	"context"
	"fmt"
)

// SetOffline makes every request on the tab fail as disconnected (on) or
// hands dispatch back to the tab's route rules (off). It shares the route
// rules' Fetch enable, so the domain is claimed once per tab and released only
// when neither rules nor offline own it.
func (rm *RouteManager) SetOffline(ctx context.Context, tabID string, offline bool) error {
	if rm == nil {
		return fmt.Errorf("route manager not initialized")
	}
	rm.mu.Lock()
	state := rm.perTab[tabID]
	if !offline {
		if state == nil {
			rm.mu.Unlock()
			return nil
		}
		state.offline = false
		release := rm.releaseLocked(tabID, state)
		rm.mu.Unlock()
		release(ctx)
		return nil
	}
	if state == nil {
		state = &tabRouteState{}
		rm.perTab[tabID] = state
	}
	prior := state.snapshot()
	state.offline = true
	claim := rm.claimFetchLocked(ctx, state)
	rm.mu.Unlock()

	if err := rm.enableFetch(ctx, tabID, claim); err != nil {
		rm.rollbackClaim(tabID, prior, claim)
		return err
	}
	return nil
}

func (rm *RouteManager) Offline(tabID string) bool {
	if rm == nil {
		return false
	}
	rm.mu.Lock()
	defer rm.mu.Unlock()
	state := rm.perTab[tabID]
	return state != nil && state.offline
}
