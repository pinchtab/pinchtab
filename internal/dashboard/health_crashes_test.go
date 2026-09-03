package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
)

type crashedInstances struct {
	instances []bridge.Instance
	crashes   bridge.CrashSummary
}

func (s crashedInstances) List() []bridge.Instance           { return s.instances }
func (s crashedInstances) CrashSummary() bridge.CrashSummary { return s.crashes }

type plainInstances struct{ instances []bridge.Instance }

func (s plainInstances) List() []bridge.Instance { return s.instances }

func healthBody(t *testing.T, instances InstanceLister) map[string]any {
	t.Helper()
	api := newConfigAPIForTest(config.Load(), instances, nil, nil, nil, "test", time.Now())
	w := httptest.NewRecorder()
	api.HandleHealth(w, httptest.NewRequest(http.MethodGet, "/health", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return body
}

// A crashed-then-relaunched instance is running again, so status stays ok; the
// crash is history and rides beside it under the same key bridge /health uses.
func TestServerModeHealthCarriesTheInstancesCrashesBesideAnOkStatus(t *testing.T) {
	running := []bridge.Instance{{ID: "inst_1", Status: "running"}}
	body := healthBody(t, crashedInstances{instances: running, crashes: bridge.CrashSummary{
		Total:  1,
		Recent: []bridge.CrashEvent{{Time: time.Now(), Reason: "unexpected context cancellation", InstanceID: "inst_1"}},
	}})
	if body["status"] != "ok" {
		t.Errorf("status = %v, want ok: a relaunched instance is serving", body["status"])
	}
	crashes, ok := body["crashes"].(map[string]any)
	if !ok {
		t.Fatalf("health has no crashes block: %v", body)
	}
	if crashes["total"] != float64(1) {
		t.Errorf("crashes.total = %v, want 1", crashes["total"])
	}
	recent, _ := crashes["recent"].([]any)
	if len(recent) != 1 {
		t.Fatalf("crashes.recent = %v, want one event", crashes["recent"])
	}
	if event, _ := recent[0].(map[string]any); event["instanceId"] != "inst_1" || event["reason"] != "unexpected context cancellation" {
		t.Errorf("event = %v, want it to name the instance and the reason", event)
	}

	clean := healthBody(t, crashedInstances{instances: running})
	if _, present := clean["crashes"]; present {
		t.Errorf("a never-crashed instance grew a crashes key: %v", clean["crashes"])
	}
	if _, present := healthBody(t, plainInstances{instances: running})["crashes"]; present {
		t.Error("a lister that cannot report crashes produced a crashes key")
	}
}
