package bridgekit

import (
	"context"
	"log/slog"

	"github.com/pinchtab/pinchtab/internal/bridge"
)

// populateEscalatedRefCache builds a ref cache for a newly escalated Chrome
// tab that maps the STATIC browser's ref names to Chrome's BackendNodeIDs.
// This is necessary because Chrome and the static browser assign different
// sequential ref numbers (e0, e1, ...) to the same page elements. We match
// by (role, name) to bridge the two numbering schemes.
func (a *BridgeAdapter) populateEscalatedRefCache(ctx context.Context, chromeTabID, liteTabID string) {
	if a.BridgeAPI.GetRefCache(chromeTabID) != nil {
		return
	}

	staticBrowser := a.proxy.StaticBrowser()
	if staticBrowser == nil {
		return
	}
	staticSnap, err := staticBrowser.Snapshot(ctx, liteTabID, "interactive")
	if err != nil || len(staticSnap.Nodes) == 0 {
		// Routine for non-lite ID aliasing; for real escalations a missing
		// ref cache surfaces later as "ref not found" — leave a trail.
		slog.Debug("escalated ref cache: static snapshot unavailable", "liteTab", liteTabID, "chromeTab", chromeTabID, "err", err)
		return
	}

	chromeNodes, err := bridge.FetchAXTree(ctx)
	if err != nil {
		slog.Debug("escalated ref cache: chrome AX tree fetch failed", "liteTab", liteTabID, "chromeTab", chromeTabID, "err", err)
		return
	}
	flat, _ := bridge.BuildSnapshot(chromeNodes, bridge.FilterInteractive, -1)
	_ = bridge.EnrichA11yNodesWithDOMMetadata(ctx, flat)

	type chromeEntry struct {
		node bridge.A11yNode
		used bool
	}
	chromeByKey := map[string][]*chromeEntry{}
	for i := range flat {
		key := flat[i].Role + "\x00" + flat[i].Name
		chromeByKey[key] = append(chromeByKey[key], &chromeEntry{node: flat[i]})
	}

	refs := make(map[string]int64, len(staticSnap.Nodes))
	targets := make(map[string]bridge.RefTarget, len(staticSnap.Nodes))
	for _, sn := range staticSnap.Nodes {
		if sn.Ref == "" {
			continue
		}
		key := sn.Role + "\x00" + sn.Name
		for _, entry := range chromeByKey[key] {
			if !entry.used && entry.node.NodeID != 0 {
				entry.used = true
				refs[sn.Ref] = entry.node.NodeID
				targets[sn.Ref] = bridge.RefTarget{
					BackendNodeID:  entry.node.NodeID,
					FrameID:        entry.node.FrameID,
					FrameURL:       entry.node.FrameURL,
					FrameName:      entry.node.FrameName,
					ChildFrameID:   entry.node.ChildFrameID,
					ChildFrameURL:  entry.node.ChildFrameURL,
					ChildFrameName: entry.node.ChildFrameName,
				}
				break
			}
		}
	}

	slog.Debug("populated escalated ref cache",
		"chromeTab", chromeTabID, "liteTab", liteTabID,
		"staticRefs", len(staticSnap.Nodes), "mapped", len(refs))

	a.SetRefCache(chromeTabID, &bridge.RefCache{
		Refs:    refs,
		Targets: targets,
		Nodes:   flat,
	})
}

func (a *BridgeAdapter) GetRefCache(tabID string) *bridge.RefCache {
	if cache := a.BridgeAPI.GetRefCache(tabID); cache != nil {
		return cache
	}
	if chromeID, ok := a.proxy.ChromeTabID(tabID); ok {
		return a.BridgeAPI.GetRefCache(chromeID)
	}
	return nil
}
