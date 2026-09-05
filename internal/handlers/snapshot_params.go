package handlers

import (
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/pinchtab/pinchtab/internal/bridge"
)

// snapshotFormats are the output formats /snapshot can produce. An empty format means
// json, so it is not listed separately in the error text a caller sees.
var snapshotFormats = []string{"json", "compact", "text", "yaml"}

// snapshotFilters are the node subsets /snapshot can return. "all" and the empty string
// both mean the whole tree; bridge.FilterInteractive is the only value that narrows it. The
// boolean `interactive` parameter is an alias: true selects bridge.FilterInteractive, false
// the whole tree, and a value that contradicts an explicit `filter` is refused rather than
// resolved by precedence the caller cannot see.
var snapshotFilters = []string{"all", bridge.FilterInteractive}

// snapshotKnownParams is every query parameter HandleSnapshot reads. A parameter absent
// from this set is reported back to the caller rather than rejected: rejecting would break
// version skew in the direction it normally happens — a newer client sending a parameter an
// older server has not learned yet — and the value here is diagnostic, not enforcement.
var snapshotKnownParams = map[string]bool{
	"tabId": true,
	// browser is read by the routing prelude rather than by HandleSnapshot itself, which
	// is precisely why it has to be listed: a parameter the request path consumes
	// somewhere else still gets reported as ignored if this set only covers one file.
	"browser":      true,
	"filter":       true,
	"interactive":  true,
	"format":       true,
	"depth":        true,
	"maxTokens":    true,
	"diff":         true,
	"noAnimations": true,
	"selector":     true,
	"output":       true,
	"path":         true,
}

// SnapshotCostControls is the validated form of the parameters that decide what a snapshot
// costs. Every field is already normalised, so the handler compares values it produced
// rather than strings the caller typed.
type SnapshotCostControls struct {
	// Format is one of snapshotFormats, never empty.
	Format string
	// Filter is bridge.FilterInteractive or "" for the whole tree.
	Filter string
	// MaxTokens is -1 when the caller set no budget.
	MaxTokens int
	// MaxDepth is -1 when the caller set no depth limit.
	MaxDepth int
	// Ignored names the query parameters the server does not read, sorted. Returned to the
	// caller so a mistyped flag is visible rather than silently doing nothing.
	Ignored []string
}

// ParseSnapshotCostControls validates the cost controls and refuses to guess. Each of them
// used to fail OPEN toward the expensive answer — an unrecognised format fell through to
// json, a mis-cased filter returned the whole tree instead of the interactive subset, an
// unparseable budget or depth was dropped entirely — so a caller that mistyped its own cost
// control was not told, just charged more.
func ParseSnapshotCostControls(q url.Values) (SnapshotCostControls, error) {
	controls := SnapshotCostControls{Format: "json", MaxTokens: -1, MaxDepth: -1}
	if err := mistypedTabTarget(q); err != nil {
		return controls, err
	}

	format := normalizeParam(q.Get("format"))
	if format != "" {
		if !acceptsValue(snapshotFormats, format) {
			return controls, fmt.Errorf("format must be one of %s (got %q)", joinQuoted(snapshotFormats), q.Get("format"))
		}
		controls.Format = format
	}

	filterSet := false
	filter := normalizeParam(q.Get("filter"))
	if filter != "" {
		if !acceptsValue(snapshotFilters, filter) {
			return controls, fmt.Errorf("filter must be one of %s (got %q)", joinQuoted(snapshotFilters), q.Get("filter"))
		}
		filterSet = true
		if filter == bridge.FilterInteractive {
			controls.Filter = filter
		}
	}

	if raw := normalizeParam(q.Get("interactive")); raw != "" {
		want, err := strconv.ParseBool(raw)
		if err != nil {
			return controls, fmt.Errorf("interactive must be \"true\" or \"false\" (got %q)", q.Get("interactive"))
		}
		aliased := ""
		if want {
			aliased = bridge.FilterInteractive
		}
		if filterSet && aliased != controls.Filter {
			return controls, fmt.Errorf("filter=%q and interactive=%q ask for different subsets; send one or the other", q.Get("filter"), q.Get("interactive"))
		}
		controls.Filter = aliased
	}

	if raw := strings.TrimSpace(q.Get("maxTokens")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 {
			return controls, fmt.Errorf("maxTokens must be a positive whole number (got %q)", q.Get("maxTokens"))
		}
		controls.MaxTokens = value
	}

	if raw := strings.TrimSpace(q.Get("depth")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < -1 {
			return controls, fmt.Errorf("depth must be a whole number >= -1, where -1 means no limit (got %q)", q.Get("depth"))
		}
		controls.MaxDepth = value
	}

	for name := range q {
		if !snapshotKnownParams[name] {
			controls.Ignored = append(controls.Ignored, name)
		}
	}
	sort.Strings(controls.Ignored)

	return controls, nil
}

// normalizeParam is the one place a caller's spelling is reconciled with ours, so `format`
// and `filter` cannot end up comparing differently-normalised strings.
func normalizeParam(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}

func acceptsValue(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

func joinQuoted(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, v := range values {
		quoted = append(quoted, strconv.Quote(v))
	}
	return strings.Join(quoted, ", ")
}
