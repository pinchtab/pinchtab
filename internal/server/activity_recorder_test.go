package server

import (
	"net/http"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/dashboard"
)

type stubRecorder struct {
	events []activity.Event
}

func (s *stubRecorder) Enabled() bool {
	return true
}

func (s *stubRecorder) Record(evt activity.Event) error {
	s.events = append(s.events, evt)
	return nil
}

func (s *stubRecorder) Query(activity.Filter) ([]activity.Event, error) {
	return nil, nil
}

func TestDashboardActivityRecorderSkipsNonClientEvents(t *testing.T) {
	base := &stubRecorder{}
	dash := dashboard.NewDashboard(nil)
	rec := newDashboardActivityRecorder(base, dash)

	for _, source := range []string{"orchestrator", "server", "bridge", "dashboard"} {
		err := rec.Record(activity.Event{
			Timestamp: time.Now().UTC(),
			Source:    source,
			Method:    http.MethodPost,
			Path:      "/tabs/tab_1/navigate",
			AgentID:   "agent-1",
		})
		if err != nil {
			t.Fatalf("Record(source=%q) error = %v", source, err)
		}
	}
	if len(base.events) != 4 {
		t.Fatalf("base recorder events = %d, want 4", len(base.events))
	}
	if len(dash.RecentEvents()) != 0 {
		t.Fatalf("dashboard recent events = %d, want 0", len(dash.RecentEvents()))
	}
}

func TestDashboardActivityRecorderBroadcastsClientEvents(t *testing.T) {
	base := &stubRecorder{}
	dash := dashboard.NewDashboard(nil)
	rec := newDashboardActivityRecorder(base, dash)

	err := rec.Record(activity.Event{
		RequestID:  "req-1",
		Timestamp:  time.Now().UTC(),
		Source:     "client",
		Method:     http.MethodPost,
		Path:       "/tabs/tab_1/navigate",
		AgentID:    "agent-1",
		Status:     http.StatusOK,
		DurationMs: 12,
	})
	if err != nil {
		t.Fatalf("Record() error = %v", err)
	}
	if len(base.events) != 1 {
		t.Fatalf("base recorder events = %d, want 1", len(base.events))
	}
	if len(dash.RecentEvents()) != 1 {
		t.Fatalf("dashboard recent events = %d, want 1", len(dash.RecentEvents()))
	}
}
