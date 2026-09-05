package orchestrator

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/api/types"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/config/workflow"
)

// effectiveInstanceStatus reconciles a stored instance Status with whether the
// instance is actually active, so a stale "stopped" on a live instance reads as
// "running" and a non-live "starting/running/stopping" reads as "stopped".
func effectiveInstanceStatus(status string, active bool) string {
	if active && status == "stopped" {
		return "running"
	}
	if !active &&
		(status == "starting" || status == "running" || status == "stopping") {
		return "stopped"
	}
	return status
}

func (o *Orchestrator) List() []bridge.Instance {
	o.mu.RLock()
	defer o.mu.RUnlock()

	result := make([]bridge.Instance, 0, len(o.instances))
	for _, inst := range o.instances {
		copyInst := inst.Instance
		copyInst.Status = effectiveInstanceStatus(copyInst.Status, instanceIsActive(inst))
		if crashes, ok := o.crashes[inst.ID]; ok && crashes.Total > 0 {
			summary := crashes
			copyInst.Crashes = &summary
		}
		result = append(result, copyInst)
	}
	return result
}

// RefreshCrashes asks every live instance for its crash record and its enforced
// security posture and keeps the answers, so List can carry the crashes and the
// dashboard can report what each instance is actually enforcing. Both are known
// only to the process that owns the browser, which in server mode is never this
// one.
func (o *Orchestrator) RefreshCrashes() map[string]bridge.CrashSummary {
	o.refreshInstanceHealth()
	o.mu.RLock()
	defer o.mu.RUnlock()
	fresh := make(map[string]bridge.CrashSummary, len(o.crashes))
	for id, crashes := range o.crashes {
		fresh[id] = crashes
	}
	return fresh
}

// EnforcedSecurity reports each running instance's enforced posture. An instance
// that could not be queried maps to nil: unknown, which is not the same answer as
// an instance enforcing nothing.
func (o *Orchestrator) EnforcedSecurity() map[string]*workflow.EnforcedSecurity {
	o.refreshInstanceHealth()
	o.mu.RLock()
	defer o.mu.RUnlock()
	out := make(map[string]*workflow.EnforcedSecurity, len(o.enforced))
	for id, posture := range o.enforced {
		out[id] = posture
	}
	return out
}

// instanceHealthTTL bounds how often one /health poll of the front door fans out
// to every instance: crashes and enforced posture ride the same instance /health,
// so the two readers share one round of fetches.
const instanceHealthTTL = 2 * time.Second

func (o *Orchestrator) refreshInstanceHealth() {
	o.mu.RLock()
	stale := time.Since(o.enforcedAt) >= instanceHealthTTL
	instances := make([]*InstanceInternal, 0, len(o.instances))
	for _, inst := range o.instances {
		if instanceIsRunning(inst) {
			instances = append(instances, inst)
		}
	}
	o.mu.RUnlock()
	if !stale {
		return
	}

	crashes := make(map[string]bridge.CrashSummary, len(instances))
	enforced := make(map[string]*workflow.EnforcedSecurity, len(instances))
	for _, inst := range instances {
		health, err := o.fetchHealth(inst)
		if err != nil || health == nil {
			enforced[inst.ID] = nil
			continue
		}
		enforced[inst.ID] = health.Security
		if health.Crashes == nil {
			continue
		}
		for i := range health.Crashes.Recent {
			health.Crashes.Recent[i].InstanceID = inst.ID
		}
		crashes[inst.ID] = *health.Crashes
	}

	o.mu.Lock()
	if o.crashes == nil {
		o.crashes = map[string]bridge.CrashSummary{}
	}
	for id, summary := range crashes {
		o.crashes[id] = summary
	}
	o.enforced = enforced
	o.enforcedAt = time.Now()
	o.mu.Unlock()
}

// CrashSummary merges the instances' crash records into the shape bridge /health
// carries, each event naming its instance.
func (o *Orchestrator) CrashSummary() bridge.CrashSummary {
	o.refreshInstanceHealth()
	o.mu.RLock()
	defer o.mu.RUnlock()
	var merged bridge.CrashSummary
	for _, crashes := range o.crashes {
		merged.Total += crashes.Total
		merged.Recent = append(merged.Recent, crashes.Recent...)
	}
	sort.SliceStable(merged.Recent, func(i, j int) bool { return merged.Recent[i].Time.Before(merged.Recent[j].Time) })
	if len(merged.Recent) > maxMergedCrashEvents {
		merged.Recent = merged.Recent[len(merged.Recent)-maxMergedCrashEvents:]
	}
	return merged
}

