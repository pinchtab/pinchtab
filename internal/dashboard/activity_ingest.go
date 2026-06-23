package dashboard

import (
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/activity"
	apiTypes "github.com/pinchtab/pinchtab/internal/api/types"
)

const persistedAgentBootstrapLimit = 1000

// RecordActivityEvent converts a backend activity record into a live tool-call event.
func (d *Dashboard) RecordActivityEvent(evt activity.Event) {
	d.recordEvents([]apiTypes.ActivityEvent{activityEventToLiveEvent(evt)})
}

func activityEventToLiveEvent(evt activity.Event) apiTypes.ActivityEvent {
	details := map[string]any{
		"status":     evt.Status,
		"durationMs": evt.DurationMs,
	}
	if evt.Source != "" {
		details["source"] = evt.Source
	}
	if evt.RequestID != "" {
		details["requestId"] = evt.RequestID
	}
	if evt.SessionID != "" {
		details["sessionId"] = evt.SessionID
	}
	if evt.InstanceID != "" {
		details["instanceId"] = evt.InstanceID
	}
	if evt.ProfileID != "" {
		details["profileId"] = evt.ProfileID
	}
	if evt.ProfileName != "" {
		details["profileName"] = evt.ProfileName
	}
	if evt.TabID != "" {
		details["tabId"] = evt.TabID
	}
	if evt.URL != "" {
		details["url"] = evt.URL
	}
	if evt.Ref != "" {
		details["ref"] = evt.Ref
	}
	if evt.Action != "" {
		details["action"] = evt.Action
	}

	return apiTypes.ActivityEvent{
		ID:        evt.RequestID,
		AgentID:   agentIDOrAnonymous(evt.AgentID),
		Channel:   "tool_call",
		Type:      classifyActivityType(evt),
		Method:    evt.Method,
		Path:      evt.Path,
		Timestamp: evt.Timestamp,
		Details:   details,
	}
}

// RecordEvent records an activity event, updates the live agent summary, and broadcasts to SSE subscribers.
func (d *Dashboard) RecordEvent(evt apiTypes.ActivityEvent) {
	d.recordEvents([]apiTypes.ActivityEvent{evt})
}

func (d *Dashboard) upsertAgentLocked(evt apiTypes.ActivityEvent) {
	agentID := agentIDOrAnonymous(evt.AgentID)
	agent, ok := d.agents[agentID]
	if !ok {
		agent = &apiTypes.Agent{
			ID:          agentID,
			Name:        agentID,
			ConnectedAt: evt.Timestamp,
		}
		d.agents[agentID] = agent
	}
	agent.LastActivity = evt.Timestamp
	agent.RequestCount++
}

// IngestPersistedAgentActivity loads new agent-tagged requests from the shared
// activity store into the live dashboard cache.
func (d *Dashboard) IngestPersistedAgentActivity(rec activity.Recorder, since time.Time) (time.Time, error) {
	if d == nil || rec == nil || !rec.Enabled() {
		return since, nil
	}

	events, err := rec.Query(activity.Filter{
		Source: "client",
		Since:  since,
		Limit:  persistedAgentBootstrapLimit,
	})
	if err != nil {
		return since, err
	}

	latest := since
	for _, evt := range events {
		if evt.Timestamp.After(latest) {
			latest = evt.Timestamp
		}
	}
	d.ingestActivityBatch(events)

	return latest, nil
}

// ingestActivityBatch filters persisted activity to the trackable client subset,
// converts to live events, and records them.
func (d *Dashboard) ingestActivityBatch(events []activity.Event) {
	batch := make([]apiTypes.ActivityEvent, 0, len(events))
	for _, evt := range events {
		if !shouldTrackPersistedAgentActivity(evt) {
			continue
		}
		batch = append(batch, activityEventToLiveEvent(evt))
	}
	d.recordEvents(batch)
}

