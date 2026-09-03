package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/dashboard"
	"github.com/pinchtab/pinchtab/internal/orchestrator"
)

// The wiring under test is RunDashboard's: one publication point reaches the
// orchestrator and the config API, and a dashboard save must not write the value
// the orchestrator's goroutines are reading. The reader runs for the whole of
// every save rather than being sampled once, so -race has a write and a read to
// pair up on every iteration instead of on a lucky one.
func TestADashboardSaveDoesNotRaceAnOrchestratorReader(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("PINCHTAB_CONFIG", configPath)
	data, err := json.MarshalIndent(config.DefaultFileConfig(), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	orch := orchestrator.NewOrchestrator(t.TempDir())
	orch.ApplyRuntimeConfig(config.Load())
	api := dashboard.NewConfigAPI(orch.LiveConfig(), nil, nil, orch, nil, "test", time.Now())

	stop := make(chan struct{})
	var readers sync.WaitGroup
	readers.Add(1)
	go func() {
		defer readers.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			// The same reads the off-request goroutines make: the security
			// policy and the child bind address the startup probe follows.
			_ = orch.AllowsEvaluate()
			_ = orch.AllowsMacro()
			_ = orch.AllowsDownload()
		}
	}()

	for i := 0; i < 25; i++ {
		payload := config.DefaultFileConfig()
		macro, download := i%2 == 0, i%2 == 1
		payload.Security.AllowMacro = &macro
		payload.Security.AllowDownload = &download
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		api.HandlePutConfig(w, httptest.NewRequest(http.MethodPut, "/api/config", bytes.NewReader(body)))
		if w.Code != http.StatusOK {
			t.Fatalf("save %d returned %d: %s", i, w.Code, w.Body.String())
		}
	}

	close(stop)
	readers.Wait()
}
