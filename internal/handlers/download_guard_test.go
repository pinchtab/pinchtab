package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestTheDownloadAllowlistControlsTheDomainAndPrivateIPChecksTogether(t *testing.T) {
	const privateHost = "10.0.0.5"

	for _, tc := range []struct {
		name    string
		allowed []string
		want    bool
	}{
		{"a bare wildcard", []string{"*"}, true},
		{"a wildcard beside an unrelated host", []string{"*", "example.com"}, true},
		{"a wildcard beside the internal host", []string{"*", privateHost}, true},
		{"the internal host alone", []string{privateHost}, true},
		{"a wildcard subdomain", []string{"*.corp.example.com"}, false},
		{"no list at all", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			guard := newDownloadURLGuard(tc.allowed)
			allowInternal := guard.allowsHost(privateHost)
			if allowInternal != tc.want {
				t.Fatalf("allowsHost(%v) = %v, want %v", tc.allowed, allowInternal, tc.want)
			}

			// The predicate matters only through the dialler that consumes it: a
			// refused host must stop at the private-IP check, not at a connection.
			_, err := resolveDownloadDialIPs(context.Background(), privateHost, allowInternal)
			if tc.want {
				if err != nil {
					t.Errorf("the dialler refused a host the operator named: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), "blocked remote IP") {
				t.Errorf("the dialler answered %v, want the private-IP refusal", err)
			}
		})
	}
}

func TestABareWildcardStillLiftsTheDownloadDomainRestriction(t *testing.T) {
	const unlisted = "https://unlisted.example/file.bin"

	if err := newDownloadURLGuard([]string{"*"}).Validate(unlisted); err != nil {
		t.Errorf("a wildcard allowlist refused an unlisted public host: %v", err)
	}
	err := newDownloadURLGuard([]string{"listed.example"}).Validate(unlisted)
	if err == nil || !strings.Contains(err.Error(), "downloadAllowedDomains") {
		t.Errorf("a named allowlist admitted an unlisted host (%v); the row above would then prove nothing", err)
	}
}

func TestAnAllowlistedLoopbackHostGetsTheSameVerdictFromValidateAndTheDialler(t *testing.T) {
	stubDownloadHostResolution(t, func(ctx context.Context, network, host string) ([]net.IP, error) {
		if host == "localhost" {
			return []net.IP{net.ParseIP("127.0.0.1")}, nil
		}
		return nil, errors.New("not found")
	})

	for _, tc := range []struct {
		name        string
		allowed     []string
		host        string
		wantAllowed bool
		wantBlocked bool
	}{
		{"loopback literal named", []string{"127.0.0.1"}, "127.0.0.1", true, false},
		{"localhost named", []string{"localhost"}, "localhost", true, false},
		{"loopback beside a wildcard", []string{"*", "127.0.0.1"}, "127.0.0.1", true, false},
		{"loopback under a bare wildcard", []string{"*"}, "127.0.0.1", true, false},
		{"loopback absent from a named list", []string{"example.com"}, "127.0.0.1", false, true},
		{"loopback with no list", nil, "127.0.0.1", false, true},
		{"private host named", []string{"10.0.0.5"}, "10.0.0.5", true, false},
		{"private host absent from a named list", []string{"example.com"}, "10.0.0.5", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			guard := newDownloadURLGuard(tc.allowed)
			err := guard.Validate("http://" + tc.host + ":18791/dl.html")
			if (err == nil) != tc.wantAllowed {
				t.Fatalf("Validate answered %v, want allowed=%v", err, tc.wantAllowed)
			}
			if errors.Is(err, errDownloadHostBlocked) != tc.wantBlocked {
				t.Fatalf("Validate answered %v, want the blocked-host refusal=%v", err, tc.wantBlocked)
			}
			_, dialErr := resolveDownloadDialIPs(context.Background(), tc.host, guard.allowsHost(tc.host))
			if (dialErr == nil) != tc.wantAllowed {
				t.Fatalf("the dialler answered %v while Validate answered %v; the two layers must agree", dialErr, err)
			}
		})
	}
}

func TestABlockedDownloadHostRefusalNamesTheSettingAndTheRemedy(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{AllowDownload: true}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/download?url=http://127.0.0.1:18791/dl.html", nil)
	w := httptest.NewRecorder()
	h.HandleDownload(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400: %s", w.Code, w.Body.String())
	}
	var body struct {
		Code    string `json:"code"`
		Error   string `json:"error"`
		Details struct {
			Host    string `json:"host"`
			Setting string `json:"setting"`
			Remedy  string `json:"remedy"`
		} `json:"details"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v: %s", err, w.Body.String())
	}
	if body.Code != codeDownloadHostBlocked {
		t.Errorf("code %q, want %q", body.Code, codeDownloadHostBlocked)
	}
	if !strings.Contains(body.Error, "internal or blocked host") {
		t.Errorf("error %q lost the refusal", body.Error)
	}
	if body.Details.Host != "127.0.0.1" || body.Details.Setting != "security.downloadAllowedDomains" {
		t.Errorf("details name host %q setting %q", body.Details.Host, body.Details.Setting)
	}
	for _, want := range []string{"config set security.downloadAllowedDomains", "127.0.0.1", "server restart"} {
		if !strings.Contains(body.Details.Remedy, want) {
			t.Errorf("remedy %q lacks %q", body.Details.Remedy, want)
		}
	}
}
