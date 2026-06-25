package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/routes"
)

// HandleStorage dispatches to the appropriate storage operation based on HTTP method.
// Storage is captured only for the current origin (active tab).
//
// GET    /storage — retrieve storage items
// POST   /storage — set a storage item
// DELETE /storage — remove storage items or clear storage
func (h *Handlers) HandleStorage(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.handleStorageGet(w, r)
	case http.MethodPost:
		h.handleStorageSet(w, r)
	case http.MethodDelete:
		h.handleStorageDelete(w, r)
	default:
		httpx.Error(w, http.StatusMethodNotAllowed, fmt.Errorf("method %s not allowed", r.Method))
	}
}

func (h *Handlers) ensureStateExportEnabled(w http.ResponseWriter) bool {
	if h.stateExportEnabled() {
		return true
	}
	h.writeCapabilityDisabled(w, routes.CapStateExport)
	return false
}

// runStorageOp resolves the current tab, runs the storage script under a 10s
// timeout, decodes the JSON result, records activity + logs, and writes the
// response. It writes any error response and returns. opLabel is the error/log
// infix ("get"/"set"/"delete"); activityAction is the recordActivity action;
// logType/logKey are the structured-log values.
func (h *Handlers) runStorageOp(w http.ResponseWriter, r *http.Request, tabID, script, opLabel, activityAction, logType, logKey string) {
	ctx, resolvedTabID, err := h.tabContext(r, tabID)
	if err != nil {
		WriteTabContextError(w, err, 404)
		return
	}
	if _, ok := h.enforceCurrentTabDomainPolicy(w, r, ctx, resolvedTabID); !ok {
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, 10*time.Second)
	defer tCancel()

	var resultJSON string
	if err := h.evalJS(tCtx, script, &resultJSON); err != nil {
		httpx.Error(w, 500, fmt.Errorf("evaluate storage %s: %w", opLabel, err))
		return
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(resultJSON), &result); err != nil {
		httpx.Error(w, 500, fmt.Errorf("parse storage %s result: %w", opLabel, err))
		return
	}

	h.recordActivity(r, activity.Update{Action: activityAction})

	slog.Info("storage: "+opLabel,
		"type", logType,
		"key", logKey,
		"tabId", resolvedTabID,
		"remoteAddr", r.RemoteAddr,
	)

	httpx.JSON(w, 200, result)
}

// handleStorageGet retrieves localStorage and/or sessionStorage items.
// Gated behind CapStateExport: storage can contain auth tokens and session data.
//
// Query params:
//   - type: "local", "session", or "" (both)
//   - key:  optional specific key to retrieve
func (h *Handlers) handleStorageGet(w http.ResponseWriter, r *http.Request) {
	if !h.ensureStateExportEnabled(w) {
		return
	}
	tabID := r.URL.Query().Get("tabId")
	storageType := r.URL.Query().Get("type")
	key := r.URL.Query().Get("key")

	script := buildStorageGetScript(storageType, key)
	h.runStorageOp(w, r, tabID, script, "get", "storage.read", storageType, key)
}

type storageSetRequest struct {
	TabID string `json:"tabId"`
	Key   string `json:"key"`
	Value string `json:"value"`
	Type  string `json:"type"`
}

func (h *Handlers) handleStorageSet(w http.ResponseWriter, r *http.Request) {
	if !h.ensureStateExportEnabled(w) {
		return
	}

	var req storageSetRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(&req); err != nil {
		httpx.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}

	if req.Key == "" {
		httpx.Error(w, 400, fmt.Errorf("key is required"))
		return
	}
	if req.Type == "" {
		httpx.Error(w, 400, fmt.Errorf("type is required (local or session)"))
		return
	}
	if req.Type != "local" && req.Type != "session" {
		httpx.Error(w, 400, fmt.Errorf("type must be 'local' or 'session'"))
		return
	}

	storageObj := "localStorage"
	if req.Type == "session" {
		storageObj = "sessionStorage"
	}

	keyJSON, _ := json.Marshal(req.Key)
	valueJSON, _ := json.Marshal(req.Value)

	script := fmt.Sprintf(`
		try {
			%s.setItem(%s, %s);
			JSON.stringify({success: true, origin: window.location.origin});
		} catch(e) {
			JSON.stringify({success: false, error: e.message, origin: window.location.origin});
		}
	`, storageObj, string(keyJSON), string(valueJSON))

	h.runStorageOp(w, r, req.TabID, script, "set", "storage.write", req.Type, req.Key)
}

// handleStorageDelete removes a storage item or clears storage.
// Supports type=local, type=session, or type=all (clears both).
func (h *Handlers) handleStorageDelete(w http.ResponseWriter, r *http.Request) {
	if !h.ensureStateExportEnabled(w) {
		return
	}

	var req struct {
		TabID string `json:"tabId"`
		Key   string `json:"key"`
		Type  string `json:"type"`
	}

	// Read body to check if it's empty (optional for DELETE)
	bodyBytes, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodySize))
	if err != nil {
		httpx.Error(w, 400, fmt.Errorf("read body: %w", err))
		return
	}

	if len(bodyBytes) > 0 {
		if err := json.Unmarshal(bodyBytes, &req); err != nil {
			httpx.Error(w, 400, fmt.Errorf("decode: %w", err))
			return
		}
	}

	if req.Type == "" {
		req.Type = "all"
	}

	if req.Type != "local" && req.Type != "session" && req.Type != "all" {
		httpx.Error(w, 400, fmt.Errorf("type must be 'local', 'session', or 'all'"))
		return
	}
	if req.Type == "all" && req.Key != "" {
		httpx.Error(w, 400, fmt.Errorf("key cannot be used with type=all; omit key to clear both storages"))
		return
	}

	script := buildStorageDeleteScript(req.Type, req.Key)
	h.runStorageOp(w, r, req.TabID, script, "delete", "storage.delete", req.Type, req.Key)
}

