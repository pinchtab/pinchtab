package runtime

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestProxyAuthEnabled_DisabledWhenNoCredentials(t *testing.T) {
	cases := []struct {
		name string
		p    config.BrowserProxyConfig
	}{
		{
			name: "zero value",
			p:    config.BrowserProxyConfig{},
		},
		{
			name: "server only, no username",
			p:    config.BrowserProxyConfig{Server: "http://proxy.example:8080"},
		},
		{
			name: "server + password but no username (validator would reject)",
			p:    config.BrowserProxyConfig{Server: "http://proxy.example:8080", Password: "secret"},
		},
		{
			name: "empty server, credentials populated",
			p:    config.BrowserProxyConfig{Username: "user", Password: "pw"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if proxyAuthEnabled(tc.p) {
				t.Fatalf("proxyAuthEnabled(%+v) = true, want false", tc.p.Redacted())
			}
		})
	}
}

func TestProxyAuthEnabled_EnabledWhenCredentialsPresent(t *testing.T) {
	p := config.BrowserProxyConfig{
		Server:   "http://proxy.example:8080",
		Username: "user",
		Password: "pw",
	}
	if !proxyAuthEnabled(p) {
		t.Fatalf("proxyAuthEnabled(%+v) = false, want true", p.Redacted())
	}
}

// Cancelled context ensures the function bails before any CDP call.
func TestEnableProxyAuth_NoopWhenDisabled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := enableProxyAuth(ctx, config.BrowserProxyConfig{}); err != nil {
		t.Fatalf("enableProxyAuth on disabled proxy returned %v, want nil", err)
	}
	if err := enableProxyAuth(ctx, config.BrowserProxyConfig{Server: "http://p:1"}); err != nil {
		t.Fatalf("enableProxyAuth on credential-less proxy returned %v, want nil", err)
	}
}

func TestProxyAuth_PasswordNotLoggedViaRedacted(t *testing.T) {
	p := config.BrowserProxyConfig{
		Server:   "http://proxy.example:8080",
		Username: "user",
		Password: "super-secret",
	}
	r := p.Redacted()
	if r.Password == p.Password {
		t.Fatalf("Redacted() did not mask password: %q", r.Password)
	}
	if r.Password == "" {
		t.Fatalf("Redacted() should keep a placeholder for non-empty passwords")
	}
}

func TestRemoteProxyAuthRequiresExplicitOptIn(t *testing.T) {
	cfg := &config.RuntimeConfig{
		Proxy: config.BrowserProxyConfig{
			Server:   "http://proxy.example:8080",
			Username: "user",
			Password: "super-secret",
		},
	}

	var err error
	logs := captureSlog(t, func() {
		err = requireRemoteProxyAuthOptIn("ws://203.0.113.10:9222/devtools/browser/abc", cfg)
	})
	if err == nil {
		t.Fatal("expected remote proxy auth forwarding to be refused")
	}
	if !strings.Contains(err.Error(), "security.attach.forwardProxyAuth=true") {
		t.Fatalf("error should mention explicit opt-in, got %v", err)
	}
	if !strings.Contains(logs, "remote_cdp.proxy_auth_blocked") || !strings.Contains(logs, "203.0.113.10") {
		t.Fatalf("audit log missing blocked event/target host: %s", logs)
	}
	if !strings.Contains(logs, "proxy.example:8080") {
		t.Fatalf("audit log missing redacted proxy endpoint: %s", logs)
	}
	if strings.Contains(logs, "super-secret") {
		t.Fatalf("audit log leaked proxy password: %s", logs)
	}
}

func TestInitRemoteCDPRefusesProxyAuthBeforeProbe(t *testing.T) {
	cfg := &config.RuntimeConfig{
		AttachAllowHosts:   []string{"203.0.113.10"},
		AttachAllowSchemes: []string{"ws"},
		Proxy: config.BrowserProxyConfig{
			Server:   "http://proxy.example:8080",
			Username: "user",
			Password: "super-secret",
		},
	}

	var err error
	logs := captureSlog(t, func() {
		_, _, _, _, _, err = InitRemoteCDP(context.Background(), cfg, "ws://203.0.113.10:9222/devtools/browser/abc")
	})
	if err == nil {
		t.Fatal("expected remote CDP proxy auth forwarding to be refused")
	}
	if !strings.Contains(err.Error(), "security.attach.forwardProxyAuth=true") {
		t.Fatalf("error should mention explicit opt-in, got %v", err)
	}
	if !strings.Contains(logs, "remote_cdp.proxy_auth_blocked") || !strings.Contains(logs, "203.0.113.10") {
		t.Fatalf("audit log missing blocked event/target host: %s", logs)
	}
	if strings.Contains(logs, "super-secret") {
		t.Fatalf("audit log leaked proxy password: %s", logs)
	}
}

func TestInitRemoteCDPRejectsDisallowedAttachSchemeBeforeProbe(t *testing.T) {
	cfg := &config.RuntimeConfig{
		AttachAllowHosts:   []string{"*"},
		AttachAllowSchemes: []string{"ws"},
	}

	_, _, _, _, _, err := InitRemoteCDP(context.Background(), cfg, "http://203.0.113.10:9222")
	if err == nil {
		t.Fatal("expected disallowed attach scheme to be rejected")
	}
	if !strings.Contains(err.Error(), "scheme") || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("error should mention disallowed scheme, got %v", err)
	}
}

func TestRemoteProxyAuthAllowsExplicitOptInAndAuditsForward(t *testing.T) {
	cfg := &config.RuntimeConfig{
		AttachForwardProxyAuth: true,
		Proxy: config.BrowserProxyConfig{
			Server:   "http://proxy.example:8080",
			Username: "user",
			Password: "super-secret",
		},
	}

	if err := requireRemoteProxyAuthOptIn("ws://203.0.113.10:9222/devtools/browser/abc", cfg); err != nil {
		t.Fatalf("requireRemoteProxyAuthOptIn returned %v, want nil", err)
	}

	logs := captureSlog(t, func() {
		auditRemoteProxyAuthForward("ws://203.0.113.10:9222/devtools/browser/abc", cfg)
	})
	if !strings.Contains(logs, "remote_cdp.proxy_auth_forwarded") || !strings.Contains(logs, "203.0.113.10") {
		t.Fatalf("audit log missing forwarded event/target host: %s", logs)
	}
	if !strings.Contains(logs, "proxy.example:8080") {
		t.Fatalf("audit log missing redacted proxy endpoint: %s", logs)
	}
	if strings.Contains(logs, "super-secret") {
		t.Fatalf("audit log leaked proxy password: %s", logs)
	}
}

func TestRemoteProxyAuthNoopWithoutCredentials(t *testing.T) {
	cfg := &config.RuntimeConfig{
		Proxy: config.BrowserProxyConfig{Server: "http://proxy.example:8080"},
	}
	logs := captureSlog(t, func() {
		if err := requireRemoteProxyAuthOptIn("ws://203.0.113.10:9222/devtools/browser/abc", cfg); err != nil {
			t.Fatalf("requireRemoteProxyAuthOptIn returned %v, want nil", err)
		}
		auditRemoteProxyAuthForward("ws://203.0.113.10:9222/devtools/browser/abc", cfg)
	})
	if logs != "" {
		t.Fatalf("unexpected audit log without proxy credentials: %s", logs)
	}
}

func captureSlog(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	defer slog.SetDefault(old)
	fn()
	return buf.String()
}
