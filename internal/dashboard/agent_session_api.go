package dashboard

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/authn"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/remedy"
	"github.com/pinchtab/pinchtab/internal/session"
)

// SessionAPI handles CRUD operations for sessions.
type SessionAPI struct {
	store             *session.Store
	browsersAvailable []string
	sessionTabIDs     func(string) []string
}

// SetSessionTabSource wires the orchestrator's successful-creation ledger into
// revoke responses without coupling the dashboard package to the orchestrator.
func (a *SessionAPI) SetSessionTabSource(source func(string) []string) {
	if a != nil {
		a.sessionTabIDs = source
	}
}

// NewSessionAPI creates a new session API handler.
func NewSessionAPI(store *session.Store, browsersAvailable []string) *SessionAPI {
	return &SessionAPI{store: store, browsersAvailable: browsersAvailable}
}

// RegisterHandlers registers session API routes. It walks session.RoutePatterns() rather
// than naming the patterns here, so this registration and the unavailable-mode ones in the
// server package cannot disagree about what the family contains. A pattern with no handler
// panics: leaving it unrouted here is exactly the bare-404 state the shared list prevents.
func (a *SessionAPI) RegisterHandlers(mux *http.ServeMux) {
	if a == nil || a.store == nil || !a.store.Enabled() {
		return
	}
	a.registerPatterns(mux, session.RoutePatterns())
}

func (a *SessionAPI) registerPatterns(mux *http.ServeMux, patterns []string) {
	for _, pattern := range patterns {
		handler := a.handlerFor(pattern)
		if handler == nil {
			panic("dashboard: no session handler bound for " + pattern)
		}
		mux.HandleFunc(pattern, a.whileEnabled(handler))
	}
}

// whileEnabled re-reads the store on every request, so a save that switches agent
// sessions off is refused by routes a boot with them on already mounted. The code
// and message are the never-mounted state's; the guidance is not, because this
// state is the one a config set alone reverses.
func (a *SessionAPI) whileEnabled(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.store == nil || !a.store.Enabled() {
			hint, r := session.DisabledGuidance(a.store.DisabledBy(), true)
			httpx.ErrorCode(w, http.StatusNotFound, session.CodeDisabled, session.MsgDisabled, false,
				remedy.Details(hint, r))
			return
		}
		next(w, r)
	}
}

func (a *SessionAPI) handlerFor(pattern string) http.HandlerFunc {
	switch pattern {
	case "POST /sessions":
		return a.handleCreate
	case "GET /sessions":
		return a.handleList
	case "GET /sessions/me":
		return a.handleMe
	case "GET /sessions/{id}":
		return a.handleGet
	case "POST /sessions/{id}/revoke":
		return a.handleRevoke
	}
	return nil
}

