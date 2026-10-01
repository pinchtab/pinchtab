package orchestrator

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/dashboard"
)

func TestEffectiveInstanceStatus(t *testing.T) {
	cases := []struct {
		name   string
		status bridge.InstanceStatus
		active bool
		want   bridge.InstanceStatus
	}{
		{"live but stored stopped -> running", "stopped", true, "running"},
		{"dead starting -> stopped", "starting", false, "stopped"},
		{"dead running -> stopped", "running", false, "stopped"},
		{"dead stopping -> stopped", "stopping", false, "stopped"},
		{"live running passes through", "running", true, "running"},
		{"dead stopped passes through", "stopped", false, "stopped"},
		{"unknown status passes through (live)", "errored", true, "errored"},
		{"unknown status passes through (dead)", "errored", false, "errored"},
	}
	for _, c := range cases {
		if got := effectiveInstanceStatus(c.status, c.active); got != c.want {
			t.Errorf("%s: effectiveInstanceStatus(%q, %v) = %q, want %q", c.name, c.status, c.active, got, c.want)
		}
	}
}

func registerInstance(o *Orchestrator, id string, status bridge.InstanceStatus, browser string, start time.Time) {
	url := "http://" + id + ".local"
	inst := &InstanceInternal{
		Instance: bridge.Instance{ID: id, Status: status, URL: url, Browser: browser, StartTime: start},
		URL:      url,
	}
	if status != "stopped" {
		inst.cmd = &mockCmd{pid: 1, isAlive: true}
	}
	o.instances[id] = inst
}

func listIDs(o *Orchestrator) []string {
	var ids []string
	for _, inst := range o.List() {
		ids = append(ids, inst.ID)
	}
	return ids
}

func okClient() *http.Client {
	return &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"status":"ok"}`)),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})}
}

func healthDefaultInstanceID(t *testing.T, o *Orchestrator) string {
	t.Helper()
	api := dashboard.NewConfigAPI(o.LiveConfig(), o, nil, nil, nil, "test", time.Now())
	w := httptest.NewRecorder()
	api.HandleHealth(w, httptest.NewRequest(http.MethodGet, "/health", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("GET /health = %d: %s", w.Code, w.Body.String())
	}
	var body struct {
		DefaultInstance *struct {
			ID string `json:"id"`
		} `json:"defaultInstance"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode /health: %v", err)
	}
	if body.DefaultInstance == nil {
		t.Fatalf("/health omitted defaultInstance: %s", w.Body.String())
	}
	return body.DefaultInstance.ID
}

func shorthandRouteInstanceID(t *testing.T, o *Orchestrator) string {
	t.Helper()
	target, status, err := o.RouteForRequest(httptest.NewRequest(http.MethodGet, "/text", nil))
	if err != nil {
		t.Fatalf("RouteForRequest status=%d err=%v", status, err)
	}
	for _, inst := range o.List() {
		if inst.URL == target {
			return inst.ID
		}
	}
	t.Fatalf("shorthand route target %q matches no instance", target)
	return ""
}

func TestListOrdersInstancesByStartTimeThenIDWhateverTheMapOrder(t *testing.T) {
	alwaysAlive(t)
	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	base := time.Now()
	registerInstance(o, "inst_z", "running", "", base)
	registerInstance(o, "inst_m", "running", "", base.Add(2*time.Second))
	registerInstance(o, "inst_b", "stopped", "", base)
	registerInstance(o, "inst_y", "running", "", base.Add(time.Second))
	registerInstance(o, "inst_a", "running", "", base.Add(2*time.Second))
	registerInstance(o, "inst_c", "running", "", base.Add(-time.Second))

	want := []string{"inst_c", "inst_b", "inst_z", "inst_y", "inst_a", "inst_m"}
	for i := 0; i < 50; i++ {
		if got := listIDs(o); !slices.Equal(got, want) {
			t.Fatalf("call %d: List() order = %v, want %v", i, got, want)
		}
	}
}

func TestHealthDefaultInstanceIsTheInstanceShorthandRoutesUse(t *testing.T) {
	alwaysAlive(t)
	cases := []struct {
		name string
		cfg  *config.RuntimeConfig
		want string
	}{
		{name: "earliest running instance without a default target", cfg: &config.RuntimeConfig{}, want: "inst_z"},
		{name: "default target browser even when another started first", cfg: &config.RuntimeConfig{
			DefaultTarget: "cloak",
			Targets: config.BrowserTargetsConfig{
				"chrome": {Provider: config.BrowserChrome},
				"cloak":  {Provider: config.BrowserCloak},
			},
		}, want: "inst_a"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
			o.ApplyRuntimeConfig(tc.cfg)
			o.client = okClient()
			base := time.Now()
			registerInstance(o, "inst_0", "stopped", config.BrowserChrome, base.Add(-time.Minute))
			registerInstance(o, "inst_z", "running", config.BrowserChrome, base)
			registerInstance(o, "inst_a", "running", config.BrowserCloak, base.Add(time.Second))

			routed := shorthandRouteInstanceID(t, o)
			if routed != tc.want {
				t.Fatalf("shorthand route instance = %q, want %q", routed, tc.want)
			}
			for i := 0; i < 20; i++ {
				if got := healthDefaultInstanceID(t, o); got != routed {
					t.Fatalf("call %d: /health defaultInstance = %q, want the shorthand route instance %q", i, got, routed)
				}
			}
		})
	}
}

func TestHealthDefaultInstanceFallsBackToTheFirstListedWhenNoneIsRoutable(t *testing.T) {
	alwaysAlive(t)
	o := NewOrchestratorWithRunner(t.TempDir(), &mockRunner{portAvail: true})
	o.client = okClient()
	base := time.Now()
	registerInstance(o, "inst_z", "starting", "", base)
	registerInstance(o, "inst_a", "starting", "", base.Add(time.Second))

	if _, ok := o.DefaultInstance(); ok {
		t.Fatal("DefaultInstance reported a routable instance while every instance is still starting")
	}
	for i := 0; i < 20; i++ {
		if got := healthDefaultInstanceID(t, o); got != "inst_z" {
			t.Fatalf("call %d: /health defaultInstance = %q, want the earliest-started inst_z", i, got)
		}
	}
}
