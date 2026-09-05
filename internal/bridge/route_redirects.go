package bridge

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
)

var ErrTooManyRedirects = errors.New("too many redirects")

// RedirectLimit is one navigation's redirect budget, counted by the route
// dispatch on the tab's Fetch domain so the limiter never owns that domain
// itself. Blocked is set the moment the count exceeds the maximum.
type RedirectLimit struct {
	max     int
	count   atomic.Int32
	blocked atomic.Bool
}

func (l *RedirectLimit) exceed() bool {
	if int(l.count.Add(1)) <= l.max {
		return false
	}
	l.blocked.Store(true)
	return true
}

// Err is the refusal to report once the navigation ends, or nil.
func (l *RedirectLimit) Err() error {
	if l == nil || !l.blocked.Load() {
		return nil
	}
	return fmt.Errorf("%w: got %d, max %d", ErrTooManyRedirects, l.count.Load(), l.max)
}

// ArmRedirectLimit installs a redirect budget for the navigation about to run
// on tabID, claiming the Fetch domain through the same enable rules and
// offline use. ctx must be the tab's long-lived context.
func (rm *RouteManager) ArmRedirectLimit(ctx context.Context, tabID string, maxRedirects int) (*RedirectLimit, error) {
	if rm == nil {
		return nil, fmt.Errorf("route manager not initialized")
	}
	limit := &RedirectLimit{max: maxRedirects}
	rm.mu.Lock()
	state := rm.perTab[tabID]
	if state == nil {
		state = &tabRouteState{}
		rm.perTab[tabID] = state
	}
	previous := state.redirect
	state.redirect = limit
	claim := rm.claimFetchLocked(ctx, state)
	rm.mu.Unlock()

	if err := rm.enableFetch(ctx, tabID, claim); err != nil {
		rm.rollbackClaim(tabID, claim, func(s *tabRouteState) { s.redirect = previous })
		return nil, err
	}
	return limit, nil
}

// DisarmRedirectLimit removes the budget once its navigation is over and
// releases the Fetch domain if nothing else owns it.
func (rm *RouteManager) DisarmRedirectLimit(ctx context.Context, tabID string, limit *RedirectLimit) {
	if rm == nil || limit == nil {
		return
	}
	rm.mu.Lock()
	state := rm.perTab[tabID]
	if state == nil || state.redirect != limit {
		rm.mu.Unlock()
		return
	}
	state.redirect = nil
	release := rm.releaseLocked(tabID, state)
	rm.mu.Unlock()
	release(ctx)
}

// countRedirect charges one redirect hop to the tab's armed budget and reports
// whether that hop exceeded it; a tab with no budget never blocks.
func (rm *RouteManager) countRedirect(tabID string) bool {
	rm.mu.Lock()
	state := rm.perTab[tabID]
	var limit *RedirectLimit
	if state != nil {
		limit = state.redirect
	}
	rm.mu.Unlock()
	return limit != nil && limit.exceed()
}
