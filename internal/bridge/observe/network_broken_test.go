package observe

import (
	"reflect"
	"testing"
)

func TestIsBrokenAssetMatrix(t *testing.T) {
	cases := []struct {
		name  string
		entry NetworkEntry
		want  bool
	}{
		{"ok document", NetworkEntry{Status: 200, ResourceType: "Document", Finished: true}, false},
		{"ok image", NetworkEntry{Status: 200, ResourceType: "Image", Finished: true}, false},
		{"redirect", NetworkEntry{Status: 302, ResourceType: "Document", Finished: true}, false},
		{"404 image", NetworkEntry{Status: 404, ResourceType: "Image", Finished: true}, true},
		{"404 script", NetworkEntry{Status: 404, ResourceType: "Script", Finished: true}, true},
		{"404 stylesheet", NetworkEntry{Status: 404, ResourceType: "Stylesheet", Finished: true}, true},
		{"404 fetch", NetworkEntry{Status: 404, ResourceType: "Fetch", Finished: true}, true},
		{"500 xhr", NetworkEntry{Status: 500, ResourceType: "XHR", Finished: true}, true},
		{"403 font", NetworkEntry{Status: 403, ResourceType: "Font", Finished: true}, true},
		{"network error", NetworkEntry{Failed: true, Error: "net::ERR_CONNECTION_REFUSED", ResourceType: "Script"}, true},
		{"aborted", NetworkEntry{Failed: true, Error: "net::ERR_ABORTED", ResourceType: "Image"}, true},
		{"in-flight", NetworkEntry{ResourceType: "Image"}, false},
	}
	for _, tc := range cases {
		if got := IsBrokenAsset(tc.entry); got != tc.want {
			t.Errorf("%s: IsBrokenAsset = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestBrokenAssetsClassification(t *testing.T) {
	entries := []NetworkEntry{
		{URL: "http://fixtures/audit-site/index.html", Status: 200, ResourceType: "Document", Finished: true},
		{URL: "http://fixtures/audit-site/assets/missing.png", Status: 404, ResourceType: "Image", Finished: true},
		{URL: "http://fixtures/audit-site/assets/missing.json", Status: 404, ResourceType: "Fetch", Finished: true},
		{URL: "http://fixtures/api", Failed: true, Error: "net::ERR_FAILED", ResourceType: "XHR"},
	}

	got := BrokenAssets(entries)
	want := []BrokenAsset{
		{URL: "http://fixtures/audit-site/assets/missing.png", ResourceType: "image", StatusCode: 404},
		{URL: "http://fixtures/audit-site/assets/missing.json", ResourceType: "fetch", StatusCode: 404},
		{URL: "http://fixtures/api", ResourceType: "xhr", StatusCode: 0, Error: "net::ERR_FAILED"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("BrokenAssets:\n got %+v\nwant %+v", got, want)
	}
}

func TestBrokenAssetsEmpty(t *testing.T) {
	got := BrokenAssets([]NetworkEntry{{URL: "http://fixtures/clean.html", Status: 200, ResourceType: "Document", Finished: true}})
	if len(got) != 0 {
		t.Errorf("BrokenAssets on clean entries = %+v, want empty", got)
	}
	if got == nil {
		t.Error("BrokenAssets should return an empty slice, not nil (JSON [])")
	}
}
