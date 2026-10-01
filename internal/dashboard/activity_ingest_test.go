package dashboard

import (
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/activity"
)

// TestLoadPersistedAgentActivityRestoresScheduledActions drives the restart path
// through a real on-disk store so the Query filter is exercised, not stubbed: a
// scheduled action must survive a restart in the per-agent summary and the recent
// history exactly as a client action does. It fails on HEAD, where the rebuild both
// queried Source:"client" only and tracked Source=="client" only.
func TestLoadPersistedAgentActivityRestoresScheduledActions(t *testing.T) {
	store, err := activity.NewStore(t.TempDir(), 30)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	now := time.Now().UTC()

	events := []activity.Event{
		{
			Timestamp: now.Add(-2 * time.Minute),
			Source:    activity.SourceClient,
			RequestID: "req-client",
			AgentID:   "agent-client",
			Method:    http.MethodPost,
			Path:      "/tabs/tab_1/action",
			Status:    http.StatusOK,
			TabID:     "tab_1",
			Action:    "click",
		},
		{
			Timestamp: now.Add(-1 * time.Minute),
			Source:    activity.SourceScheduler,
			RequestID: "req-sched",
			AgentID:   "agent-sched",
			Method:    http.MethodPost,
			Path:      "/tabs/tab_2/action",
			Status:    http.StatusOK,
			TabID:     "tab_2",
			Action:    "reload",
		},
		{
			Timestamp: now,
			Source:    activity.SourceDashboard,
			RequestID: "req-dash",
			Method:    http.MethodGet,
			Path:      "/agents",
			Status:    http.StatusOK,
		},
	}
	for _, evt := range events {
		if err := store.Record(evt); err != nil {
			t.Fatalf("Record(%s) error = %v", evt.Source, err)
		}
	}

	d := NewDashboard(nil)
	if err := d.LoadPersistedAgentActivity(store); err != nil {
		t.Fatalf("LoadPersistedAgentActivity() error = %v", err)
	}

	agentIDs := map[string]bool{}
	for _, a := range d.Agents() {
		agentIDs[a.ID] = true
	}
	if !agentIDs["agent-sched"] {
		t.Errorf("scheduled action dropped out of the per-agent summary after restart; agents = %v", agentIDs)
	}
	if !agentIDs["agent-client"] {
		t.Errorf("client action missing from the per-agent summary; agents = %v", agentIDs)
	}
	if agentIDs["anonymous"] {
		t.Errorf("dashboard's own request was tracked as agent activity; agents = %v", agentIDs)
	}

	if got := eventRequestIDs(d.RecentEvents()); got != "req-client,req-sched" {
		t.Errorf("RecentEvents() request ids = %q, want %q (scheduled action must be in recent history, dashboard request must not)", got, "req-client,req-sched")
	}
}

// TestLoadPersistedAgentActivityBudgetIsNotSpentOnNonAgentSources guards the rebuild's
// query budget: agent events must not be crowded out of the most-recent window by a
// busy source that is not agent activity. A dashboard left open writes a steady stream
// of dashboard-source events, so a rebuild that queries every source and filters in
// memory afterwards spends its whole budget on them and restores few or no agent events.
func TestLoadPersistedAgentActivityBudgetIsNotSpentOnNonAgentSources(t *testing.T) {
	store, err := activity.NewStore(t.TempDir(), 30)
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	base := time.Now().UTC().Add(-time.Hour)

	agentIDs := map[string]bool{}
	record := func(source, agentID string, ts time.Time) {
		if err := store.Record(activity.Event{
			Timestamp: ts,
			Source:    source,
			AgentID:   agentID,
			Method:    http.MethodPost,
			Path:      "/tabs/tab/action",
			Status:    http.StatusOK,
			TabID:     "tab",
		}); err != nil {
			t.Fatalf("Record(%s) error = %v", source, err)
		}
	}

	for i := range 5 {
		id := "client-" + strconv.Itoa(i)
		agentIDs[id] = true
		record(activity.SourceClient, id, base.Add(time.Duration(i)*time.Second))
	}
	for i := range 3 {
		id := "sched-" + strconv.Itoa(i)
		agentIDs[id] = true
		record(activity.SourceScheduler, id, base.Add(time.Duration(10+i)*time.Second))
	}
	for i := range 1200 {
		record(activity.SourceDashboard, "", base.Add(time.Duration(100+i)*time.Second))
	}

	d := NewDashboard(nil)
	if err := d.LoadPersistedAgentActivity(store); err != nil {
		t.Fatalf("LoadPersistedAgentActivity() error = %v", err)
	}

	got := map[string]bool{}
	for _, a := range d.Agents() {
		got[a.ID] = true
	}
	for id := range agentIDs {
		if !got[id] {
			t.Errorf("agent %q dropped from the rebuild; the budget was spent on non-agent sources. rebuilt = %v", id, got)
		}
	}
	if got["anonymous"] {
		t.Errorf("a non-agent dashboard event was tracked as agent activity; rebuilt = %v", got)
	}
}
