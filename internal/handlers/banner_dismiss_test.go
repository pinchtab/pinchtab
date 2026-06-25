package handlers

import (
	"context"
	"net/http/httptest"
	"strings"
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

func TestBannerDismissScriptDoesNotHardRemoveGenericDialogsOrOverlays(t *testing.T) {
	for _, forbidden := range []string{
		`[role="dialog"]`,
		`[aria-modal="true"]`,
		`[class*="overlay" i]`,
	} {
		if strings.Contains(bannerDismissJS, forbidden) {
			t.Fatalf("banner dismiss script must not hard-remove generic selector %s", forbidden)
		}
	}
}

func TestBannerDismissScriptStillTargetsCookieConsentContainers(t *testing.T) {
	for _, want := range []string{
		`[id*="cookie" i]`,
		`[class*="cookie" i]`,
		`[id*="consent" i]`,
		`[class*="consent" i]`,
	} {
		if !strings.Contains(bannerDismissJS, want) {
			t.Fatalf("banner dismiss script missing cookie/consent selector %s", want)
		}
	}
}
