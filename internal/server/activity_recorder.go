package server

import (
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

// Record fans one event out to two INDEPENDENT sinks: the on-disk store and the
// dashboard's live feed. Returning on the store's error skipped the broadcast, so
// a disk fault took the live feed — which touches no disk — dark with it. Each
// sink is now attempted regardless of the other, and the caller is told what
// failed rather than only that something did.
func (r dashboardActivityRecorder) Record(evt activity.Event) error {
	var err error
	if r.base != nil && r.base.Enabled() {
		err = r.base.Record(evt)
	}
	if r.dash != nil && shouldBroadcastDashboardActivity(evt) {
		r.dash.RecordActivityEvent(evt)
	}
	return err
}

func (r dashboardActivityRecorder) Query(filter activity.Filter) ([]activity.Event, error) {
	if r.base == nil {
		return []activity.Event{}, nil
	}
	return r.base.Query(filter)
}

func shouldBroadcastDashboardActivity(evt activity.Event) bool {
	return evt.Source == "client"
}
