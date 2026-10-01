package handlers

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
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
	if h.allows(routes.CapStateExport) {
		return true
	}
	h.writeCapabilityDisabled(w, routes.CapStateExport)
	return false
}

// runStorageOp resolves the current tab, runs the storage script under a 10s
// timeout, decodes the JSON result, records activity + logs, and writes the
// response. It writes any error response and returns. opLabel is the error/log
// infix ("get"/"set"/"delete"); activityAction is the recordActivity action;
// logType/logKey are the structured-log values. guards comes from the caller
// because the three ops share this body but not their guard set: the writes
// carry handoff-pause, the read does not.
func (h *Handlers) runStorageOp(w http.ResponseWriter, r *http.Request, tabID, script, opLabel, activityAction, logType, logKey string, guards tabGuards) {
	ctx, resolvedTabID, ok := h.guardedTabContext(w, r, tabID, guards)
	if !ok {
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, 10*time.Second)
	defer tCancel()

	var resultJSON string
	if err := h.evalJS(tCtx, script, &resultJSON); err != nil {
		httpx.Error(w, 500, fmt.Errorf("evaluate storage %s: %w", opLabel, err))
		return
	}

	var result map[string]any
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
//   - type: "local", "session", "all", or "" (all)
//   - key:  optional specific key to retrieve
func (h *Handlers) handleStorageGet(w http.ResponseWriter, r *http.Request) {
	if !h.ensureStateExportEnabled(w) {
		return
	}
	query := r.URL.Query()
	scope, err := parseStorageScope(cmp.Or(query.Get("type"), string(storageAll)))
	if err != nil {
		httpx.Error(w, 400, err)
		return
	}
	key := query.Get("key")

	script := buildStorageGetScript(scope, key)
	h.runStorageOp(w, r, query.Get("tabId"), script, "get", "storage.read", string(scope), key, guardTabLease|guardDialogBlocked|guardDomainPolicy)
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
	if !decodeRequiredJSON(w, r, &req) {
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
	scope, err := parseStorageScope(req.Type)
	if err == nil && scope == storageAll {
		err = fmt.Errorf("type must be 'local' or 'session' to set an item")
	}
	if err != nil {
		httpx.Error(w, 400, err)
		return
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
	`, storageObjects[scope], string(keyJSON), string(valueJSON))

	h.runStorageOp(w, r, req.TabID, script, "set", "storage.write", req.Type, req.Key, guardTabLease|guardDialogBlocked|guardDomainPolicy|guardHandoffPause)
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

	if !decodeOptionalJSON(w, r, &req) {
		return
	}
	query := r.URL.Query()
	req.TabID = cmp.Or(req.TabID, query.Get("tabId"))
	req.Key = cmp.Or(req.Key, query.Get("key"))
	req.Type = cmp.Or(req.Type, query.Get("type"), string(storageAll))

	scope, err := parseStorageScope(req.Type)
	if err != nil {
		httpx.Error(w, 400, err)
		return
	}
	if scope == storageAll && req.Key != "" {
		httpx.Error(w, 400, fmt.Errorf("key cannot be used with type=all; omit key to clear both storages"))
		return
	}

	script := buildStorageDeleteScript(scope, req.Key)
	h.runStorageOp(w, r, req.TabID, script, "delete", "storage.delete", req.Type, req.Key, guardTabLease|guardDialogBlocked|guardDomainPolicy|guardHandoffPause)
}

type storageScope string

const (
	storageLocal   storageScope = "local"
	storageSession storageScope = "session"
	storageAll     storageScope = "all"
)

var storageObjects = map[storageScope]string{
	storageLocal:   "localStorage",
	storageSession: "sessionStorage",
}

func parseStorageScope(raw string) (storageScope, error) {
	switch scope := storageScope(raw); scope {
	case storageLocal, storageSession, storageAll:
		return scope, nil
	}
	return "", fmt.Errorf("type must be 'local', 'session', or 'all', got %q", raw)
}

func (s storageScope) includes(store storageScope) bool {
	return s == storageAll || s == store
}

// buildStorageGetScript builds a JS expression that reads from localStorage
// and/or sessionStorage. Returns a JSON string with local/session/origin fields.
// A keyed read lets a storage error surface as the error field; a full read
// returns whatever items it reached.
func buildStorageGetScript(scope storageScope, key string) string {
	read := func(store storageScope) string {
		if !scope.includes(store) {
			return "[]"
		}
		obj := storageObjects[store]
		if key != "" {
			keyJSON, _ := json.Marshal(key)
			return fmt.Sprintf(`(function(v) { return v !== null ? [{key: %s, value: v}] : []; })(%s.getItem(%s))`, keyJSON, obj, keyJSON)
		}
		return fmt.Sprintf(`
			(function() {
				var items = [];
				try {
					for (var i = 0; i < %[1]s.length; i++) {
						var k = %[1]s.key(i);
						items.push({key: k, value: %[1]s.getItem(k)});
					}
				} catch(e) {}
				return items;
			})()`, obj)
	}
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
	`, read(storageLocal), read(storageSession))
}

// buildStorageDeleteScript builds a JS expression that removes a specific key
// or clears the entire storage. Supports type local, session, or all.
// Returns a JSON string with success/origin fields.
func buildStorageDeleteScript(scope storageScope, key string) string {
	if scope == storageAll {
		return fmt.Sprintf(`
		(function() {
			try {
				%s.clear();
				%s.clear();
				return JSON.stringify({success: true, action: "clear", type: "all", origin: window.location.origin});
			} catch(e) {
				return JSON.stringify({success: false, error: e.message, origin: window.location.origin});
			}
		})()
	`, storageObjects[storageLocal], storageObjects[storageSession])
	}

	storageObj := storageObjects[scope]

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
