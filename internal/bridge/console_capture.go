package bridge

import (
	"context"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// setupConsoleCapture enables runtime domain and listens for console/exception events.
func (tm *TabManager) setupConsoleCapture(ctx context.Context, rawCDPID string) {
	if tm.logStore == nil {
		return
	}

	execContextSources := make(map[runtime.ExecutionContextID]string)
	var execContextMu sync.RWMutex

	// Browsers whose native stealth patches suppress Runtime domain events
	// (no CapRuntimeConsoleEvents) record through the Console domain side
	// session instead; the chromedp listener below must then stay silent so
	// the buffered replay Runtime.enable still emits is not double-recorded.
	recordViaRuntime := tm.runtimeConsoleEventsSupported()

	chromedp.ListenTarget(ctx, func(ev any) {
		switch ev := ev.(type) {
		case *runtime.EventExecutionContextCreated:
			if ev.Context == nil {
				return
			}
			execContextMu.Lock()
			execContextSources[ev.Context.ID] = executionContextSource(ev.Context)
			execContextMu.Unlock()

		case *runtime.EventExecutionContextDestroyed:
			execContextMu.Lock()
			delete(execContextSources, ev.ExecutionContextID)
			execContextMu.Unlock()

		case *runtime.EventExecutionContextsCleared:
			execContextMu.Lock()
			clear(execContextSources)
			execContextMu.Unlock()

		case *runtime.EventConsoleAPICalled:
			if !recordViaRuntime {
				return
			}
			var msg string
			for _, arg := range ev.Args {
				if len(arg.Value) > 0 {
					val := string(arg.Value)
					if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
						val = val[1 : len(val)-1]
					}
					msg += val
				} else if arg.Description != "" {
					msg += arg.Description
				}
				msg += " "
			}

			var ts time.Time
			if ev.Timestamp != nil {
				ts = time.Time(*ev.Timestamp)
			} else {
				ts = time.Now()
			}

			source := stackTraceSource(ev.StackTrace)
			if source == "" {
				execContextMu.RLock()
				source = execContextSources[ev.ExecutionContextID]
				execContextMu.RUnlock()
			}
			if source == "" {
				source = strings.TrimSpace(ev.Context)
			}
			if isInternalConsoleSource(source) {
				return
			}

			tm.logStore.AddConsoleLog(rawCDPID, LogEntry{
				Timestamp: ts,
				Level:     string(ev.Type),
				Message:   msg,
				Source:    source,
			})

		case *runtime.EventExceptionThrown:
			if !recordViaRuntime {
				return
			}
			msg := ev.ExceptionDetails.Text
			if ev.ExceptionDetails.Exception != nil && ev.ExceptionDetails.Exception.Description != "" {
				msg += ": " + ev.ExceptionDetails.Exception.Description
			}

			var ts time.Time
			if ev.Timestamp != nil {
				ts = time.Time(*ev.Timestamp)
			} else {
				ts = time.Now()
			}

			source := exceptionSource(ev.ExceptionDetails)
			if source == "" {
				execContextMu.RLock()
				source = execContextSources[ev.ExceptionDetails.ExecutionContextID]
				execContextMu.RUnlock()
			}
			if isInternalConsoleSource(source) {
				return
			}

			stack := ""
			if ev.ExceptionDetails.Exception != nil {
				stack = ev.ExceptionDetails.Exception.Description
			}

			tm.logStore.AddErrorLog(rawCDPID, ErrorEntry{
				Timestamp: ts,
				Message:   msg,
				URL:       ev.ExceptionDetails.URL,
				Line:      ev.ExceptionDetails.LineNumber,
				Column:    ev.ExceptionDetails.ColumnNumber,
				Stack:     stack,
			})
		}
	})

	// Enable the Runtime domain synchronously before returning. Doing this in
	// a detached goroutine raced page load: under CPU contention the page's
	// synchronous console.* calls fired before Runtime.enable landed, and
	// Chrome only dispatches consoleAPICalled events after the domain is
	// enabled — so early console/exception entries were silently dropped.
	// Bounded so a wedged target cannot stall tab creation.
	enableCtx, cancel := context.WithTimeout(ctx, runtimeEnableTimeout)
	defer cancel()
	_ = chromedp.Run(enableCtx, chromedp.ActionFunc(func(c context.Context) error {
		return runtime.Enable().Do(c)
	}))

	if !recordViaRuntime {
		go tm.runConsoleDomainFallback(ctx, rawCDPID)
	}
}

// runtimeEnableTimeout bounds the synchronous Runtime.enable in
// setupConsoleCapture; enabling normally takes a couple of milliseconds.
const runtimeEnableTimeout = 5 * time.Second

func (tm *TabManager) shouldEagerlyCaptureConsole() bool {
	if tm == nil || tm.config == nil {
		return true
	}
	return !strings.EqualFold(strings.TrimSpace(tm.config.StealthLevel), "full")
}

func (tm *TabManager) EnsureConsoleCapture(tabID string) {
	if tm == nil || tm.logStore == nil {
		return
	}

	tm.mu.Lock()
	entry := tm.tabs[tabID]
	if entry == nil && tabID == "" {
		entry = tm.tabs[tm.currentTab]
	}
	if entry == nil || entry.Ctx == nil || entry.ConsoleCaptureEnabled {
		tm.mu.Unlock()
		return
	}
	entry.ConsoleCaptureEnabled = true
	ctx := entry.Ctx
	rawCDPID := entry.CDPID
	tm.mu.Unlock()

	tm.setupConsoleCapture(ctx, rawCDPID)
}

func executionContextSource(ctx *runtime.ExecutionContextDescription) string {
	if ctx == nil {
		return ""
	}
	if source := strings.TrimSpace(ctx.Origin); source != "" {
		return source
	}
	return strings.TrimSpace(ctx.Name)
}

func exceptionSource(details *runtime.ExceptionDetails) string {
	if details == nil {
		return ""
	}
	if source := strings.TrimSpace(details.URL); source != "" {
		return source
	}
	return stackTraceSource(details.StackTrace)
}

func stackTraceSource(trace *runtime.StackTrace) string {
	for trace != nil {
		for _, frame := range trace.CallFrames {
			if frame == nil {
				continue
			}
			if source := strings.TrimSpace(frame.URL); source != "" {
				return source
			}
		}
		trace = trace.Parent
	}
	return ""
}

func isInternalConsoleSource(source string) bool {
	source = strings.TrimSpace(source)
	if source == "" {
		return false
	}

	lower := strings.ToLower(source)
	switch {
	case strings.HasPrefix(lower, "chrome-extension://"),
		strings.HasPrefix(lower, "edge-extension://"),
		strings.HasPrefix(lower, "moz-extension://"),
		strings.HasPrefix(lower, "safari-extension://"),
		strings.HasPrefix(lower, "devtools://"),
		strings.HasPrefix(lower, "chrome://"),
		strings.HasPrefix(lower, "about:"):
		return true
	}

	parsed, err := url.Parse(source)
	if err != nil {
		return false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "chrome-extension", "edge-extension", "moz-extension", "safari-extension", "devtools", "chrome", "about":
		return true
	default:
		return false
	}
}
