package server

import (
	"net/http"
	"strings"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/dashboard"
)

type dashboardActivityRecorder struct {
	base activity.Recorder
	dash *dashboard.Dashboard
}

func newDashboardActivityRecorder(base activity.Recorder, dash *dashboard.Dashboard) activity.Recorder {
	return dashboardActivityRecorder{base: base, dash: dash}
}

func (r dashboardActivityRecorder) Enabled() bool {
	if r.base != nil && r.base.Enabled() {
		return true
	}
	return r.dash != nil
}

func (r dashboardActivityRecorder) Record(evt activity.Event) error {
	if r.base != nil && r.base.Enabled() {
		if err := r.base.Record(evt); err != nil {
			return err
		}
	}
	if r.dash != nil && shouldBroadcastDashboardActivity(evt) {
		r.dash.RecordActivityEvent(evt)
	}
	return nil
}

func (r dashboardActivityRecorder) Query(filter activity.Filter) ([]activity.Event, error) {
	if r.base == nil {
		return []activity.Event{}, nil
	}
	return r.base.Query(filter)
}

func shouldBroadcastDashboardActivity(evt activity.Event) bool {
	if !strings.EqualFold(strings.TrimSpace(evt.Source), "orchestrator") {
		return true
	}
	if evt.Method != http.MethodGet {
		return true
	}
	switch evt.Path {
	case "/health", "/tabs", "/metrics", "/api/metrics":
		return false
	default:
		return true
	}
}
