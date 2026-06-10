package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/pinchtab/pinchtab/internal/activity"
	bridgeruntime "github.com/pinchtab/pinchtab/internal/bridge/runtime"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

// credentialPair holds HTTP basic auth credentials for a tab.
type credentialPair struct {
	Username string
	Password string
}

// credentialStore provides thread-safe per-tab credential storage and tracks
// which tabs already have a CDP event listener installed to avoid stacking
// duplicate listeners on repeated calls.
type credentialStore struct {
	mu          sync.RWMutex
	credentials map[string]*credentialPair
	listeners   map[string]bool
}

func newCredentialStore() *credentialStore {
	return &credentialStore{
		credentials: make(map[string]*credentialPair),
		listeners:   make(map[string]bool),
	}
}

func (cs *credentialStore) Set(tabID string, cred *credentialPair) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.credentials[tabID] = cred
}

func (cs *credentialStore) Get(tabID string) (*credentialPair, bool) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	cred, ok := cs.credentials[tabID]
	return cred, ok
}

func (cs *credentialStore) Delete(tabID string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	delete(cs.credentials, tabID)
	// Keep listeners[tabID] — the bridge listener is bound to the tab's
	// context and survives across clear/re-set cycles. Clearing the flag
	// here would cause a second listener to be installed on re-set.
}

// MarkListenerIfAbsent atomically marks a listener as installed for tabID.
// Returns true if this call was the one that set it (i.e., no listener existed).
func (cs *credentialStore) MarkListenerIfAbsent(tabID string) bool {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.listeners[tabID] {
		return false
	}
	cs.listeners[tabID] = true
	return true
}

type credentialsRequest struct {
	TabID    string  `json:"tabId"`
	Username *string `json:"username"`
	Password string  `json:"password"`
}

// HandleSetCredentials sets HTTP auth credentials via CDP Fetch domain.
// POST /emulation/credentials
func (h *Handlers) HandleSetCredentials(w http.ResponseWriter, r *http.Request) {
	var req credentialsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		httpx.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}

	h.setCredentials(w, r, req)
}

// HandleTabSetCredentials sets HTTP auth credentials for a specific tab.
// POST /tabs/{id}/emulation/credentials
func (h *Handlers) HandleTabSetCredentials(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if tabID == "" {
		httpx.Error(w, 400, fmt.Errorf("missing tab ID"))
		return
	}

	var req credentialsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		httpx.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}

	if req.TabID != "" && req.TabID != tabID {
		httpx.Error(w, 400, fmt.Errorf("tabId in body %q does not match URL path %q", req.TabID, tabID))
		return
	}
	req.TabID = tabID

	h.setCredentials(w, r, req)
}

func (h *Handlers) setCredentials(w http.ResponseWriter, r *http.Request, req credentialsRequest) {
	if req.Username == nil {
		httpx.Error(w, 400, fmt.Errorf("missing required field: username"))
		return
	}

	username := *req.Username

	// Non-empty username requires password field (empty password is allowed).
	// Empty username means "clear credentials".

	ctx, resolvedTabID, err := h.tabContext(r, req.TabID)
	if err != nil {
		WriteTabContextError(w, err, 404)
		return
	}
	if _, ok := h.enforceCurrentTabDomainPolicy(w, r, ctx, resolvedTabID); !ok {
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, 5*time.Second)
	defer tCancel()

	if username == "" {
		// Clear credentials: disable fetch domain and remove stored credentials.
		h.credentialStore.Delete(resolvedTabID)

		if proxyAuthConfigured(h.Config) {
			// Proxy auth shares the Fetch domain; keep it enabled with auth
			// handling instead of disabling, and hand pause dispatch back to
			// the proxy-auth listener.
			if err := h.Bridge.EnableFetchWithAuth(tCtx); err != nil {
				httpx.Error(w, 500, fmt.Errorf("CDP fetch re-enable for proxy auth: %w", err))
				return
			}
		} else if err := h.Bridge.DisableFetch(tCtx); err != nil {
			httpx.Error(w, 500, fmt.Errorf("CDP fetch disable: %w", err))
			return
		}
		h.setFetchPauseSuppressed(resolvedTabID, false)

		h.recordActivity(r, activity.Update{Action: "emulation.credentials", TabID: resolvedTabID})

		httpx.JSON(w, 200, map[string]any{
			"status": "cleared",
		})
		return
	}

	// Store credentials for the event listener to reference.
	h.credentialStore.Set(resolvedTabID, &credentialPair{
		Username: username,
		Password: req.Password,
	})

	if err := h.Bridge.EnableFetchWithAuth(tCtx); err != nil {
		httpx.Error(w, 500, fmt.Errorf("CDP fetch enable: %w", err))
		return
	}
	// The credentials listener continues paused requests itself; quiet the
	// proxy-auth listener's blanket continue while this session is active.
	// Precedence on auth challenges is racy by design: the proxy-auth
	// listener answers proxy-source challenges, this listener answers via
	// stored credentials; a double answer logs a debug error harmlessly.
	h.setFetchPauseSuppressed(resolvedTabID, true)

	// Install event listener only once per tab. The listener reads credentials
	// from the store dynamically, so updating creds doesn't need a new listener.
	if h.credentialStore.MarkListenerIfAbsent(resolvedTabID) {
		h.Bridge.ListenAuthRequired(ctx, func(requestID string, isAuth bool) {
			if isAuth {
				go func() {
					cred, ok := h.credentialStore.Get(resolvedTabID)
					if !ok {
						return
					}
					if err := h.Bridge.ContinueWithAuth(ctx, requestID, cred.Username, cred.Password); err != nil {
						slog.Warn("credentials: ContinueWithAuth failed", "tab", resolvedTabID, "err", err)
					}
				}()
			} else {
				go func() {
					if err := h.Bridge.ContinueRequest(ctx, requestID); err != nil {
						slog.Warn("credentials: ContinueRequest failed", "tab", resolvedTabID, "err", err)
					}
				}()
			}
		})
	}

	h.recordActivity(r, activity.Update{Action: "emulation.credentials", TabID: resolvedTabID})

	httpx.JSON(w, 200, map[string]any{
		"username": username,
		"status":   "applied",
	})
}

// fetchPauseSuppressor is probed on the bridge for Fetch-domain coordination
// with the proxy-auth listener. The ghost-chrome adapter does not promote
// this method (interface-embedded *Bridge), so suppression is skipped there —
// acceptable: routes/credentials + proxy auth on ghost-chrome is an edge of
// an edge, and the failure mode is a raced duplicate continue, not a hang.
type fetchPauseSuppressor interface {
	SetFetchPauseSuppressed(tabID string, v bool)
}

func (h *Handlers) setFetchPauseSuppressed(tabID string, v bool) {
	if sup, ok := h.Bridge.(fetchPauseSuppressor); ok {
		sup.SetFetchPauseSuppressed(tabID, v)
	}
}

func proxyAuthConfigured(cfg *config.RuntimeConfig) bool {
	return cfg != nil && bridgeruntime.ProxyAuthEnabled(cfg.Proxy)
}
