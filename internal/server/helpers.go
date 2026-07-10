package server

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/orchestrator"
	"github.com/pinchtab/pinchtab/internal/proxy"
)

// ProbeHealth is the single transport for /health-style readiness probes: it
// issues a GET with the given timeout and headers and returns the status code +
// body. reachable is false when the server could not be contacted at all.
// Callers interpret the status/body for their own readiness semantics, so the
// CLI's local-status, protected-listener, and background-marker probes share one
// request/client construction instead of drifting per copy.
func ProbeHealth(url string, timeout time.Duration, headers map[string]string) (statusCode int, body []byte, reachable bool) {
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return 0, nil, false
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, false
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b, true
}

func AuthorizationHeaderValue(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	if strings.HasPrefix(token, "ses_") {
		return "Session " + token
	}
	return "Bearer " + token
}

func CheckPinchTabRunning(port, token string) bool {
	headers := map[string]string{}
	if auth := AuthorizationHeaderValue(token); auth != "" {
		headers["Authorization"] = auth
	}
	status, _, reachable := ProbeHealth(fmt.Sprintf("http://localhost:%s/health", port), 500*time.Millisecond, headers)
	return reachable && status == 200
}

// DefaultProxyShorthands is the curated subset of bridge shorthand routes the
// no-instance/default landing proxy forwards to the first running instance. It
// is deliberately narrow (not the full shorthand surface); every entry must be a
// real catalog route, enforced by TestDefaultProxyShorthandsAreCatalogRoutes.
var DefaultProxyShorthands = []string{
	"GET /snapshot", "GET /screenshot", "GET /annotate", "GET /text",
	"POST /navigate", "POST /action", "POST /actions", "POST /evaluate",
	"POST /tab", "POST /lock", "POST /unlock",
	"GET /cookies", "POST /cookies", "DELETE /cookies",
	"GET /download", "POST /upload",
	"GET /stealth/status", "POST /fingerprint/rotate",
	"GET /screencast", "GET /screencast/tabs",
	"POST /find", "POST /macro",
}

func RegisterDefaultProxyRoutes(mux *http.ServeMux, orch *orchestrator.Orchestrator) {
	mux.HandleFunc("GET /tabs", func(w http.ResponseWriter, r *http.Request) {
		target := orch.FirstRunningURL()
		if target == "" {
			httpx.JSON(w, 200, map[string]any{"tabs": []any{}})
			return
		}
		proxy.HTTP(w, r, target+"/tabs")
	})

	for _, ep := range DefaultProxyShorthands {
		endpoint := ep
		mux.HandleFunc(endpoint, func(w http.ResponseWriter, r *http.Request) {
			target := orch.FirstRunningURL()
			if target == "" {
				httpx.Error(w, 503, fmt.Errorf("no running instances — launch one from the Profiles tab"))
				return
			}
			path := r.URL.Path
			proxy.HTTP(w, r, target+path)
		})
	}
}

// ShutdownServer sends POST /shutdown to a running server and waits for it to exit.
func ShutdownServer(port, token string) error {
	client := &http.Client{Timeout: 5 * time.Second}
	url := fmt.Sprintf("http://127.0.0.1:%s/shutdown", port)
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if auth := AuthorizationHeaderValue(token); auth != "" {
		req.Header.Set("Authorization", auth)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("shutdown request: %w", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("shutdown returned HTTP %d", resp.StatusCode)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(300 * time.Millisecond)
		if !CheckPinchTabRunning(port, token) {
			return nil
		}
	}
	return fmt.Errorf("server did not exit within 10s")
}

func MetricFloat(value any) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case uint64:
		return float64(v)
	default:
		return 0
	}
}

func MetricInt(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case uint64:
		return int(v)
	default:
		return 0
	}
}
