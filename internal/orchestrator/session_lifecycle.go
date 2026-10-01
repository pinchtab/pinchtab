package orchestrator

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/handlers"
	"github.com/pinchtab/pinchtab/internal/session"
)

const (
	sessionCloseSlots         = 4
	sessionCloseShutdownGrace = 5 * time.Second
)

func (o *Orchestrator) SessionLifecycleHook() session.LifecycleHook {
	if o == nil {
		return func(session.LifecycleEvent) {}
	}
	return func(evt session.LifecycleEvent) {
		if evt.SessionID == "" {
			return
		}
		o.bindings.ClearSession(evt.SessionID)
		ctx, slots, ok := o.beginSessionClose()
		if !ok {
			return
		}
		go func() {
			defer o.sessionCloses.Done()
			select {
			case slots <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-slots }()
			o.closeEndedSessionTabs(ctx, evt.SessionID)
		}()
	}
}

func (o *Orchestrator) beginSessionClose() (context.Context, chan struct{}, bool) {
	o.sessionCloseMu.Lock()
	defer o.sessionCloseMu.Unlock()
	if o.sessionClosesEnded {
		return nil, nil, false
	}
	if o.sessionCloseCtx == nil {
		o.sessionCloseCtx, o.cancelSessionCloses = context.WithCancel(context.Background())
		o.sessionCloseSlotSem = make(chan struct{}, sessionCloseSlots)
	}
	o.sessionCloses.Add(1)
	return o.sessionCloseCtx, o.sessionCloseSlotSem, true
}

func (o *Orchestrator) endSessionCloses() {
	o.sessionCloseMu.Lock()
	defer o.sessionCloseMu.Unlock()
	o.sessionClosesEnded = true
	if o.cancelSessionCloses != nil {
		o.cancelSessionCloses()
	}
}

func (o *Orchestrator) closeEndedSessionTabs(parent context.Context, sessionID string) {
	o.mu.RLock()
	var targets []*InstanceInternal
	for _, inst := range o.instances {
		if inst.Status == bridge.InstanceRunning && instanceIsActive(inst) && o.hopIsTrusted(inst) {
			targets = append(targets, inst)
		}
	}
	o.mu.RUnlock()

	ctx, cancel := context.WithTimeout(parent, o.timings.sessionTabsCloseBudget)
	defer cancel()
	header := http.Header{}
	header.Set(activity.HeaderPTSessionID, sessionID)
	var wg sync.WaitGroup
	for _, inst := range targets {
		wg.Add(1)
		go func(inst *InstanceInternal) {
			defer wg.Done()
			resp, err := o.instanceRequest(ctx, http.MethodPost, inst, handlers.SessionTabsClosePath, header)
			if err != nil {
				slog.Warn("could not close an ended session's tabs", "sessionId", sessionID, "instanceId", inst.ID, "err", err)
				return
			}
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				slog.Warn("could not close an ended session's tabs", "sessionId", sessionID, "instanceId", inst.ID, "status", resp.StatusCode)
			}
		}(inst)
	}
	wg.Wait()
}