const maxMergedCrashEvents = 20

func (o *Orchestrator) Logs(id string) (string, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	inst, ok := o.instances[id]
	if !ok {
		return "", fmt.Errorf("instance %q not found", id)
	}
	if inst.logBuf == nil {
		return "", nil
	}
	return inst.logBuf.String(), nil
}

func (o *Orchestrator) LogsSince(id string, offset uint64) (chunk string, newOffset uint64, reset bool, err error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	inst, ok := o.instances[id]
	if !ok {
		return "", 0, false, fmt.Errorf("instance %q not found", id)
	}
	if inst.logBuf == nil {
		return "", 0, false, nil
	}
	c, n, rs := inst.logBuf.since(offset)
	return c, n, rs, nil
}

func (o *Orchestrator) FirstRunningURL() string {
	return o.firstRunningURL(nil)
}

func (o *Orchestrator) FirstRunningURLForBrowser(browser string) string {
	browser = strings.TrimSpace(browser)
	if browser == "" {
		return o.FirstRunningURL()
	}
	normalized := config.NormalizeBrowser(browser)
	// No legacy empty-Browser fallback here: a request for a SPECIFIC
	// browser must not be routed to an instance of unknown provenance.
	// Browserless legacy instances stay reachable via FirstRunningURL.
	return o.firstRunningURL(func(inst *InstanceInternal) bool {
		return inst != nil && inst.Browser == normalized
	})
}

func (o *Orchestrator) firstRunningURL(match func(*InstanceInternal) bool) string {
	o.mu.RLock()
	defer o.mu.RUnlock()
	// Collect running instances and sort by start time for determinism.
	type candidate struct {
		start time.Time
		url   string
	}
	var candidates []candidate
	for _, inst := range o.instances {
		if instanceIsRunning(inst) {
			if inst.URL == "" {
				continue
			}
			if match != nil && !match(inst) {
				continue
			}
			candidates = append(candidates, candidate{start: inst.StartTime, url: inst.URL})
		}
	}
	if len(candidates) == 0 {
		return ""
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].start.Equal(candidates[j].start) {
			return candidates[i].url < candidates[j].url
		}
		return candidates[i].start.Before(candidates[j].start)
	})
	return candidates[0].url
}

func (o *Orchestrator) FirstRunningURLForRequest(r *http.Request) (string, int, error) {
	requested := ExtractRequestedBrowser(r)
	if requested == "" {
		resolved, err := config.ResolveDefaultBrowserTarget(o.cfg())
		if err != nil {
			return "", http.StatusBadRequest, err
		}
		if resolved != nil && !resolved.Legacy {
			if resolved.Provider == "" {
				return "", http.StatusBadRequest, fmt.Errorf("no default browser target configured and none requested")
			}
			return o.FirstRunningURLForBrowser(resolved.Provider), 0, nil
		}
		return o.FirstRunningURL(), 0, nil
	}

	if _, err := config.ParseBrowser(requested, nil); err != nil {
		return "", http.StatusBadRequest, fmt.Errorf("unknown browser %q", requested)
	}

	normalized := config.NormalizeBrowser(requested)
	if cfg := o.cfg(); cfg != nil && len(cfg.Targets) > 0 {
		matches := config.TargetsForBrowser(cfg, requested)
		if len(matches) == 0 {
			return "", http.StatusBadRequest, fmt.Errorf("no browser target configured for browser %q", requested)
		}
		u := o.FirstRunningURLForBrowser(normalized)
		if u == "" {
			return "", http.StatusConflict, fmt.Errorf("no running instance for browser %q", requested)
		}
		return u, 0, nil
	}

	if u := o.FirstRunningURLForBrowser(normalized); u != "" {
		return u, 0, nil
	}
	// The requested browser has no running instance. If a single instance with a
	// different browser is running, this browser-pinned server can't serve the
	// request: return a clear conflict instead of an empty result the caller
	// would poll on until a misleading "instance not ready" timeout. On-demand
	// cross-browser launch is the multi-instance (simple) strategy's job; a
	// single always-on server serves one browser.
	if only := o.singleRunningInstance(); only != nil && only.Browser != "" &&
		config.NormalizeBrowser(only.Browser) != normalized {
		return "", http.StatusConflict, fmt.Errorf(
			"browser %q requested but %q is already running; restart the server with --browser %s or launch an instance with that browser",
			requested, only.Browser, requested)
	}
	return "", 0, nil
}