// IngestTail reads only newly-appended events from the tail reader, avoiding
// a full file rescan on each tick.
func (d *Dashboard) IngestTail(tr *activity.TailReader) (int, error) {
	if d == nil || tr == nil {
		return 0, nil
	}

	events, err := tr.Read(persistedAgentBootstrapLimit)
	if err != nil {
		return 0, err
	}

	d.ingestActivityBatch(events)

	return len(events), nil
}

// LoadPersistedAgentActivity rebuilds the in-memory agent summaries and recent
// tool-call history from the persisted activity log on server startup.
func (d *Dashboard) LoadPersistedAgentActivity(rec activity.Recorder) error {
	_, err := d.IngestPersistedAgentActivity(rec, time.Time{})
	return err
}

func shouldTrackPersistedAgentActivity(evt activity.Event) bool {
	return evt.Source == "client"
}

func (d *Dashboard) rememberEventIDLocked(id string) {
	if id == "" {
		return
	}
	d.seenEventIDs[id] = struct{}{}
	if evicted, didEvict := d.seenEventOrder.push(id); didEvict {
		delete(d.seenEventIDs, evicted)
	}
}

func normalizeEvent(evt apiTypes.ActivityEvent) apiTypes.ActivityEvent {
	if evt.ID == "" {
		evt.ID = generateID()
	}
	if evt.Timestamp.IsZero() {
		evt.Timestamp = time.Now().UTC()
	}
	if evt.Channel == "" {
		evt.Channel = "tool_call"
	}
	if evt.AgentID == "" {
		evt.AgentID = "anonymous"
	}
	return evt
}

func (d *Dashboard) recordEvents(events []apiTypes.ActivityEvent) {
	if d == nil || len(events) == 0 {
		return
	}

	d.mu.Lock()
	broadcast := d.cacheActivityEventsLocked(events)
	chans := d.activitySubscribersLocked()
	d.mu.Unlock()

	broadcastActivityEvents(chans, broadcast)
}

// cacheActivityEventsLocked normalizes + dedupes events, updates agent summaries
// and the recent-event ring, and returns the newly-accepted events to broadcast.
// Caller must hold d.mu.
func (d *Dashboard) cacheActivityEventsLocked(events []apiTypes.ActivityEvent) []apiTypes.ActivityEvent {
	broadcast := make([]apiTypes.ActivityEvent, 0, len(events))
	for _, raw := range events {
		evt := normalizeEvent(raw)
		if _, ok := d.seenEventIDs[evt.ID]; ok {
			continue
		}
		d.rememberEventIDLocked(evt.ID)
		d.upsertAgentLocked(evt)
		d.recentEvents.push(evt)
		broadcast = append(broadcast, evt)
	}
	return broadcast
}

// activitySubscribersLocked snapshots the current SSE subscriber channels.
// Caller must hold d.mu.
func (d *Dashboard) activitySubscribersLocked() []chan apiTypes.ActivityEvent {
	chans := make([]chan apiTypes.ActivityEvent, 0, len(d.activityConns))
	for ch := range d.activityConns {
		chans = append(chans, ch)
	}
	return chans
}

// broadcastActivityEvents non-blockingly fans out events to each subscriber,
// dropping when a subscriber's buffer is full.
func broadcastActivityEvents(chans []chan apiTypes.ActivityEvent, events []apiTypes.ActivityEvent) {
	for _, evt := range events {
		for _, ch := range chans {
			select {
			case ch <- evt:
			default:
			}
		}
	}
}

func classifyActivityType(evt activity.Event) string {
	if evt.Action != "" {
		switch evt.Action {
		case "navigate", "snapshot", "screenshot", "text":
			return evt.Action
		default:
			return "action"
		}
	}
	path := evt.Path
	switch {
	case strings.Contains(path, "/navigate"):
		return "navigate"
	case strings.Contains(path, "/snapshot"):
		return "snapshot"
	case strings.Contains(path, "/screenshot"):
		return "screenshot"
	case strings.Contains(path, "/text"):
		return "text"
	case strings.Contains(path, "/action"):
		return "action"
	default:
		return "other"
	}
}

func agentIDOrAnonymous(agentID string) string {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return "anonymous"
	}
	return agentID
}
