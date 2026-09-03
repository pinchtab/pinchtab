package bridge

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chromedp/cdproto/inspector"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
)

const maxRecentCrashes = 20

const maxTabsLostToCrashes = 256

// CrashEvent contains information about a crash. Generation identifies the
// browser context that was live when the crash was recorded; it is internal to
// the liveness lookup and never appears in /health or /metrics output.
type CrashEvent struct {
	Time       time.Time `json:"time"`
	TargetID   string    `json:"targetId,omitempty"`
	TabID      string    `json:"tabId,omitempty"`
	URL        string    `json:"url,omitempty"`
	Reason     string    `json:"reason"`
	LastError  string    `json:"lastError,omitempty"`
	InstanceID string    `json:"instanceId,omitempty"`
	Generation uint64    `json:"-"`
}

// CrashSummary is the crash block of /health and /metrics in both modes.
type CrashSummary struct {
	Total  uint64       `json:"total"`
	Recent []CrashEvent `json:"recent"`
}

// TabNotFoundError names the tab a lookup missed and, when the browser's death
// is what removed it, the crash that did so.
type TabNotFoundError struct {
	TabID string
	Crash *CrashEvent
}

func (e *TabNotFoundError) Error() string { return fmt.Sprintf("tab %s not found", e.TabID) }

func tabNotFound(tabID string) error {
	err := &TabNotFoundError{TabID: tabID}
	if crash, ok := CrashThatDestroyedTab(tabID); ok {
		err.Crash = &crash
	}
	return err
}

// CrashHandler is called when a crash is detected
type CrashHandler func(event CrashEvent)

var (
	crashMu           sync.Mutex
	recentCrashEvents []CrashEvent
	crashEventsTotal  uint64
	tabsLostToCrashes = map[string]CrashEvent{}
	tabsLostOrder     []string

	genMu            sync.Mutex
	generations      = map[context.Context]uint64{}
	generationOrder  []context.Context
	latestGeneration uint64
)

// maxTrackedBrowserContexts bounds the generation table. The process drives few
// browser contexts at a time (one per instance, plus replacements), and evicting
// the oldest only ever costs an annotation, never a false one.
const maxTrackedBrowserContexts = 16

// BrowserContextGeneration numbers browser contexts: each context keeps its own
// generation for as long as it is tracked, and a context never seen before takes
// the next one. Crash state is process-global and shared across every instance
// the process drives, so a crash is only current when its generation matches the
// browser context the request is being served on — that is what keeps a replaced
// browser, or a different instance, from answering for someone else's death.
// Generations are keyed off the context itself rather than a counter the
// lifecycle paths maintain, so no launch/attach/remote-CDP site can forget to
// bump one.
func BrowserContextGeneration(ctx context.Context) uint64 {
	if ctx == nil {
		return 0
	}
	genMu.Lock()
	defer genMu.Unlock()
	if generation, ok := generations[ctx]; ok {
		return generation
	}
	latestGeneration++
	generations[ctx] = latestGeneration
	generationOrder = append(generationOrder, ctx)
	if len(generationOrder) > maxTrackedBrowserContexts {
		delete(generations, generationOrder[0])
		generationOrder = generationOrder[1:]
	}
	return latestGeneration
}

func recordCrashEventForContext(ctx context.Context, ev CrashEvent) {
	ev.Generation = BrowserContextGeneration(ctx)
	recordCrashEvent(ev)
}

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

// CrashForBrowserContext returns the most recent crash recorded for the browser
// context the caller is serving, if any. Crashes from a browser that has since
// been replaced belong to an earlier generation and are not reported, so a
// recovered browser stops answering for its predecessor's death.
func CrashForBrowserContext(ctx context.Context) (CrashEvent, bool) {
	if ctx == nil {
		return CrashEvent{}, false
	}
	generation := BrowserContextGeneration(ctx)
	crashMu.Lock()
	defer crashMu.Unlock()
	for i := len(recentCrashEvents) - 1; i >= 0; i-- {
		if recentCrashEvents[i].Generation == generation {
			return recentCrashEvents[i], true
		}
	}
	return CrashEvent{}, false
}

