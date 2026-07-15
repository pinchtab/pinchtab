package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/pinchtab/pinchtab/internal/browsers"
	"github.com/pinchtab/pinchtab/internal/config"
)

// The Console domain fallback captures page console output for browsers
// without CapRuntimeConsoleEvents (e.g. CloakBrowser). Those builds natively
// suppress Runtime.consoleAPICalled / Runtime.exceptionThrown after the first
// navigation because Runtime.enable is a bot-detection vector, so the
// chromedp listener in setupConsoleCapture never sees page logs. The
// deprecated CDP Console domain is not patched and keeps delivering
// Console.messageAdded across navigations, so we attach a second, read-only
// CDP session to the tab's target and record from there. The extra session is
// pure CDP — nothing is injected into the page, so it adds no fingerprint
// surface the stealth build tries to remove.
const (
	consoleFallbackDialAttempts = 3
	consoleFallbackDialBackoff  = 500 * time.Millisecond
	consoleFallbackDialTimeout  = 5 * time.Second
)

func (tm *TabManager) runtimeConsoleEventsSupported() bool {
	if tm == nil || tm.config == nil {
		return true
	}
	if b, ok := browsers.Get(config.NormalizeBrowser(tm.config.DefaultBrowser)); ok {
		return b.Capabilities().Has(browsers.CapRuntimeConsoleEvents)
	}
	return true
}

// consoleDomainEndpoint resolves the per-target DevTools WebSocket URL for the
// fallback session: the launched browser's resolved debug port, or the attach
// URL's host when PinchTab attached to an external browser.
func consoleDomainEndpoint(cfg *config.RuntimeConfig, targetID string) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("no runtime config")
	}
	if cfg.BrowserDebugPort > 0 {
		return fmt.Sprintf("ws://127.0.0.1:%d/devtools/page/%s", cfg.BrowserDebugPort, targetID), nil
	}
	if attach := strings.TrimSpace(cfg.CDPAttachURL); attach != "" {
		u, err := url.Parse(attach)
		if err != nil {
			return "", fmt.Errorf("parse cdp attach url: %w", err)
		}
		scheme := "ws"
		if u.Scheme == "wss" || u.Scheme == "https" {
			scheme = "wss"
		}
		return fmt.Sprintf("%s://%s/devtools/page/%s", scheme, u.Host, targetID), nil
	}
	return "", fmt.Errorf("no devtools endpoint available")
}

type consoleDomainMessage struct {
	Source string `json:"source"`
	Level  string `json:"level"`
	Text   string `json:"text"`
	URL    string `json:"url"`
	Line   int64  `json:"line"`
	Column int64  `json:"column"`
}

// parseConsoleDomainEvent extracts the message from a raw Console.messageAdded
// frame. The second return is false for every other CDP frame (command
// replies, other events).
func parseConsoleDomainEvent(data []byte) (consoleDomainMessage, bool) {
	var envelope struct {
		Method string `json:"method"`
		Params struct {
			Message consoleDomainMessage `json:"message"`
		} `json:"params"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return consoleDomainMessage{}, false
	}
	if envelope.Method != "Console.messageAdded" {
		return consoleDomainMessage{}, false
	}
	return envelope.Params.Message, true
}

// recordConsoleDomainMessage mirrors the Runtime-listener semantics: console
// API calls feed the console store, uncaught page errors feed the error
// store, everything else (network/violation/deprecation noise that the
// Runtime path never recorded) is dropped.
func (tm *TabManager) recordConsoleDomainMessage(rawCDPID string, msg consoleDomainMessage) {
	if isInternalConsoleSource(msg.URL) {
		return
	}
	switch msg.Source {
	case "console-api":
		tm.logStore.AddConsoleLog(rawCDPID, LogEntry{
			Timestamp: time.Now(),
			Level:     msg.Level,
			Message:   msg.Text,
			Source:    msg.URL,
		})
	case "javascript":
		if msg.Level != "error" {
			return
		}
		tm.logStore.AddErrorLog(rawCDPID, ErrorEntry{
			Timestamp: time.Now(),
			Message:   msg.Text,
			URL:       msg.URL,
			Line:      msg.Line,
			Column:    msg.Column,
		})
	}
}

// runConsoleDomainFallback owns the side session for one tab: dial, enable
// the Console domain (Chrome replays buffered messages on enable), then pump
// events into the log store until the tab context ends or the socket drops.
func (tm *TabManager) runConsoleDomainFallback(ctx context.Context, rawCDPID string) {
	endpoint, err := consoleDomainEndpoint(tm.config, rawCDPID)
	if err != nil {
		slog.Debug("console domain fallback unavailable", "tab", rawCDPID, "error", err)
		return
	}

	var conn net.Conn
	for attempt := 1; ; attempt++ {
		dialCtx, cancel := context.WithTimeout(ctx, consoleFallbackDialTimeout)
		c, _, _, dialErr := ws.Dial(dialCtx, endpoint)
		cancel()
		if dialErr == nil {
			conn = c
			break
		}
		if attempt >= consoleFallbackDialAttempts || ctx.Err() != nil {
			slog.Warn("console domain fallback dial failed; page console logs will be missing for this tab",
				"tab", rawCDPID, "error", dialErr)
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(consoleFallbackDialBackoff):
		}
	}

	// Unblock the read loop when the tab goes away.
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
		case <-done:
		}
		_ = conn.Close()
	}()

	if err := wsutil.WriteClientText(conn, []byte(`{"id":1,"method":"Console.enable"}`)); err != nil {
		slog.Warn("console domain fallback enable failed", "tab", rawCDPID, "error", err)
		return
	}

	for {
		data, err := wsutil.ReadServerText(conn)
		if err != nil {
			if ctx.Err() == nil {
				slog.Debug("console domain fallback session ended", "tab", rawCDPID, "error", err)
			}
			return
		}
		if msg, ok := parseConsoleDomainEvent(data); ok {
			tm.recordConsoleDomainMessage(rawCDPID, msg)
		}
	}
}
