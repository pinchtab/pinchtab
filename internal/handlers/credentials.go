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

type credentialPair struct {
	Username string
	Password string
}

// tabRemovalNotifier lets the bridge notify handlers when a tab is removed, so
// per-tab state (credential bookkeeping) can be reclaimed on any removal path.
type tabRemovalNotifier interface {
	AddTabRemovedHook(fn func(tabID string))
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

// Get returns a copy of the stored credentials so callers cannot mutate
// store-owned state outside the lock.
func (cs *credentialStore) Get(tabID string) (credentialPair, bool) {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	cred, ok := cs.credentials[tabID]
	if !ok {
		return credentialPair{}, false
	}
	return *cred, true
}

func (cs *credentialStore) Delete(tabID string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	delete(cs.credentials, tabID)
	// Keep listeners[tabID] — the bridge listener is bound to the tab's
	// context and survives across clear/re-set cycles. Clearing the flag
	// here would cause a second listener to be installed on re-set.
}

// RemoveTab drops all per-tab bookkeeping when a tab is gone. Unlike Delete,
// this also clears listeners[tabID]: the tab's listener context is cancelled on
// removal, so the dedup flag is meaningless and would otherwise leak. Wired into
// the bridge tab-removal lifecycle so dead tab IDs do not accumulate.
func (cs *credentialStore) RemoveTab(tabID string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	delete(cs.credentials, tabID)
	delete(cs.listeners, tabID)
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

// HandleSetCredentials sets HTTP auth credentials via the CDP Fetch domain.
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
		h.Bridge.SetFetchPauseSuppressed(resolvedTabID, false)

		h.recordActivity(r, activity.Update{Action: "emulation.credentials", TabID: resolvedTabID})

		httpx.JSON(w, 200, map[string]any{
			"status": "cleared",
		})
		return
	}

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
	h.Bridge.SetFetchPauseSuppressed(resolvedTabID, true)

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

func proxyAuthConfigured(cfg *config.RuntimeConfig) bool {
	return cfg != nil && bridgeruntime.ProxyAuthEnabled(cfg.Proxy)
}
