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

func TestDashboardActivityRecorderSkipsInternalOrchestratorMonitoringEvents(t *testing.T) {
	base := &stubRecorder{}
	dash := dashboard.NewDashboard(nil)
	rec := newDashboardActivityRecorder(base, dash)

	err := rec.Record(activity.Event{
		Timestamp: time.Now().UTC(),
		Source:    "orchestrator",
		Method:    http.MethodGet,
		Path:      "/tabs",
	})
	if err != nil {
		t.Fatalf("Record() error = %v", err)
	}
	if len(base.events) != 1 {
		t.Fatalf("base recorder events = %d, want 1", len(base.events))
	}
	if len(dash.RecentEvents()) != 0 {
		t.Fatalf("dashboard recent events = %d, want 0", len(dash.RecentEvents()))
	}
}

func TestDashboardActivityRecorderBroadcastsNonMonitoringEvents(t *testing.T) {
	base := &stubRecorder{}
	dash := dashboard.NewDashboard(nil)
	rec := newDashboardActivityRecorder(base, dash)

	err := rec.Record(activity.Event{
		RequestID:  "req-1",
		Timestamp:  time.Now().UTC(),
		Source:     "orchestrator",
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
