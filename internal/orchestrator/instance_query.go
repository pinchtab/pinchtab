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
)

func (o *Orchestrator) List() []bridge.Instance {
	o.mu.RLock()
	defer o.mu.RUnlock()

	result := make([]bridge.Instance, 0, len(o.instances))
	for _, inst := range o.instances {
		copyInst := inst.Instance
		if instanceIsActive(inst) && copyInst.Status == "stopped" {
			copyInst.Status = "running"
		}
		if !instanceIsActive(inst) &&
			(copyInst.Status == "starting" || copyInst.Status == "running" || copyInst.Status == "stopping") {
			copyInst.Status = "stopped"
		}

		result = append(result, copyInst)
	}
	return result
}

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

func (o *Orchestrator) FirstRunningURL() string {
	return o.firstRunningURL(nil)
}

func (o *Orchestrator) FirstRunningURLForBrowser(browser string) string {
	browser = strings.TrimSpace(browser)
	if browser == "" {
		return o.FirstRunningURL()
	}
	normalized := config.NormalizeBrowser(browser)
	url := o.firstRunningURL(func(inst *InstanceInternal) bool {
		return inst != nil && inst.Browser == normalized
	})
	if url != "" {
		return url
	}
	// Legacy fallback: match instances with empty Browser field.
	return o.firstRunningURL(func(inst *InstanceInternal) bool {
		return inst != nil && inst.Browser == ""
	})
}

func (o *Orchestrator) firstRunningURL(match func(*InstanceInternal) bool) string {
	o.mu.RLock()
	defer o.mu.RUnlock()
	// Collect running instances and sort by start time for determinism.
	// This works for both local launched instances and attached remote bridges.
	type candidate struct {
		start time.Time
		url   string
	}
	var candidates []candidate
	for _, inst := range o.instances {
		if inst.Status == "running" && instanceIsActive(inst) {
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
		resolved, err := config.ResolveDefaultBrowserTarget(o.runtimeCfg)
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

	// Validate the requested browser before normalization.
	if _, err := config.ParseBrowser(requested, nil); err != nil {
		return "", http.StatusBadRequest, fmt.Errorf("unknown browser %q", requested)
	}

	normalized := config.NormalizeBrowser(requested)
	if o.runtimeCfg != nil && len(o.runtimeCfg.Targets) > 0 {
		matches := config.TargetsForBrowser(o.runtimeCfg, requested)
		if len(matches) == 0 {
			return "", http.StatusBadRequest, fmt.Errorf("no browser target configured for browser %q", requested)
		}
		// Valid browser — check if there's a running instance for it.
		u := o.FirstRunningURLForBrowser(normalized)
		if u == "" {
			return "", http.StatusConflict, fmt.Errorf("no running instance for browser %q", requested)
		}
		return u, 0, nil
	}

	return o.FirstRunningURLForBrowser(normalized), 0, nil
}

// instanceTabsCached returns the tab list for inst using the per-instance
// cache. fresh=true forces a bypass and refresh. Returned tabs are
// InstanceTab-shaped (with InstanceID set) so callers can hand them
// straight to JSON encoders.
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
		if inst.Status == "running" && instanceIsActive(inst) {
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

// FindInstanceByTab returns the running instance owning tabID, or (nil,false).
func (o *Orchestrator) FindInstanceByTab(tabID string) (*bridge.Instance, bool) {
	if tabID == "" {
		return nil, false
	}

	o.mu.RLock()
	instances := make([]*InstanceInternal, 0, len(o.instances))
	for _, inst := range o.instances {
		if inst.Status == "running" && instanceIsActive(inst) {
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
		if inst.Status == "running" && instanceIsActive(inst) {
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
			InstanceID:    inst.ID,
			ProfileName:   inst.ProfileName,
			JSHeapUsedMB:  mem.JSHeapUsedMB,
			JSHeapTotalMB: mem.JSHeapTotalMB,
			Documents:     mem.Documents,
			Frames:        mem.Frames,
			Nodes:         mem.Nodes,
			Listeners:     mem.Listeners,
		})
	}
	return all
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
