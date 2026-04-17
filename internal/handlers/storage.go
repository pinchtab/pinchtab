package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/httpx"
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
	httpx.ErrorCode(w, 403, "state_export_disabled", httpx.DisabledEndpointMessage("stateExport", "security.allowStateExport"), false, map[string]any{
		"setting": "security.allowStateExport",
	})
	return false
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

	ctx, resolvedTabID, err := h.tabContext(r, tabID)
	if err != nil {
		httpx.Error(w, 404, err)
		return
	}
	if _, ok := h.enforceCurrentTabDomainPolicy(w, r, ctx, resolvedTabID); !ok {
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, 10*time.Second)
	defer tCancel()

	script := buildStorageGetScript(storageType, key)

	var resultJSON string
	if err := h.evalJS(tCtx, script, &resultJSON); err != nil {
		httpx.Error(w, 500, fmt.Errorf("evaluate storage get: %w", err))
		return
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(resultJSON), &result); err != nil {
		httpx.Error(w, 500, fmt.Errorf("parse storage result: %w", err))
		return
	}

	h.recordActivity(r, activity.Update{Action: "storage.read"})

	slog.Info("storage: get",
		"type", storageType,
		"key", key,
		"tabId", resolvedTabID,
		"remoteAddr", r.RemoteAddr,
	)

	httpx.JSON(w, 200, result)
}

type storageSetRequest struct {
	TabID string `json:"tabId"`
	Key   string `json:"key"`
	Value string `json:"value"`
	Type  string `json:"type"`
}

// handleStorageSet sets a single storage item.
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

	ctx, resolvedTabID, err := h.tabContext(r, req.TabID)
	if err != nil {
		httpx.Error(w, 404, err)
		return
	}
	if _, ok := h.enforceCurrentTabDomainPolicy(w, r, ctx, resolvedTabID); !ok {
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, 10*time.Second)
	defer tCancel()

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

	var resultJSON string
	if err := h.evalJS(tCtx, script, &resultJSON); err != nil {
		httpx.Error(w, 500, fmt.Errorf("evaluate storage set: %w", err))
		return
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(resultJSON), &result); err != nil {
		httpx.Error(w, 500, fmt.Errorf("parse storage set result: %w", err))
		return
	}

	h.recordActivity(r, activity.Update{Action: "storage.write"})

	slog.Info("storage: set",
		"type", req.Type,
		"key", req.Key,
		"tabId", resolvedTabID,
		"remoteAddr", r.RemoteAddr,
	)

	httpx.JSON(w, 200, result)
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

	// Defaults for empty body/missing type
	if req.Type == "" {
		req.Type = "all"
	}

	if req.Type != "local" && req.Type != "session" && req.Type != "all" {
		httpx.Error(w, 400, fmt.Errorf("type must be 'local', 'session', or 'all'"))
		return
	}
	// key is not compatible with type=all
	if req.Type == "all" && req.Key != "" {
		httpx.Error(w, 400, fmt.Errorf("key cannot be used with type=all; omit key to clear both storages"))
		return
	}

	ctx, resolvedTabID, err := h.tabContext(r, req.TabID)
	if err != nil {
		httpx.Error(w, 404, err)
		return
	}
	if _, ok := h.enforceCurrentTabDomainPolicy(w, r, ctx, resolvedTabID); !ok {
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, 10*time.Second)
	defer tCancel()

	script := buildStorageDeleteScript(req.Type, req.Key)

	var resultJSON string
	if err := h.evalJS(tCtx, script, &resultJSON); err != nil {
		httpx.Error(w, 500, fmt.Errorf("evaluate storage delete: %w", err))
		return
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(resultJSON), &result); err != nil {
		httpx.Error(w, 500, fmt.Errorf("parse storage delete result: %w", err))
		return
	}

	h.recordActivity(r, activity.Update{Action: "storage.delete"})

	slog.Info("storage: delete",
		"type", req.Type,
		"key", req.Key,
		"tabId", resolvedTabID,
		"remoteAddr", r.RemoteAddr,
	)

	httpx.JSON(w, 200, result)
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

	// No key specified: return all items
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
	// type=all: clear both localStorage and sessionStorage
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
	tabID := r.PathValue("id")
	if tabID == "" {
		httpx.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}

	q := r.URL.Query()
	q.Set("tabId", tabID)

	req := r.Clone(r.Context())
	u := *r.URL
	u.RawQuery = q.Encode()
	req.URL = &u

	h.handleStorageGet(w, req)
}

// HandleTabStorageSet sets a storage item for a tab identified by path ID.
//
// @Endpoint POST /tabs/{id}/storage
func (h *Handlers) HandleTabStorageSet(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if tabID == "" {
		httpx.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}

	body := map[string]any{}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize))
	if err := dec.Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		httpx.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}

	if rawTabID, ok := body["tabId"]; ok {
		if provided, ok := rawTabID.(string); !ok || provided == "" {
			httpx.Error(w, 400, fmt.Errorf("invalid tabId"))
			return
		} else if provided != tabID {
			httpx.Error(w, 400, fmt.Errorf("tabId in body does not match path id"))
			return
		}
	}

	body["tabId"] = tabID

	payload, err := json.Marshal(body)
	if err != nil {
		httpx.Error(w, 500, fmt.Errorf("encode: %w", err))
		return
	}

	req := r.Clone(r.Context())
	req.Body = io.NopCloser(bytes.NewReader(payload))
	req.ContentLength = int64(len(payload))
	req.Header = r.Header.Clone()
	req.Header.Set("Content-Type", "application/json")
	h.handleStorageSet(w, req)
}

// HandleTabStorageDelete deletes storage items for a tab identified by path ID.
//
// @Endpoint DELETE /tabs/{id}/storage
func (h *Handlers) HandleTabStorageDelete(w http.ResponseWriter, r *http.Request) {
	tabID := r.PathValue("id")
	if tabID == "" {
		httpx.Error(w, 400, fmt.Errorf("tab id required"))
		return
	}

	body := map[string]any{}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize))
	if err := dec.Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		httpx.Error(w, 400, fmt.Errorf("decode: %w", err))
		return
	}

	if rawTabID, ok := body["tabId"]; ok {
		if provided, ok := rawTabID.(string); !ok || provided == "" {
			httpx.Error(w, 400, fmt.Errorf("invalid tabId"))
			return
		} else if provided != tabID {
			httpx.Error(w, 400, fmt.Errorf("tabId in body does not match path id"))
			return
		}
	}

	body["tabId"] = tabID

	payload, err := json.Marshal(body)
	if err != nil {
		httpx.Error(w, 500, fmt.Errorf("encode: %w", err))
		return
	}

	req := r.Clone(r.Context())
	req.Body = io.NopCloser(bytes.NewReader(payload))
	req.ContentLength = int64(len(payload))
	req.Header = r.Header.Clone()
	req.Header.Set("Content-Type", "application/json")
	h.handleStorageDelete(w, req)
}
