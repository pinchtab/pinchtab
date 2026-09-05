package handlers

import (
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"sort"
	"strings"

	"github.com/pinchtab/pinchtab/internal/httpx"
)

type queryContract struct {
	endpoint string
	keys     map[string]struct{}
}

func jsonKeysOf(t reflect.Type) map[string]struct{} {
	keys := make(map[string]struct{})
	for _, field := range reflect.VisibleFields(t) {
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if name != "" && name != "-" {
			keys[name] = struct{}{}
		}
	}
	return keys
}

func (c queryContract) unknown(q url.Values) []string {
	var unknown []string
	for key := range q {
		if _, known := c.keys[key]; known {
			continue
		}
		if strings.TrimSpace(q.Get(key)) == "" {
			continue
		}
		unknown = append(unknown, key)
	}
	sort.Strings(unknown)
	return unknown
}

func (c queryContract) unknownError(unknown []string) error {
	named := make([]string, 0, len(unknown))
	for _, key := range unknown {
		if near := c.nearest(key); near != "" {
			named = append(named, fmt.Sprintf("%s (did you mean %s?)", key, near))
			continue
		}
		named = append(named, key)
	}
	return fmt.Errorf("%s: not a parameter of %s and would be silently dropped; check the spelling or send the request as POST %s with a JSON body", strings.Join(named, ", "), c.endpoint, c.endpoint)
}

func (c queryContract) nearest(key string) string {
	if len(key) < 4 {
		return ""
	}
	best := ""
	bestDistance := 0
	for candidate := range c.keys {
		distance := editDistance(strings.ToLower(key), strings.ToLower(candidate))
		if best == "" || distance < bestDistance || (distance == bestDistance && candidate < best) {
			best, bestDistance = candidate, distance
		}
	}
	if bestDistance > 2 {
		return ""
	}
	return best
}

func editDistance(a, b string) int {
	previous := make([]int, len(b)+1)
	current := make([]int, len(b)+1)
	for j := range previous {
		previous[j] = j
	}
	for i := 1; i <= len(a); i++ {
		current[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			current[j] = min(previous[j]+1, min(current[j-1]+1, previous[j-1]+cost))
		}
		previous, current = current, previous
	}
	return previous[len(b)]
}

var routedPostQueryKeys = map[string]string{
	"browser": "the multi-instance router (orchestrator.ExtractRequestedBrowser) reads it from the query before any handler runs, and the MCP client's routedPath sends it on every POST",
}

func routedPostQueryKeySet() map[string]struct{} {
	keys := make(map[string]struct{}, len(routedPostQueryKeys))
	for key := range routedPostQueryKeys {
		keys[key] = struct{}{}
	}
	return keys
}

func refusePostQuery(w http.ResponseWriter, r *http.Request, endpoint string, body reflect.Type) bool {
	dropped := queryContract{endpoint: endpoint, keys: routedPostQueryKeySet()}.unknown(r.URL.Query())
	if len(dropped) == 0 {
		return true
	}
	bodyKeys := jsonKeysOf(body)
	named := make([]string, 0, len(dropped))
	for _, key := range dropped {
		if _, carried := bodyKeys[key]; carried {
			named = append(named, key+" (send it in the JSON body)")
			continue
		}
		named = append(named, key)
	}
	httpx.Error(w, 400, fmt.Errorf("%s: POST %s takes its fields from the JSON body and would silently drop this query parameter; name the tab and every other field in the body", strings.Join(named, ", "), endpoint))
	return false
}

var mistypedTabTargets = []string{"tab", "tabID", "tab_id"}

func mistypedTabTarget(q url.Values) error {
	for _, key := range mistypedTabTargets {
		if strings.TrimSpace(q.Get(key)) == "" {
			continue
		}
		return fmt.Errorf("%s: not a targeting parameter and would answer about the current tab instead of the one named; the tab is named by tabId", key)
	}
	return nil
}
