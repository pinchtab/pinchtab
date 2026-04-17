package dashboard

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/authn"
	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/session"
)

// SessionAPI handles CRUD operations for sessions.
type SessionAPI struct {
	store *session.Store
}

// NewSessionAPI creates a new session API handler.
func NewSessionAPI(store *session.Store) *SessionAPI {
	return &SessionAPI{store: store}
}

// RegisterHandlers registers session API routes.
func (a *SessionAPI) RegisterHandlers(mux *http.ServeMux) {
	if a == nil || a.store == nil || !a.store.Enabled() {
		return
	}
	mux.HandleFunc("POST /sessions", a.handleCreate)
	mux.HandleFunc("GET /sessions", a.handleList)
	mux.HandleFunc("GET /sessions/me", a.handleMe)
	mux.HandleFunc("GET /sessions/{id}", a.handleGet)
	mux.HandleFunc("POST /sessions/{id}/revoke", a.handleRevoke)
}

func (a *SessionAPI) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AgentID string `json:"agentId"`
		Label   string `json:"label,omitempty"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil {
		httpx.ErrorCode(w, http.StatusBadRequest, "bad_request", "invalid request body", false, nil)
		return
	}
	if strings.TrimSpace(req.AgentID) == "" {
		httpx.ErrorCode(w, http.StatusBadRequest, "missing_agent_id", "agentId is required", false, nil)
		return
	}

	sessionID, token, err := a.store.Create(req.AgentID, req.Label)
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

	httpx.JSON(w, http.StatusCreated, map[string]any{
		"id":           sessionID,
		"agentId":      sess.AgentID,
		"label":        sess.Label,
		"sessionToken": token,
		"createdAt":    sess.CreatedAt,
		"expiresAt":    sess.ExpiresAt,
		"status":       sess.Status,
	})
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
		httpx.ErrorCode(w, http.StatusNotFound, "session_not_found", "session not found", false, nil)
		return
	}
	httpx.JSON(w, http.StatusOK, sess)
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
			httpx.ErrorCode(w, http.StatusForbidden, "forbidden", "session callers may only revoke their own session", false, nil)
			return
		}
	case authn.MethodHeader, authn.MethodCookie:
		// Dashboard-authenticated callers may revoke any session.
	default:
		httpx.ErrorCode(w, http.StatusForbidden, "forbidden", "not allowed to revoke this session", false, nil)
		return
	}
	if !a.store.Revoke(id) {
		httpx.ErrorCode(w, http.StatusNotFound, "session_not_found", "session not found", false, nil)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
