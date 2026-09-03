package activity

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pinchtab/pinchtab/internal/authn"
	"github.com/pinchtab/pinchtab/internal/browserops"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

const (
	HeaderAgentID      = "X-Agent-Id"
	HeaderPTSessionID  = "X-PinchTab-Session-Id"
	HeaderPTSource     = "X-PinchTab-Source"
	HeaderPTInstance   = "X-PinchTab-Instance-Id"
	HeaderPTProfileID  = "X-PinchTab-Profile-Id"
	HeaderPTProfile    = "X-PinchTab-Profile-Name"
	HeaderPTTabID      = "X-PinchTab-Tab-Id"
	HeaderPTTabCreated = "X-PinchTab-Tab-Created"
)

type requestStateKey struct{}

type requestState struct {
	mu    sync.Mutex
	event Event
}

type Update struct {
	RequestID   string
	SessionID   string
	AgentID     string
	InstanceID  string
	ProfileID   string
	ProfileName string
	TabID       string
	URL         string
	Action      string
	Route       *browserops.RouteMetadata
	Ref         string
}

func Middleware(rec Recorder, source string, next http.Handler) http.Handler {
	if rec == nil || !rec.Enabled() {
		return next
	}

	// Recording must never fail a request, so the error is swallowed — but
	// swallowing it silently is how the activity feed goes empty with nothing
	// anywhere saying why. Reported on the TRANSITION in each direction: a fault
	// that lasts warns once rather than once per request, which is the volume at
	// which an operator stops reading it, and the recovery says so too.
	var recordingBroken atomic.Bool

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &httpx.StatusWriter{ResponseWriter: w, Code: 200}
		state := &requestState{
			event: Event{
				Timestamp:  start.UTC(),
				Source:     sourceFor(r, source),
				RequestID:  requestIDFor(r, w),
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
		evt.Code, evt.Error = sw.FailureCode, sw.FailureMessage
		if evt.RequestID == "" {
			evt.RequestID = requestIDFor(r, sw)
		}
		if evt.AgentID == "" {
			evt.AgentID = agentIDFor(r)
		}
		if evt.Path == "" {
			evt.Path = r.URL.Path
		}
		if evt.Method == "" {
			evt.Method = r.Method
		}
		if err := rec.Record(evt); err != nil {
			if recordingBroken.CompareAndSwap(false, true) {
				slog.Warn("activity: recording failed; events are being lost until this recovers", "err", err)
			}
		} else if recordingBroken.CompareAndSwap(true, false) {
			slog.Info("activity: recording recovered")
		}
	})
}

func sourceFor(r *http.Request, fallback string) string {
	if source := strings.TrimSpace(r.Header.Get(HeaderPTSource)); source != "" {
		return source
	}
	creds := authn.CredentialsFromRequest(r)
	if creds.Method == authn.MethodCookie {
		return "dashboard"
	}
	if creds.Method == authn.MethodHeader || creds.Method == authn.MethodSession {
		return "client"
	}
	return fallback
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
		state.event.URL = sanitizeActivityURL(update.URL)
	}
	if update.Action != "" {
		state.event.Action = update.Action
	}
	if update.Route != nil {
		state.event.Route = update.Route
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
		req.Header.Set(httpx.RequestIDHeader, evt.RequestID)
	}
	if evt.AgentID != "" {
		req.Header.Set(HeaderAgentID, evt.AgentID)
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
		if rid := strings.TrimSpace(w.Header().Get(httpx.RequestIDHeader)); rid != "" {
			return rid
		}
	}
	return strings.TrimSpace(r.Header.Get(httpx.RequestIDHeader))
}

func agentIDFor(r *http.Request) string {
	if value := strings.TrimSpace(r.Header.Get(HeaderAgentID)); value != "" {
		return value
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

// replayedBody hands the downstream handler the bytes already read followed by the
// rest of the stream, and closes the original. It is a ReadCloser rather than a
// NopCloser over a buffer because the request body is not fully in memory: only the
// peek is.
type replayedBody struct {
	io.Reader
	io.Closer
}

func initialURL(r *http.Request) string {
	if u := strings.TrimSpace(r.URL.Query().Get("url")); u != "" {
		return sanitizeActivityURL(u)
	}
	return ""
}

// activityPeekBytes bounds what the enrichment reads from a request body. It is a
// budget for THIS function's parse, never a limit on the request: the peeked bytes
// are put back in front of the rest, so the handler always sees every byte the
// client sent. Substituting the peek for the body truncated an oversize action or
// navigate payload and then reported the client's own JSON as invalid.
const activityPeekBytes = 8 << 10

// EnrichRouteActivity peeks at the request body for action and navigate
// requests to extract kind, ref, and url for the activity stream. The body is
// restored in full for the downstream handler: what was read is replayed ahead of
// whatever is still unread, so enrichment costs the request nothing. A payload
// whose first activityPeekBytes are not a complete JSON object simply enriches
// nothing — the activity row loses a field, the request keeps its bytes.
func EnrichRouteActivity(r *http.Request) {
	if r == nil || r.Body == nil || r.Method != http.MethodPost {
		return
	}
	path := r.URL.Path
	isAction := path == "/action" || strings.HasSuffix(path, "/action")
	isNavigate := path == "/navigate" || strings.HasSuffix(path, "/navigate")
	if !isAction && !isNavigate {
		return
	}

	original := r.Body
	peeked, err := io.ReadAll(io.LimitReader(original, activityPeekBytes))
	r.Body = replayedBody{
		Reader: io.MultiReader(bytes.NewReader(peeked), original),
		Closer: original,
	}
	if err != nil || len(peeked) == 0 {
		return
	}
	body := peeked

	var peek struct {
		Kind string `json:"kind"`
		Ref  string `json:"ref"`
		URL  string `json:"url"`
	}
	if json.Unmarshal(body, &peek) != nil {
		return
	}

	update := Update{}
	if isAction && peek.Kind != "" {
		update.Action = peek.Kind
		update.Ref = peek.Ref
	}
	if isNavigate && peek.URL != "" {
		update.Action = "navigate"
		update.URL = peek.URL
	}
	if update.Action != "" {
		EnrichRequest(r, update)
	}
}
