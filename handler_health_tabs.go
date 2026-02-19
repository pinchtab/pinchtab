package main

import (
	"net/http"
	"time"
)

func (b *Bridge) handleHealth(w http.ResponseWriter, r *http.Request) {
	targets, err := b.ListTargets()
	if err != nil {
		jsonResp(w, 200, map[string]any{"status": "disconnected", "error": err.Error(), "cdp": cfg.CdpURL})
		return
	}
	jsonResp(w, 200, map[string]any{"status": "ok", "tabs": len(targets), "cdp": cfg.CdpURL})
}

func (b *Bridge) handleTabs(w http.ResponseWriter, r *http.Request) {
	targets, err := b.ListTargets()
	if err != nil {
		jsonErr(w, 500, err)
		return
	}

	tabs := make([]map[string]any, 0, len(targets))
	for _, t := range targets {
		entry := map[string]any{
			"id":    string(t.TargetID),
			"url":   t.URL,
			"title": t.Title,
			"type":  t.Type,
		}
		if lock := b.locks.Get(string(t.TargetID)); lock != nil {
			entry["owner"] = lock.Owner
			entry["lockedUntil"] = lock.ExpiresAt.Format(time.RFC3339)
		}
		tabs = append(tabs, entry)
	}
	jsonResp(w, 200, map[string]any{"tabs": tabs})
}
