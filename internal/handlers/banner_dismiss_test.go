package handlers

import (
	"context"
	"net/http/httptest"
	"testing"
)

func TestHistoryDismissBannersFlag(t *testing.T) {
	cases := []struct {
		raw  string
		want bool
	}{
		{"true", true},
		{"True", true},
		{"TRUE", true},
		{"1", true},
		{"false", false},
		{"0", false},
		{"", false},
		{"yes", false},
	}
	for _, c := range cases {
		req := httptest.NewRequest("POST", "/back?dismissBanners="+c.raw, nil)
		got := historyDismissBannersFlag(req)
		if got != c.want {
			t.Errorf("dismissBanners=%q -> %v, want %v", c.raw, got, c.want)
		}
	}
}

// dismissBanners is a no-op when enabled=false or when tabID is empty. The
// helper guards against both so call-sites can wire the post-action hook
// unconditionally without checking the flag and without re-validating the
// resolved tab ID.
func TestDismissBannersDisabledIsNoop(t *testing.T) {
	h := &Handlers{}
	h.dismissBanners(context.Background(), "tab", false)
	h.dismissBanners(context.Background(), "", true)
}