func (o *Orchestrator) instanceTabsCached(inst *InstanceInternal, fresh bool) ([]bridge.InstanceTab, error) {
	if inst == nil {
		return nil, fmt.Errorf("nil instance")
	}
	if !fresh {
		if cached, ok := o.tabsCache.Get(inst.ID); ok {
			return cached, nil
		}
	}
	tabs, err := o.fetchTabs(inst)
	if err != nil {
		return nil, err
	}
	out := make([]bridge.InstanceTab, 0, len(tabs))
	for _, tab := range tabs {
		out = append(out, bridge.InstanceTab{
			ID:         tab.ID,
			InstanceID: inst.ID,
			URL:        tab.URL,
			Title:      tab.Title,
		})
	}
	o.tabsCache.Set(inst.ID, out)
	return out, nil
}

func (o *Orchestrator) AllTabs() []bridge.InstanceTab {
	return o.allTabs(false)
}

func (o *Orchestrator) allTabs(fresh bool) []bridge.InstanceTab {
	o.mu.RLock()
	instances := make([]*InstanceInternal, 0)
	for _, inst := range o.instances {
		if instanceIsRunning(inst) {
			instances = append(instances, inst)
		}
	}
	o.mu.RUnlock()

	all := make([]bridge.InstanceTab, 0)
	for _, inst := range instances {
		tabs, err := o.instanceTabsCached(inst, fresh)
		if err != nil {
			continue
		}
		all = append(all, tabs...)
	}
	return all
}

func (o *Orchestrator) FindInstanceByTab(tabID string) (*bridge.Instance, bool) {
	if tabID == "" {
		return nil, false
	}

	o.mu.RLock()
	instances := make([]*InstanceInternal, 0, len(o.instances))
	for _, inst := range o.instances {
		if instanceIsRunning(inst) {
			instances = append(instances, inst)
		}
	}
	o.mu.RUnlock()

	for _, inst := range instances {
		tabs, err := o.instanceTabsCached(inst, false)
		if err != nil {
			continue
		}
		for _, tab := range tabs {
			if tab.ID == tabID {
				copyInst := inst.Instance
				return &copyInst, true
			}
		}
	}
	return nil, false
}

func (o *Orchestrator) AllMetrics() []types.InstanceMetrics {
	o.mu.RLock()
	instances := make([]*InstanceInternal, 0)
	for _, inst := range o.instances {
		if instanceIsRunning(inst) {
			instances = append(instances, inst)
		}
	}
	o.mu.RUnlock()

	all := make([]types.InstanceMetrics, 0)
	for _, inst := range instances {
		mem, err := o.fetchMetrics(inst)
		if err != nil || mem == nil {
			continue
		}
		all = append(all, types.InstanceMetrics{
			InstanceID:        inst.ID,
			ProfileName:       inst.ProfileName,
			MemoryMB:          mem.MemoryMB,
			Renderers:         mem.Renderers,
			Page:              pageMetricsDTO(mem.Page),
			UnreadableTargets: mem.UnreadableTargets,
		})
	}
	return all
}

func pageMetricsDTO(page *bridge.PageMetrics) *types.PageMetrics {
	if page == nil {
		return nil
	}
	return &types.PageMetrics{
		Targets:          page.Targets,
		JSHeapUsedMB:     page.JSHeapUsedMB,
		JSHeapTotalMB:    page.JSHeapTotalMB,
		Documents:        page.Documents,
		Frames:           page.Frames,
		Nodes:            page.Nodes,
		JSEventListeners: page.JSEventListeners,
	}
}

func (o *Orchestrator) ScreencastURL(instanceID, tabID string) string {
	o.mu.RLock()
	defer o.mu.RUnlock()
	inst, ok := o.instances[instanceID]
	if !ok {
		return ""
	}
	target, err := o.instancePathURL(inst, "/screencast", "tabId="+url.QueryEscape(tabID))
	if err != nil {
		return ""
	}
	switch target.Scheme {
	case "https":
		target.Scheme = "wss"
	default:
		target.Scheme = "ws"
	}
	return target.String()
}
