package scheduler

import "testing"

func TestBuildActionBodyEnvelope(t *testing.T) {
	task := &Task{
		Action:   "click",
		Ref:      "e5",
		Selector: "css:button",
		TabID:    "tab-1",
		Params:   map[string]any{"button": "left", "x": 10},
	}

	body := buildActionBody(task)

	if body["kind"] != "click" {
		t.Errorf("kind = %v, want click", body["kind"])
	}
	if body["ref"] != "e5" {
		t.Errorf("ref = %v, want e5", body["ref"])
	}
	if body["selector"] != "css:button" {
		t.Errorf("selector = %v, want css:button", body["selector"])
	}
	if body["button"] != "left" {
		t.Errorf("button = %v, want left (Params passthrough)", body["button"])
	}
	if body["x"] != 10 {
		t.Errorf("x = %v, want 10 (Params passthrough)", body["x"])
	}
}

func TestBuildActionBodyReservedKeysNotOverlaid(t *testing.T) {
	// A caller must not be able to clobber envelope fields via Params.
	task := &Task{
		Action:   "click",
		Ref:      "e5",
		Selector: "css:button",
		TabID:    "tab-1",
		Params: map[string]any{
			"kind":     "evil",
			"ref":      "e999",
			"tabId":    "other-tab",
			"selector": "css:hacked",
			"safe":     "kept",
		},
	}

	body := buildActionBody(task)

	if body["kind"] != "click" {
		t.Errorf("kind = %v, want click (Params must not overlay)", body["kind"])
	}
	if body["ref"] != "e5" {
		t.Errorf("ref = %v, want e5 (Params must not overlay)", body["ref"])
	}
	if body["selector"] != "css:button" {
		t.Errorf("selector = %v, want css:button (Params must not overlay)", body["selector"])
	}
	if _, ok := body["tabId"]; ok {
		t.Errorf("tabId leaked from Params into body: %v", body["tabId"])
	}
	if body["safe"] != "kept" {
		t.Errorf("safe = %v, want kept (non-reserved Params pass through)", body["safe"])
	}
}

func TestBuildActionBodyOmitsEmptyEnvelope(t *testing.T) {
	task := &Task{Action: "snapshot"}
	body := buildActionBody(task)
	if _, ok := body["ref"]; ok {
		t.Error("empty ref should be omitted")
	}
	if _, ok := body["selector"]; ok {
		t.Error("empty selector should be omitted")
	}
}
