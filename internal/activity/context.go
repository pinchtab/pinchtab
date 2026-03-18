package activity

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/pinchtab/pinchtab/internal/authn"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

const (
	HeaderAgentID     = "X-Agent-Id"
	HeaderPTAgentID   = "X-PinchTab-Agent-Id"
	HeaderPTActorID   = "X-PinchTab-Actor-Id"
	HeaderPTSessionID = "X-PinchTab-Session-Id"
	HeaderPTSource    = "X-PinchTab-Source"
	HeaderPTInstance  = "X-PinchTab-Instance-Id"
	HeaderPTProfileID = "X-PinchTab-Profile-Id"
	HeaderPTProfile   = "X-PinchTab-Profile-Name"
	HeaderPTTabID     = "X-PinchTab-Tab-Id"
)

type requestStateKey struct{}

type requestState struct {
	mu    sync.Mutex
	event Event
}

type Update struct {
	RequestID   string
	SessionID   string
	ActorID     string
	AgentID     string
	InstanceID  string
	ProfileID   string
	ProfileName string
	TabID       string
	URL         string
	Action      string
	Engine      string
	Ref         string
}

func Middleware(rec Recorder, source string, next http.Handler) http.Handler {
	if rec == nil || !rec.Enabled() {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &httpx.StatusWriter{ResponseWriter: w, Code: 200}
		state := &requestState{
			event: Event{
				Timestamp:  start.UTC(),
				Source:     source,
				RequestID:  requestIDFor(r, w),
				ActorID:    actorIDFor(r),
				AgentID:    agentIDFor(r),
				SessionID:  strings.TrimSpace(r.Header.Get(HeaderPTSessionID)),
				Method:     r.Method,
				Path:       r.URL.Path,
				RemoteAddr: remoteAddrFor(r),
				InstanceID: strings.TrimSpace(r.Header.Get(HeaderPTInstance)),
				ProfileID:  strings.TrimSpace(r.Header.Get(HeaderPTProfileID)),
				ProfileName: strings.TrimSpace(
					r.Header.Get(HeaderPTProfile),
				),
				TabID:  initialTabID(r),
				Action: initialAction(r),
				URL:    initialURL(r),
			},
		}

		next.ServeHTTP(sw, r.WithContext(context.WithValue(r.Context(), requestStateKey{}, state)))

		evt := state.snapshot()
		evt.Status = sw.Code
		evt.DurationMs = time.Since(start).Milliseconds()
		if evt.RequestID == "" {
			evt.RequestID = requestIDFor(r, sw)
		}
		if evt.AgentID == "" {
			evt.AgentID = agentIDFor(r)
		}
		if evt.ActorID == "" {
			evt.ActorID = actorIDFor(r)
		}
		if evt.Path == "" {
			evt.Path = r.URL.Path
		}
		if evt.Method == "" {
			evt.Method = r.Method
		}
		_ = rec.Record(evt)
	})
}

func EnrichRequest(r *http.Request, update Update) {
	if r == nil {
		return
	}
	state, _ := r.Context().Value(requestStateKey{}).(*requestState)
	if state == nil {
		return
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	if update.RequestID != "" {
		state.event.RequestID = update.RequestID
	}
	if update.SessionID != "" {
		state.event.SessionID = update.SessionID
	}
	if update.ActorID != "" {
		state.event.ActorID = update.ActorID
	}
	if update.AgentID != "" {
		state.event.AgentID = update.AgentID
	}
	if update.InstanceID != "" {
		state.event.InstanceID = update.InstanceID
	}
	if update.ProfileID != "" {
		state.event.ProfileID = update.ProfileID
	}
	if update.ProfileName != "" {
		state.event.ProfileName = update.ProfileName
	}
	if update.TabID != "" {
		state.event.TabID = update.TabID
	}
	if update.URL != "" {
		state.event.URL = update.URL
	}
	if update.Action != "" {
		state.event.Action = update.Action
	}
	if update.Engine != "" {
		state.event.Engine = update.Engine
	}
	if update.Ref != "" {
		state.event.Ref = update.Ref
	}
}

func PropagateHeaders(ctx context.Context, req *http.Request) {
	if req == nil {
		return
	}
	state, _ := ctx.Value(requestStateKey{}).(*requestState)
	if state == nil {
		return
	}

	evt := state.snapshot()
	if evt.RequestID != "" {
		req.Header.Set("X-Request-Id", evt.RequestID)
	}
	if evt.ActorID != "" {
		req.Header.Set(HeaderPTActorID, evt.ActorID)
	}
	if evt.AgentID != "" {
		req.Header.Set(HeaderPTAgentID, evt.AgentID)
	}
	if evt.SessionID != "" {
		req.Header.Set(HeaderPTSessionID, evt.SessionID)
	}
	if evt.InstanceID != "" {
		req.Header.Set(HeaderPTInstance, evt.InstanceID)
	}
	if evt.ProfileID != "" {
		req.Header.Set(HeaderPTProfileID, evt.ProfileID)
	}
	if evt.ProfileName != "" {
		req.Header.Set(HeaderPTProfile, evt.ProfileName)
	}
	if evt.TabID != "" {
		req.Header.Set(HeaderPTTabID, evt.TabID)
	}
	if evt.Source != "" {
		req.Header.Set(HeaderPTSource, evt.Source)
	}
}

func requestIDFor(r *http.Request, w http.ResponseWriter) string {
	if w != nil {
		if rid := strings.TrimSpace(w.Header().Get("X-Request-Id")); rid != "" {
			return rid
		}
	}
	return strings.TrimSpace(r.Header.Get("X-Request-Id"))
}

func actorIDFor(r *http.Request) string {
	if actorID := strings.TrimSpace(r.Header.Get(HeaderPTActorID)); actorID != "" {
		return actorID
	}
	return FingerprintToken(authn.TokenFromRequest(r))
}

func agentIDFor(r *http.Request) string {
	for _, header := range []string{HeaderPTAgentID, HeaderAgentID, "X-Agent-ID"} {
		if value := strings.TrimSpace(r.Header.Get(header)); value != "" {
			return value
		}
	}
	return ""
}

func remoteAddrFor(r *http.Request) string {
	return authn.ClientIP(r)
}

func (s *requestState) snapshot() Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.event
}

func initialTabID(r *http.Request) string {
	if tabID := strings.TrimSpace(r.Header.Get(HeaderPTTabID)); tabID != "" {
		return tabID
	}
	if tabID := strings.TrimSpace(r.URL.Query().Get("tabId")); tabID != "" {
		return tabID
	}
	path := strings.Trim(r.URL.Path, "/")
	parts := strings.Split(path, "/")
	if len(parts) >= 2 && parts[0] == "tabs" {
		return strings.TrimSpace(parts[1])
	}
	return ""
}

func initialAction(r *http.Request) string {
	if action := strings.TrimSpace(r.URL.Query().Get("kind")); action != "" {
		return action
	}
	switch {
	case r.URL.Path == "/navigate" || strings.HasSuffix(r.URL.Path, "/navigate"):
		return "navigate"
	case r.URL.Path == "/snapshot" || strings.HasSuffix(r.URL.Path, "/snapshot"):
		return "snapshot"
	case r.URL.Path == "/text" || strings.HasSuffix(r.URL.Path, "/text"):
		return "text"
	case r.URL.Path == "/pdf" || strings.HasSuffix(r.URL.Path, "/pdf"):
		return "pdf"
	}
	return ""
}

func initialURL(r *http.Request) string {
	if u := strings.TrimSpace(r.URL.Query().Get("url")); u != "" {
		return u
	}
	return ""
}
