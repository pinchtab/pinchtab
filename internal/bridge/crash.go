package bridge

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chromedp/cdproto/inspector"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
)

const maxRecentCrashes = 20

// CrashEvent contains information about a crash
type CrashEvent struct {
	Time      time.Time `json:"time"`
	TargetID  string    `json:"targetId,omitempty"`
	TabID     string    `json:"tabId,omitempty"`
	URL       string    `json:"url,omitempty"`
	Reason    string    `json:"reason"`
	LastError string    `json:"lastError,omitempty"`
}

// CrashHandler is called when a crash is detected
type CrashHandler func(event CrashEvent)

var (
	crashMu           sync.Mutex
	recentCrashEvents []CrashEvent
	crashEventsTotal  uint64
)

func recordCrashEvent(ev CrashEvent) {
	if ev.Time.IsZero() {
		ev.Time = time.Now()
	}
	atomic.AddUint64(&crashEventsTotal, 1)
	crashMu.Lock()
	defer crashMu.Unlock()
	recentCrashEvents = append(recentCrashEvents, ev)
	if len(recentCrashEvents) > maxRecentCrashes {
		recentCrashEvents = append([]CrashEvent(nil), recentCrashEvents[len(recentCrashEvents)-maxRecentCrashes:]...)
	}
}

// CrashSnapshot returns recent crash diagnostics for /health and /metrics.
func CrashSnapshot() map[string]any {
	crashMu.Lock()
	recent := append([]CrashEvent(nil), recentCrashEvents...)
	crashMu.Unlock()
	return map[string]any{
		"total":  atomic.LoadUint64(&crashEventsTotal),
		"recent": recent,
	}
}

// HasCrashDiagnostics reports whether any crash events have been recorded.
func HasCrashDiagnostics() bool {
	crashMu.Lock()
	recentCount := len(recentCrashEvents)
	crashMu.Unlock()
	return atomic.LoadUint64(&crashEventsTotal) > 0 || recentCount > 0
}

// ResetCrashMonitoringForTests clears crash diagnostics state.
func ResetCrashMonitoringForTests() {
	atomic.StoreUint64(&crashEventsTotal, 0)
	crashMu.Lock()
	recentCrashEvents = nil
	crashMu.Unlock()
}

// RecordCrashForTests injects a crash event for diagnostics tests.
func RecordCrashForTests(ev CrashEvent) {
	recordCrashEvent(ev)
}

// MonitorCrashes listens for browser and tab crashes
func (b *Bridge) MonitorCrashes(handler CrashHandler) {
	if b.BrowserCtx == nil {
		slog.Warn("cannot monitor crashes: no browser context")
		return
	}

	// Listen for target crashes on browser context
	chromedp.ListenTarget(b.BrowserCtx, func(ev interface{}) {
		switch e := ev.(type) {
		case *inspector.EventTargetCrashed:
			event := CrashEvent{
				Time:   time.Now(),
				Reason: "inspector.targetCrashed",
			}
			recordCrashEvent(event)
			slog.Error("🔥 TARGET CRASHED",
				"event", "inspector.targetCrashed",
			)
			if handler != nil {
				handler(event)
			}

		case *target.EventTargetCrashed:
			event := CrashEvent{
				Time:     time.Now(),
				TargetID: string(e.TargetID),
				Reason:   e.Status,
			}
			recordCrashEvent(event)
			slog.Error("🔥 TARGET CRASHED",
				"targetId", e.TargetID,
				"status", e.Status,
				"errorCode", e.ErrorCode,
			)
			if handler != nil {
				handler(event)
			}

		case *target.EventTargetDestroyed:
			if b.TabManager != nil {
				go b.purgeTrackedTabStateByTargetID(string(e.TargetID))
			}
			slog.Debug("target destroyed", "targetId", e.TargetID)
		}
	})

	// Monitor browser context cancellation
	go func() {
		<-b.BrowserCtx.Done()
		err := b.BrowserCtx.Err()
		if err == nil || err == context.Canceled {
			slog.Info("browser context ended", "reason", "context canceled")
			return
		}
		event := CrashEvent{
			Time:   time.Now(),
			Reason: err.Error(),
		}
		recordCrashEvent(event)
		slog.Warn("🔥 BROWSER CONTEXT ENDED",
			"error", err,
		)
		if handler != nil {
			handler(event)
		}
	}()

	slog.Info("crash monitoring enabled")
}

// GetCrashLogs returns recent crash information from Chrome's preferences
func (b *Bridge) GetCrashLogs() []string {
	if b.Config == nil || b.Config.ProfileDir == "" {
		return nil
	}

	var logs []string

	// Check if last exit was unclean
	if WasUncleanExit(b.Config.ProfileDir) {
		logs = append(logs, "Previous session ended with unclean exit (crash detected)")
	}

	return logs
}
