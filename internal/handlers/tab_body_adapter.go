package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/pinchtab/pinchtab/internal/httpx"
)

func decodeJSONBody[T any](w http.ResponseWriter, r *http.Request) (T, bool) {
	var req T
	ok := decodeRequiredJSON(w, r, &req)
	return req, ok
}

func requirePathTabID(w http.ResponseWriter, r *http.Request) (string, bool) {
	tabID := strings.TrimSpace(r.PathValue("id"))
	if tabID == "" {
		httpx.Error(w, 400, fmt.Errorf("tab id required"))
		return "", false
	}
	return tabID, true
}

func bodyTabID(body map[string]any) (string, error) {
	raw, present := body["tabId"]
	if !present {
		return "", nil
	}
	provided, ok := raw.(string)
	if !ok || provided == "" {
		return "", fmt.Errorf("invalid tabId")
	}
	return provided, nil
}

func (h *Handlers) requirePathTabIDMatch(w http.ResponseWriter, r *http.Request, bodyTabID string) (string, bool) {
	tabID, ok := requirePathTabID(w, r)
	if !ok {
		return "", false
	}
	if bodyTabID != "" && bodyTabID != tabID {
		httpx.Error(w, 400, fmt.Errorf("tabId in body does not match path id"))
		return "", false
	}
	return tabID, true
}

// withPathTabIDBody decodes a JSON-object body, validates any body "tabId"
// against the {id} path value, injects the path tabId, and forwards a cloned
// request (rewritten body + application/json) to root. Empty body is tolerated
// (EOF); the path id is required. Sibling to withPathTabID (the query-param
// variant in inspect_lifecycle.go) for POST /tabs/{id}/... handlers that
// delegate to a root (or another tab) handler.
func (h *Handlers) withPathTabIDBody(w http.ResponseWriter, r *http.Request, root http.HandlerFunc) {
	h.withPathTabIDBodyMutate(w, r, nil, root)
}

// withPathTabIDBodyMutate is withPathTabIDBody plus an optional hook to mutate
// the decoded body map after the path tabId is injected and before the body is
// re-marshaled — for endpoints that must also stamp extra fields onto the
// forwarded body (e.g. POST /tabs/{id}/navigate forcing newTab=false). A nil
// mutate makes it identical to withPathTabIDBody.
func (h *Handlers) withPathTabIDBodyMutate(w http.ResponseWriter, r *http.Request, mutate func(body map[string]any), root http.HandlerFunc) {
	body := map[string]any{}
	if !decodeOptionalJSON(w, r, &body) {
		return
	}
	provided, err := bodyTabID(body)
	if err != nil {
		httpx.Error(w, 400, err)
		return
	}
	tabID, ok := h.requirePathTabIDMatch(w, r, provided)
	if !ok {
		return
	}
	body["tabId"] = tabID
	if mutate != nil {
		mutate(body)
	}

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
	root(w, req)
}
