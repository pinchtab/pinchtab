package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"
)

var errNoValidatedWebhookIPs = errors.New("no validated callback IPs")

var webhookHTTPTimeout = 10 * time.Second

var dialWebhookAddress = func(ctx context.Context, network, addr string) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, network, addr)
}

func newPinnedWebhookClient(target *validatedCallbackTarget) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		var lastErr error
		for _, ip := range target.IPs {
			conn, err := dialWebhookAddress(ctx, network, net.JoinHostPort(ip.String(), target.Port))
			if err == nil {
				return conn, nil
			}
			lastErr = err
		}
		if lastErr == nil {
			lastErr = errNoValidatedWebhookIPs
		}
		return nil, lastErr
	}
	return &http.Client{
		Timeout:   webhookHTTPTimeout,
		Transport: transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// sendWebhook delivers a task snapshot to the configured callbackUrl.
// Delivery is best-effort: failures are logged but do not affect task state.
func sendWebhook(callbackURL string, t *Task) {
	if callbackURL == "" {
		return
	}

	target, err := validateCallbackTarget(callbackURL)
	if err != nil {
		slog.Warn("webhook: callback rejected", "task", t.ID, "url", callbackURL, "err", err)
		return
	}

	snap := t.Snapshot()
	payload, err := json.Marshal(snap)
	if err != nil {
		slog.Warn("webhook: failed to marshal task", "task", t.ID, "err", err)
		return
	}

	req, err := http.NewRequest(http.MethodPost, target.URL.String(), bytes.NewReader(payload))
	if err != nil {
		slog.Warn("webhook: failed to create request", "task", t.ID, "err", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-PinchTab-Event", "task.completed")
	req.Header.Set("X-PinchTab-Task-ID", snap.ID)

	client := newPinnedWebhookClient(target)
	if transport, ok := client.Transport.(*http.Transport); ok {
		defer transport.CloseIdleConnections()
	}

	resp, err := client.Do(req)
	if err != nil {
		slog.Warn("webhook: delivery failed", "task", t.ID, "url", callbackURL, "err", err)
		return
	}
	// Drain body so the underlying connection can be reused.
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		slog.Warn("webhook: non-success response", "task", t.ID, "url", callbackURL, "status", resp.StatusCode)
		return
	}

	slog.Info("webhook: delivered", "task", t.ID, "url", callbackURL, "status", resp.StatusCode)
}
