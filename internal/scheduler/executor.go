package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"

	"github.com/pinchtab/pinchtab/internal/activity"
)

// TaskExecutor runs a single task and returns its result, decoupling dispatch
// policy from the concrete action-endpoint transport.
type TaskExecutor interface {
	Execute(ctx context.Context, t *Task) (any, error)
}

// actionEndpointExecutor dispatches tasks to a tab instance's
// POST /tabs/{id}/action endpoint over localhost HTTP.
type actionEndpointExecutor struct {
	resolver InstanceResolver
	client   *http.Client
}

// reservedActionKeys are envelope fields owned by the task itself; a task's
// Params must never overlay them (otherwise a caller could smuggle e.g.
// {"kind":"evil"} via Params and clobber the dispatched action).
var reservedActionKeys = map[string]bool{
	"kind":     true,
	"ref":      true,
	"tabId":    true,
	"selector": true,
}

// buildActionBody assembles the /tabs/{id}/action request body for a task,
// matching the immediate-path action format. The envelope (kind/ref/selector)
// is set from the task's own fields and is protected from Params overlay.
func buildActionBody(t *Task) map[string]any {
	body := map[string]any{
		"kind": t.Action,
	}
	if t.Ref != "" {
		body["ref"] = t.Ref
	}
	if t.Selector != "" {
		body["selector"] = t.Selector
	}
	for k, v := range t.Params {
		if reservedActionKeys[k] {
			continue
		}
		body[k] = v
	}
	return body
}

func (e *actionEndpointExecutor) Execute(ctx context.Context, t *Task) (any, error) {
	if t.TabID == "" {
		return nil, fmt.Errorf("tabId is required for task execution")
	}

	port, err := e.resolver.ResolveTabInstance(t.TabID)
	if err != nil {
		return nil, fmt.Errorf("could not resolve tab %q: %w", t.TabID, err)
	}

	payload, err := json.Marshal(buildActionBody(t))
	if err != nil {
		return nil, fmt.Errorf("failed to encode task body: %w", err)
	}

	targetURL := &url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort("localhost", port),
		Path:   fmt.Sprintf("/tabs/%s/action", t.TabID),
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL.String(), bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(activity.HeaderPTSource, "scheduler")
	req.Header.Set(activity.HeaderPTTabID, t.TabID)
	if t.AgentID != "" {
		req.Header.Set(activity.HeaderAgentID, t.AgentID)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executor request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read executor response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("executor returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return string(respBody), nil
	}
	return result, nil
}
