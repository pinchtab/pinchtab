package mcp

import (
	"slices"
	"testing"
)

// clickFields is the decided pinchtab_click surface. button is deliberately absent: the
// click action never reads it, so declaring it would advertise an argument that does nothing.
var clickFields = []string{
	"browser", "dialogAction", "dialogText", "dismissBanners", "dismissKnownInterstitials",
	"element", "humanize", "mode", "nodeId", "onDialog", "promptText", "query", "ref",
	"selector", "snap", "submit", "tabId", "target", "waitNav", "x", "y",
}

func TestClickDeclaresExactlyTheDecidedFieldSet(t *testing.T) {
	for _, tool := range allTools() {
		if tool.Name != "pinchtab_click" {
			continue
		}
		var declared []string
		for name := range tool.InputSchema.Properties {
			declared = append(declared, name)
		}
		slices.Sort(declared)
		if !slices.Equal(declared, clickFields) {
			t.Fatalf("pinchtab_click declares %v, want the decided set %v; adding or dropping a click field is a surface decision, so update clickFields with the reason", declared, clickFields)
		}
		return
	}
	t.Fatal("pinchtab_click is not registered")
}

func TestClickForwardsTheCLIParityFields(t *testing.T) {
	srv := mockPinchTab()
	defer srv.Close()

	r := callTool(t, "pinchtab_click", map[string]any{
		"ref": "e5", "submit": true, "dismissKnownInterstitials": true, "waitNav": true, "dismissBanners": true,
	}, srv)
	if r.IsError {
		t.Fatalf("click refused the parity fields: %s", resultText(t, r))
	}
	body, _ := resultJSON(t, r)["body"].(map[string]any)
	req := decodeActionBody(t, body)
	if !req.Submit || !req.DismissKnownInterstitials || !req.WaitNav || !req.DismissBanners {
		t.Fatalf("posted %v, want submit, dismissKnownInterstitials, waitNav and dismissBanners forwarded", body)
	}
}
