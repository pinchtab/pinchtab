package activity

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	apiTypes "github.com/pinchtab/pinchtab/internal/api/types"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

func RegisterHandlers(mux *http.ServeMux, rec Recorder) {
	if mux == nil || rec == nil || !rec.Enabled() {
		return
	}

	mux.HandleFunc("GET /api/activity", func(w http.ResponseWriter, r *http.Request) {
		filter, err := filterFromRequest(r)
		if err != nil {
			httpx.ErrorCode(w, http.StatusBadRequest, "bad_filter", err.Error(), false, nil)
			return
		}

		events, err := rec.Query(filter)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, err)
			return
		}

		resp := apiTypes.ActivityLogResponse{
			Events: make([]apiTypes.ActivityLogEvent, 0, len(events)),
			Count:  len(events),
		}
		for _, event := range events {
			resp.Events = append(resp.Events, apiTypes.ActivityLogEvent{
				Timestamp:   event.Timestamp,
				Source:      event.Source,
				RequestID:   event.RequestID,
				SessionID:   event.SessionID,
				ActorID:     event.ActorID,
				AgentID:     event.AgentID,
				Method:      event.Method,
				Path:        event.Path,
				Status:      event.Status,
				DurationMs:  event.DurationMs,
				RemoteAddr:  event.RemoteAddr,
				InstanceID:  event.InstanceID,
				ProfileID:   event.ProfileID,
				ProfileName: event.ProfileName,
				TabID:       event.TabID,
				URL:         event.URL,
				Action:      event.Action,
				Engine:      event.Engine,
				Ref:         event.Ref,
			})
		}

		httpx.JSON(w, http.StatusOK, resp)
	})
}

func filterFromRequest(r *http.Request) (Filter, error) {
	q := r.URL.Query()
	filter := Filter{
		Source:      strings.TrimSpace(q.Get("source")),
		RequestID:   strings.TrimSpace(q.Get("requestId")),
		SessionID:   strings.TrimSpace(q.Get("sessionId")),
		ActorID:     strings.TrimSpace(q.Get("actorId")),
		AgentID:     strings.TrimSpace(q.Get("agentId")),
		InstanceID:  strings.TrimSpace(q.Get("instanceId")),
		ProfileID:   strings.TrimSpace(q.Get("profileId")),
		ProfileName: strings.TrimSpace(q.Get("profileName")),
		TabID:       strings.TrimSpace(q.Get("tabId")),
		Action:      strings.TrimSpace(q.Get("action")),
		Engine:      strings.TrimSpace(q.Get("engine")),
		PathPrefix:  strings.TrimSpace(q.Get("pathPrefix")),
	}

	if limit := strings.TrimSpace(q.Get("limit")); limit != "" {
		n, err := strconv.Atoi(limit)
		if err != nil || n <= 0 {
			return Filter{}, errInvalidQuery("limit")
		}
		filter.Limit = clampQueryLimit(n)
	}
	if ageSec := strings.TrimSpace(q.Get("ageSec")); ageSec != "" {
		n, err := strconv.Atoi(ageSec)
		if err != nil || n < 0 {
			return Filter{}, errInvalidQuery("ageSec")
		}
		filter.Since = time.Now().UTC().Add(-time.Duration(n) * time.Second)
	}
	if since := strings.TrimSpace(q.Get("since")); since != "" {
		t, err := time.Parse(time.RFC3339, since)
		if err != nil {
			return Filter{}, errInvalidQuery("since")
		}
		filter.Since = t.UTC()
	}
	if until := strings.TrimSpace(q.Get("until")); until != "" {
		t, err := time.Parse(time.RFC3339, until)
		if err != nil {
			return Filter{}, errInvalidQuery("until")
		}
		filter.Until = t.UTC()
	}
	return filter, nil
}

type invalidQuery string

func (e invalidQuery) Error() string {
	return "invalid query parameter: " + string(e)
}

func errInvalidQuery(name string) error {
	return invalidQuery(name)
}