// buildStorageGetScript builds a JS expression that reads from localStorage
// and/or sessionStorage. Returns a JSON string with local/session/origin fields.
func buildStorageGetScript(storageType, key string) string {
	if key != "" {
		keyJSON, _ := json.Marshal(key)
		switch storageType {
		case "local":
			return fmt.Sprintf(`
				(function() {
					try {
						var v = localStorage.getItem(%s);
						return JSON.stringify({
							local: v !== null ? [{key: %s, value: v}] : [],
							session: [],
							origin: window.location.origin
						});
					} catch(e) {
						return JSON.stringify({error: e.message, local: [], session: [], origin: window.location.origin});
					}
				})()
			`, string(keyJSON), string(keyJSON))
		case "session":
			return fmt.Sprintf(`
				(function() {
					try {
						var v = sessionStorage.getItem(%s);
						return JSON.stringify({
							local: [],
							session: v !== null ? [{key: %s, value: v}] : [],
							origin: window.location.origin
						});
					} catch(e) {
						return JSON.stringify({error: e.message, local: [], session: [], origin: window.location.origin});
					}
				})()
			`, string(keyJSON), string(keyJSON))
		default:
			return fmt.Sprintf(`
				(function() {
					try {
						var lv = localStorage.getItem(%s);
						var sv = sessionStorage.getItem(%s);
						return JSON.stringify({
							local: lv !== null ? [{key: %s, value: lv}] : [],
							session: sv !== null ? [{key: %s, value: sv}] : [],
							origin: window.location.origin
						});
					} catch(e) {
						return JSON.stringify({error: e.message, local: [], session: [], origin: window.location.origin});
					}
				})()
			`, string(keyJSON), string(keyJSON), string(keyJSON), string(keyJSON))
		}
	}

	getAllScript := func(storageObj string) string {
		return fmt.Sprintf(`
			(function() {
				var items = [];
				try {
					for (var i = 0; i < %s.length; i++) {
						var k = %s.key(i);
						items.push({key: k, value: %s.getItem(k)});
					}
				} catch(e) {}
				return items;
			})()
		`, storageObj, storageObj, storageObj)
	}

	switch storageType {
	case "local":
		return fmt.Sprintf(`
			(function() {
				try {
					var local = %s;
					return JSON.stringify({local: local, session: [], origin: window.location.origin});
				} catch(e) {
					return JSON.stringify({error: e.message, local: [], session: [], origin: window.location.origin});
				}
			})()
		`, getAllScript("localStorage"))
	case "session":
		return fmt.Sprintf(`
			(function() {
				try {
					var session = %s;
					return JSON.stringify({local: [], session: session, origin: window.location.origin});
				} catch(e) {
					return JSON.stringify({error: e.message, local: [], session: [], origin: window.location.origin});
				}
			})()
		`, getAllScript("sessionStorage"))
	default:
		return fmt.Sprintf(`
			(function() {
				try {
					var local = %s;
					var session = %s;
					return JSON.stringify({local: local, session: session, origin: window.location.origin});
				} catch(e) {
					return JSON.stringify({error: e.message, local: [], session: [], origin: window.location.origin});
				}
			})()
		`, getAllScript("localStorage"), getAllScript("sessionStorage"))
	}
}

// buildStorageDeleteScript builds a JS expression that removes a specific key
// or clears the entire storage. Supports type local, session, or all.
// Returns a JSON string with success/origin fields.
func buildStorageDeleteScript(storageType, key string) string {
	if storageType == "all" {
		return `
		(function() {
			try {
				localStorage.clear();
				sessionStorage.clear();
				return JSON.stringify({success: true, action: "clear", type: "all", origin: window.location.origin});
			} catch(e) {
				return JSON.stringify({success: false, error: e.message, origin: window.location.origin});
			}
		})()
	`
	}

	storageObj := "localStorage"
	if storageType == "session" {
		storageObj = "sessionStorage"
	}

	if key != "" {
		keyJSON, _ := json.Marshal(key)
		return fmt.Sprintf(`
			(function() {
				try {
					%s.removeItem(%s);
					return JSON.stringify({success: true, action: "removeItem", key: %s, origin: window.location.origin});
				} catch(e) {
					return JSON.stringify({success: false, error: e.message, origin: window.location.origin});
				}
			})()
		`, storageObj, string(keyJSON), string(keyJSON))
	}

	return fmt.Sprintf(`
		(function() {
			try {
				%s.clear();
				return JSON.stringify({success: true, action: "clear", origin: window.location.origin});
			} catch(e) {
				return JSON.stringify({success: false, error: e.message, origin: window.location.origin});
			}
		})()
	`, storageObj)
}

// HandleTabStorageGet retrieves storage items for a tab identified by path ID.
//
// @Endpoint GET /tabs/{id}/storage
func (h *Handlers) HandleTabStorageGet(w http.ResponseWriter, r *http.Request) {
	h.withPathTabID(w, r, h.handleStorageGet)
}

// HandleTabStorageSet sets a storage item for a tab identified by path ID.
//
// @Endpoint POST /tabs/{id}/storage
func (h *Handlers) HandleTabStorageSet(w http.ResponseWriter, r *http.Request) {
	h.withPathTabIDBody(w, r, h.handleStorageSet)
}

// HandleTabStorageDelete deletes storage items for a tab identified by path ID.
//
// @Endpoint DELETE /tabs/{id}/storage
func (h *Handlers) HandleTabStorageDelete(w http.ResponseWriter, r *http.Request) {
	h.withPathTabIDBody(w, r, h.handleStorageDelete)
}
