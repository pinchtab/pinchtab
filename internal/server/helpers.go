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
		"POST /tab", "POST /tab/lock", "POST /tab/unlock",
		"GET /cookies", "POST /cookies",
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
