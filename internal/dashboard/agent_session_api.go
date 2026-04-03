package dashboard

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/pinchtab/pinchtab/internal/agentsession"
	"github.com/pinchtab/pinchtab/internal/authn"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

// AgentSessionAPI handles CRUD operations for agent sessions.
//
// SECURITY NOTE: There is no per-agent authorization scoping. Any authenticated
// caller (bearer or cookie) can create sessions for any agentId, list all sessions,
// revoke any agent's session, and rotate any agent's token. This is by design for
// trusted single-operator environments. For multi-agent setups, consider restricting
// session management to bearer-only auth or adding agent-scoped permissions.
type AgentSessionAPI struct {
	store *agentsession.Store
}

// NewAgentSessionAPI creates a new agent session API handler.
func NewAgentSessionAPI(store *agentsession.Store) *AgentSessionAPI {
	return &AgentSessionAPI{store: store}
}

// RegisterHandlers registers agent session API routes.
func (a *AgentSessionAPI) RegisterHandlers(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/sessions", a.handleCreate)
	mux.HandleFunc("GET /api/sessions", a.handleList)
	mux.HandleFunc("GET /api/sessions/me", a.handleMe)
	mux.HandleFunc("GET /api/sessions/{id}", a.handleGet)
	mux.HandleFunc("POST /api/sessions/{id}/rotate", a.handleRotate)
	mux.HandleFunc("POST /api/sessions/{id}/revoke", a.handleRevoke)
}

func (a *AgentSessionAPI) handleCreate(w http.ResponseWriter, r *http.Request) {
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
		httpx.ErrorCode(w, http.StatusInternalServerError, "create_failed", "failed to create agent session", false, nil)
		return
	}

	sess, _ := a.store.Get(sessionID)
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

func (a *AgentSessionAPI) handleList(w http.ResponseWriter, _ *http.Request) {
	sessions := a.store.List()
	if sessions == nil {
		sessions = []agentsession.Session{}
	}
	httpx.JSON(w, http.StatusOK, sessions)
}

func (a *AgentSessionAPI) handleGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, ok := a.store.Get(id)
	if !ok {
		httpx.ErrorCode(w, http.StatusNotFound, "session_not_found", "agent session not found", false, nil)
		return
	}
	httpx.JSON(w, http.StatusOK, sess)
}

func (a *AgentSessionAPI) handleMe(w http.ResponseWriter, r *http.Request) {
	creds := authn.CredentialsFromRequest(r)
	if creds.Method != authn.MethodSession {
		httpx.ErrorCode(w, http.StatusBadRequest, "session_auth_required", "this endpoint requires session authentication", false, nil)
		return
	}
	sess, ok := a.store.Authenticate(creds.Value)
	if !ok || sess == nil {
		httpx.ErrorCode(w, http.StatusUnauthorized, "bad_session", "invalid or expired agent session", false, nil)
		return
	}
	httpx.JSON(w, http.StatusOK, sess)
}

func (a *AgentSessionAPI) handleRotate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	newToken, err := a.store.Rotate(id)
	if err != nil {
		httpx.ErrorCode(w, http.StatusBadRequest, "rotate_failed", err.Error(), false, nil)
		return
	}
	sess, _ := a.store.Get(id)
	httpx.JSON(w, http.StatusOK, map[string]any{
		"id":           sess.ID,
		"agentId":      sess.AgentID,
		"sessionToken": newToken,
		"status":       sess.Status,
	})
}

func (a *AgentSessionAPI) handleRevoke(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !a.store.Revoke(id) {
		httpx.ErrorCode(w, http.StatusNotFound, "session_not_found", "agent session not found", false, nil)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