func (a *SessionAPI) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AgentID string `json:"agentId"`
		Label   string `json:"label,omitempty"`
		Browser string `json:"browser,omitempty"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil {
		httpx.ErrorCode(w, http.StatusBadRequest, "bad_request", "invalid request body", false, nil)
		return
	}
	if strings.TrimSpace(req.AgentID) == "" {
		httpx.ErrorCode(w, http.StatusBadRequest, "missing_agent_id", "agentId is required", false, nil)
		return
	}
	if req.Browser != "" {
		if _, err := config.ParseBrowser(req.Browser, a.browsersAvailable); err != nil {
			httpx.ErrorCode(w, http.StatusBadRequest, "invalid_browser", err.Error(), false, nil)
			return
		}
	}

	sessionID, token, err := a.store.Create(req.AgentID, req.Label, req.Browser)
	if err != nil {
		httpx.ErrorCode(w, http.StatusInternalServerError, "create_failed", "failed to create session", false, nil)
		return
	}

	sess, _ := a.store.Get(sessionID)

	activity.EnrichRequest(r, activity.Update{
		SessionID: sessionID,
		AgentID:   sess.AgentID,
		Action:    "sessions",
	})

	resp := map[string]any{
		"id":           sessionID,
		"agentId":      sess.AgentID,
		"label":        sess.Label,
		"sessionToken": token,
		"createdAt":    sess.CreatedAt,
		"expiresAt":    sess.ExpiresAt,
		"status":       sess.Status,
	}
	if sess.Browser != "" {
		resp["browser"] = sess.Browser
	}
	httpx.JSON(w, http.StatusCreated, resp)
}

func (a *SessionAPI) handleList(w http.ResponseWriter, _ *http.Request) {
	sessions := a.store.List()
	if sessions == nil {
		sessions = []session.Session{}
	}
	httpx.JSON(w, http.StatusOK, sessions)
}

func (a *SessionAPI) handleGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, ok := a.store.Get(id)
	if !ok {
		respondSessionNotFound(w, id)
		return
	}
	httpx.JSON(w, http.StatusOK, sess)
}

// tokenSuppliedForIDDetails explains the one mistake these endpoints cannot otherwise
// explain: `session create` hands the caller a TOKEN and every id-taking endpoint here
// rejects it, so the refusal reads as already-gone and the likely next action is to
// shrug and leave a live session running. The two values are distinguishable exactly,
// so the refusal names which one arrived rather than guessing.
//
// callerSessionID is the id of the session the request is authenticated AS, when it is
// authenticated as one at all. Handing that back is safe — the caller already holds
// that session's secret — and it is the whole remedy, so they need no second command.
// Returns nil for anything that is not a token, leaving the plain refusal alone.
//
// When the caller's own id is unknown the remedy is the listing, because that is the one
// command that always works here: `session info` needs PINCHTAB_SESSION exported, which is
// a precondition this refusal cannot verify, so it belongs in the hint.
func tokenSuppliedForIDDetails(supplied, callerSessionID string) map[string]any {
	if !session.LooksLikeToken(supplied) {
		return nil
	}
	r := listSessions.Remedy()
	if callerSessionID != "" {
		r = revokeCallerSession.Fill(callerSessionID)
	}
	return remedy.Details("that is a session TOKEN, not a session id — these endpoints take the id so an operator can end a session without holding its secret. With PINCHTAB_SESSION set, pinchtab session info prints the id.", r)
}

var (
	listSessions        = remedy.Declare("pinchtab session list")
	revokeCallerSession = remedy.Declare("pinchtab session revoke <session-id>")
)

func respondSessionNotFound(w http.ResponseWriter, supplied string) {
	httpx.ErrorCode(w, http.StatusNotFound, "session_not_found", "session not found", false,
		tokenSuppliedForIDDetails(supplied, ""))
}

func (a *SessionAPI) handleMe(w http.ResponseWriter, r *http.Request) {
	creds := authn.CredentialsFromRequest(r)
	if creds.Method != authn.MethodSession {
		httpx.ErrorCode(w, http.StatusUnauthorized, "session_auth_required", "this endpoint requires session authentication", false, nil)
		return
	}
	sess, ok := session.FromRequest(r)
	if !ok || sess == nil {
		httpx.ErrorCode(w, http.StatusUnauthorized, "bad_session", "invalid or expired session", false, nil)
		return
	}
	httpx.JSON(w, http.StatusOK, sess)
}

func (a *SessionAPI) handleRevoke(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	creds := authn.CredentialsFromRequest(r)
	switch creds.Method {
	case authn.MethodSession:
		sess, ok := session.FromRequest(r)
		if !ok || sess == nil {
			httpx.ErrorCode(w, http.StatusUnauthorized, "bad_session", "invalid or expired session", false, nil)
			return
		}
		if sess.ID != id {
			// The path a caller following the product's own instructions takes: with
			// PINCHTAB_SESSION exported, revoking "$PINCHTAB_SESSION" lands HERE rather
			// than on the 404, and "may only revoke their own session" is then actively
			// misleading — this IS their own session, named by the wrong value.
			httpx.ErrorCode(w, http.StatusForbidden, "forbidden", "session callers may only revoke their own session", false,
				tokenSuppliedForIDDetails(id, sess.ID))
			return
		}
	case authn.MethodHeader, authn.MethodCookie:
		// Dashboard-authenticated callers may revoke any session.
	default:
		httpx.ErrorCode(w, http.StatusForbidden, "forbidden", "not allowed to revoke this session", false, nil)
		return
	}
	remainingTabIDs := []string{}
	if a.sessionTabIDs != nil {
		remainingTabIDs = append(remainingTabIDs, a.sessionTabIDs(id)...)
	}
	if !a.store.Revoke(id) {
		respondSessionNotFound(w, id)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"status":          "ok",
		"remainingTabIds": remainingTabIDs,
	})
}
