package server

import (
	"fmt"
	"net/http"
	"time"

	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/orchestrator"
	"github.com/pinchtab/pinchtab/internal/proxy"
)

func CheckPinchTabRunning(port, token string) bool {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	url := fmt.Sprintf("http://localhost:%s/health", port)
	req, _ := http.NewRequest("GET", url, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	return resp.StatusCode == 200
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

	proxyEndpoints := []string{
		"GET /snapshot", "GET /screenshot", "GET /text",
		"POST /navigate", "POST /action", "POST /actions", "POST /evaluate",
		"POST /tab", "POST /lock", "POST /unlock",
		"GET /cookies", "POST /cookies", "DELETE /cookies",
		"GET /download", "POST /upload",
		"GET /stealth/status", "POST /fingerprint/rotate",
		"GET /screencast", "GET /screencast/tabs",
		"POST /find", "POST /macro",
	}
	for _, ep := range proxyEndpoints {
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
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
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
