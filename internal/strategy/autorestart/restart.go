package autorestart

import (
	"context"
	"log/slog"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/orchestrator"
)

// launchInitial launches the first instance (or after strategy start).
func (s *Strategy) launchInitial() {
	s.mu.Lock()
	ctx := s.ctx
	s.mu.Unlock()

	if ctx == nil || ctx.Err() != nil {
		return
	}

	inst, err := s.orch.Launch(s.config.ProfileName, "", s.config.Headless, nil)
	if err != nil {
		slog.Error(s.logPrefix("initial launch failed"), "profile", s.config.ProfileName, "err", err)
		return
	}

	s.mu.Lock()
	s.instanceID = inst.ID
	s.headless = inst.Headless
	s.lastStart = time.Now()
	s.mu.Unlock()

	slog.Info(s.logPrefix("instance launched"), "id", inst.ID, "profile", s.config.ProfileName)
}

// handleEvent processes orchestrator lifecycle events.
func (s *Strategy) handleEvent(evt orchestrator.InstanceEvent) {
	s.mu.Lock()
	managedID := s.instanceID
	deliberate := s.deliberate
	restarting := s.restarting
	ctx := s.ctx
	s.mu.Unlock()

	// Only handle events for the managed instance.
	if evt.Instance == nil || evt.Instance.ID != managedID {
		return
	}

	// Skip if a restart is already in progress (prevents duplicate handling
	// when both instance.error and instance.stopped fire for the same crash).
	if restarting {
		return
	}

	switch evt.Type {
	case "instance.stopped":
		if deliberate {
			slog.Info(s.logPrefix("instance stopped deliberately"), "id", managedID)
			return
		}
		// Instance exited unexpectedly — check if we should restart.
		s.handleCrash(ctx, managedID)

	case "instance.error":
		if deliberate {
			return
		}
		s.handleCrash(ctx, managedID)
	}
}

// handleCrash decides whether to restart the crashed instance.
func (s *Strategy) handleCrash(ctx context.Context, crashedID string) {
	if ctx == nil || ctx.Err() != nil {
		return
	}

	s.mu.Lock()
	s.restarting = true
	s.restartCount++
	s.lastCrash = time.Now()
	count := s.restartCount
	maxRestarts := s.config.MaxRestarts
	backoff := s.config.InitBackoff * time.Duration(1<<uint(count-1))
	if backoff > s.config.MaxBackoff {
		backoff = s.config.MaxBackoff
	}
	s.mu.Unlock()

	if s.hasRestartLimit() && count > maxRestarts {
		slog.Error(s.logPrefix("max restarts exceeded, giving up"),
			"id", crashedID,
			"restartCount", count-1,
			"maxRestarts", maxRestarts,
		)
		s.mu.Lock()
		s.restarting = false
		s.mu.Unlock()
		if s.orch != nil {
			s.orch.EmitEvent("instance.crashed", &bridge.Instance{
				ID:     crashedID,
				Status: "crashed",
			})
		}
		return
	}

	args := []any{
		"id", crashedID,
		"attempt", count,
		"backoff", backoff,
	}
	if s.hasRestartLimit() {
		args = append(args, "maxRestarts", maxRestarts)
	}
	slog.Warn(s.logPrefix("instance crashed, scheduling restart"), args...)

	// Emit restarting event.
	if s.orch != nil {
		s.orch.EmitEvent("instance.restarting", &bridge.Instance{
			ID:     crashedID,
			Status: "restarting",
		})
	}

	// Wait for backoff period (respecting cancellation).
	select {
	case <-time.After(backoff):
	case <-ctx.Done():
		return
	}

	s.restartInstance()
}

// restartInstance launches a new instance to replace the crashed one.
func (s *Strategy) restartInstance() {
	s.mu.Lock()
	ctx := s.ctx
	oldID := s.instanceID
	headless := s.headless
	s.mu.Unlock()

	if ctx == nil || ctx.Err() != nil {
		s.mu.Lock()
		s.restarting = false
		s.mu.Unlock()
		return
	}

	// Clean up the old crashed instance so the orchestrator releases the
	// profile slot and allocated port before we attempt a new launch.
	if oldID != "" {
		if err := s.orch.Stop(oldID); err != nil {
			slog.Debug(s.logPrefix("stop old instance (may already be gone)"), "id", oldID, "err", err)
		}
	}

	inst, err := s.orch.Launch(s.config.ProfileName, "", headless, nil)
	if err != nil {
		slog.Error(s.logPrefix("restart failed"),
			"oldId", oldID,
			"err", err,
		)
		s.mu.Lock()
		s.restarting = false
		s.mu.Unlock()
		return
	}

	s.mu.Lock()
	s.instanceID = inst.ID
	s.headless = inst.Headless
	s.lastStart = time.Now()
	count := s.restartCount
	s.restarting = false
	s.mu.Unlock()

	slog.Info(s.logPrefix("instance restarted"),
		"oldId", oldID,
		"newId", inst.ID,
		"attempt", count,
	)

	// Emit restarted event for dashboard/SSE consumers.
	s.orch.EmitEvent("instance.restarted", inst)
}

// stabilityLoop resets the restart counter after the instance runs stably.
func (s *Strategy) stabilityLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		s.mu.Lock()
		ctx := s.ctx
		s.mu.Unlock()

		if ctx == nil {
			return
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.mu.Lock()
			if s.restartCount > 0 && !s.lastStart.IsZero() && time.Since(s.lastStart) > s.config.StableAfter {
				slog.Info(s.logPrefix("instance stable, resetting restart counter"),
					"id", s.instanceID,
					"stableFor", time.Since(s.lastStart).Round(time.Second),
				)
				s.restartCount = 0
			}
			s.mu.Unlock()
		}
	}
}
