package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/pinchtab/pinchtab/internal/httpx"
)

// requirePathTabIDMatch validates the {id} path value (required) against an
// optional body-provided tabId, returning the resolved path tabID. ok=false
// means an error response was already written. For typed tab handlers that call
// an internal func directly (no JSON re-marshal) — the typed sibling of
// withPathTabIDBody.
func (h *Handlers) requirePathTabIDMatch(w http.ResponseWriter, r *http.Request, bodyTabID string) (string, bool) {
	tabID := r.PathValue("id")
	if tabID == "" {
		httpx.Error(w, 400, fmt.Errorf("tab id required"))
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