// CrashSnapshot returns recent crash diagnostics for /health and /metrics.
func CrashSnapshot() CrashSummary {
	crashMu.Lock()
	recent := make([]CrashEvent, 0, len(recentCrashEvents))
	recent = append(recent, recentCrashEvents...)
	crashMu.Unlock()
	return CrashSummary{
		Total:  atomic.LoadUint64(&crashEventsTotal),
		Recent: recent,
	}
}

// recordTabsLostToCrash remembers which tabs a browser death destroyed, so a later
// lookup of one of them can say the crash removed it rather than blaming the id.
// This outlives the browser generation on purpose: the tab belonged to the dead
// browser, so its loss is attributable however many relaunches follow.
func recordTabsLostToCrash(tabIDs []string, ev CrashEvent) {
	crashMu.Lock()
	defer crashMu.Unlock()
	for _, id := range tabIDs {
		if _, seen := tabsLostToCrashes[id]; !seen {
			tabsLostOrder = append(tabsLostOrder, id)
		}
		tabsLostToCrashes[id] = ev
	}
	for len(tabsLostOrder) > maxTabsLostToCrashes {
		delete(tabsLostToCrashes, tabsLostOrder[0])
		tabsLostOrder = tabsLostOrder[1:]
	}
}

// CrashThatDestroyedTab reports the browser death that removed tabID, if any.
func CrashThatDestroyedTab(tabID string) (CrashEvent, bool) {
	crashMu.Lock()
	defer crashMu.Unlock()
	ev, ok := tabsLostToCrashes[tabID]
	return ev, ok
}

// recordBrowserDeath records a whole-browser crash together with every tab it
// held, which is the fact a caller of one of those tabs needs next.
func (b *Bridge) recordBrowserDeath(reason string) CrashEvent {
	event := CrashEvent{Time: time.Now(), Reason: reason}
	recordCrashEventForContext(b.BrowserCtx, event)
	if b.TabManager != nil {
		recordTabsLostToCrash(b.trackedTabIDs(), event)
	}
	return event
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
	tabsLostToCrashes = map[string]CrashEvent{}
	tabsLostOrder = nil
	crashMu.Unlock()
	genMu.Lock()
	generations = map[context.Context]uint64{}
	generationOrder = nil
	genMu.Unlock()
}

// RecordCrashForTests injects a crash event for diagnostics tests.
func RecordCrashForTests(ev CrashEvent) {
	recordCrashEvent(ev)
}

// RecordCrashForContextTests injects a crash event stamped with the generation
// of ctx, so tests can stage crashes on a specific browser context.
func RecordCrashForContextTests(ctx context.Context, ev CrashEvent) {
	recordCrashEventForContext(ctx, ev)
}

// MonitorCrashes listens for browser and tab crashes
func (b *Bridge) MonitorCrashes(handler CrashHandler) {
	if b.BrowserCtx == nil {
		slog.Warn("cannot monitor crashes: no browser context")
		return
	}

	chromedp.ListenTarget(b.BrowserCtx, func(ev interface{}) {
		switch e := ev.(type) {
		case *inspector.EventTargetCrashed:
			event := CrashEvent{
				Time:   time.Now(),
				Reason: "inspector.targetCrashed",
			}
			recordCrashEventForContext(b.BrowserCtx, event)
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
			recordCrashEventForContext(b.BrowserCtx, event)
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

	// Monitor browser context cancellation. A canceled context during
	// draining is expected (deliberate shutdown); otherwise it indicates
	// an unexpected failure such as an in-process GPU crash.
	go func() {
		<-b.BrowserCtx.Done()
		err := b.BrowserCtx.Err()
		if b.draining {
			slog.Info("browser context ended during drain", "reason", err)
			return
		}
		reason := "unexpected context cancellation"
		if err != nil && err != context.Canceled {
			reason = err.Error()
		}
		event := b.recordBrowserDeath(reason)
		slog.Warn("🔥 BROWSER CONTEXT ENDED UNEXPECTEDLY", "error", err)
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

	if WasUncleanExit(b.Config.ProfileDir) {
		logs = append(logs, "Previous session ended with unclean exit (crash detected)")
	}

	return logs
}
