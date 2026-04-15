package bridge

import (
	"fmt"
	"strings"
	"time"
)

// TabHandoffState tracks manual handoff pauses per tab.
type TabHandoffState struct {
	Status        string    `json:"status"`
	Reason        string    `json:"reason,omitempty"`
	PausedAt      time.Time `json:"pausedAt"`
	LastUpdatedAt time.Time `json:"lastUpdatedAt"`
	ExpiresAt     time.Time `json:"expiresAt,omitempty"`
}

func (b *Bridge) SetTabHandoff(tabID, reason string, timeout time.Duration) error {
	if b == nil {
		return fmt.Errorf("bridge not initialized")
	}
	if strings.TrimSpace(tabID) == "" {
		return fmt.Errorf("tab id required")
	}
	now := time.Now().UTC()
	state := TabHandoffState{
		Status:        "paused_handoff",
		Reason:        strings.TrimSpace(reason),
		PausedAt:      now,
		LastUpdatedAt: now,
	}
	if timeout > 0 {
		state.ExpiresAt = now.Add(timeout)
	}
	if state.Reason == "" {
		state.Reason = "manual_handoff"
	}

	b.handoffMu.Lock()
	defer b.handoffMu.Unlock()
	b.handoffs[tabID] = state
	return nil
}

func (b *Bridge) ResumeTabHandoff(tabID string) error {
	if b == nil {
		return fmt.Errorf("bridge not initialized")
	}
	if strings.TrimSpace(tabID) == "" {
		return fmt.Errorf("tab id required")
	}
	b.handoffMu.Lock()
	defer b.handoffMu.Unlock()
	delete(b.handoffs, tabID)
	return nil
}

func (b *Bridge) TabHandoffState(tabID string) (TabHandoffState, bool) {
	if b == nil {
		return TabHandoffState{}, false
	}
	b.handoffMu.RLock()
	state, ok := b.handoffs[tabID]
	b.handoffMu.RUnlock()
	if !ok {
		return TabHandoffState{}, false
	}
	if !state.ExpiresAt.IsZero() && time.Now().UTC().After(state.ExpiresAt) {
		b.handoffMu.Lock()
		if latest, stillPresent := b.handoffs[tabID]; stillPresent && latest.LastUpdatedAt.Equal(state.LastUpdatedAt) {
			delete(b.handoffs, tabID)
		}
		b.handoffMu.Unlock()
		return TabHandoffState{}, false
	}
	return state, ok
}
