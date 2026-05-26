package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateBrowserProxy_TableDriven(t *testing.T) {
	tests := []struct {
		name    string
		proxy   BrowserProxyConfig
		wantErr bool
		wantSub string
	}{
		{name: "empty disables proxy", proxy: BrowserProxyConfig{}},
		{name: "good http",
			proxy: BrowserProxyConfig{Server: "http://proxy.example.com:8080"}},
		{name: "good https",
			proxy: BrowserProxyConfig{Server: "https://proxy.example.com:8443"}},
		{name: "good socks5",
			proxy: BrowserProxyConfig{Server: "socks5://10.0.0.1:1080"}},
		{name: "good socks4",
			proxy: BrowserProxyConfig{Server: "socks4://10.0.0.1:1080"}},
		{name: "good with bypass list",
			proxy: BrowserProxyConfig{
				Server:     "http://proxy.example.com:8080",
				BypassList: []string{"*.local", "127.0.0.1"},
			}},
		{name: "good with credentials",
			proxy: BrowserProxyConfig{
				Server:   "http://proxy.example.com:8080",
				Username: "alice",
				Password: "s3cr3t",
			}},
		{name: "bad scheme",
			proxy:   BrowserProxyConfig{Server: "ftp://proxy.example.com:21"},
			wantErr: true, wantSub: "scheme"},
		{name: "missing scheme",
			proxy:   BrowserProxyConfig{Server: "proxy.example.com:8080"},
			wantErr: true, wantSub: "scheme"},
		{name: "missing host",
			proxy:   BrowserProxyConfig{Server: "http://:8080"},
			wantErr: true, wantSub: "host"},
		{name: "missing port",
			proxy:   BrowserProxyConfig{Server: "http://proxy.example.com"},
			wantErr: true, wantSub: "host:port"},
		{name: "port out of range",
			proxy:   BrowserProxyConfig{Server: "http://proxy.example.com:99999"},
			wantErr: true, wantSub: "port"},
		{name: "embedded credentials rejected",
			proxy:   BrowserProxyConfig{Server: "http://user:pass@proxy.example.com:8080"},
			wantErr: true, wantSub: "embedded credentials"},
		{name: "username without password",
			proxy: BrowserProxyConfig{
				Server:   "http://proxy.example.com:8080",
				Username: "alice",
			},
			wantErr: true, wantSub: "password is required"},
		{name: "password without username",
			proxy: BrowserProxyConfig{
				Server:   "http://proxy.example.com:8080",
				Password: "secret",
			},
			wantErr: true, wantSub: "username is required"},
		{name: "bypass with whitespace",
			proxy: BrowserProxyConfig{
				Server:     "http://proxy.example.com:8080",
				BypassList: []string{"foo bar"},
			},
			wantErr: true, wantSub: "whitespace"},
		{name: "bypass with semicolon",
			proxy: BrowserProxyConfig{
				Server:     "http://proxy.example.com:8080",
				BypassList: []string{"a;b"},
			},
			wantErr: true, wantSub: "';'"},
		{name: "empty bypass entry",
			proxy: BrowserProxyConfig{
				Server:     "http://proxy.example.com:8080",
				BypassList: []string{""},
			},
			wantErr: true, wantSub: "must not be empty"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateBrowserProxy("browser.proxy", tt.proxy)
			if tt.wantErr {
				if len(errs) == 0 {
					t.Fatalf("expected validation error, got none")
				}
				if tt.wantSub != "" {
					joined := ""
					for _, e := range errs {
						joined += e.Error() + "\n"
					}
					if !strings.Contains(joined, tt.wantSub) {
						t.Errorf("expected error to contain %q, got: %s", tt.wantSub, joined)
					}
				}
				return
			}
			if len(errs) > 0 {
				t.Fatalf("unexpected validation errors: %v", errs)
			}
		})
	}
}

func TestValidateBrowserProxy_GeoBlock(t *testing.T) {
	t.Run("good geo accepted", func(t *testing.T) {
		errs := ValidateBrowserProxy("browser.proxy", BrowserProxyConfig{
			Server: "http://proxy.example.com:8080",
			Geo: &BrowserProxyGeoConfig{
				Timezone:   "Europe/London",
				Locale:     "en-GB",
				WebRTCIP:   "203.0.113.7",
				CountryISO: "GB",
			},
		})
		if len(errs) != 0 {
			t.Fatalf("expected no errors, got %v", errs)
		}
	})
	t.Run("bad timezone rejected", func(t *testing.T) {
		errs := ValidateBrowserProxy("browser.proxy", BrowserProxyConfig{
			Server: "http://proxy.example.com:8080",
			Geo:    &BrowserProxyGeoConfig{Timezone: "Not/AZone"},
		})
		if len(errs) == 0 {
			t.Fatal("expected validation error for bad timezone")
		}
	})
	t.Run("bad locale rejected", func(t *testing.T) {
		errs := ValidateBrowserProxy("browser.proxy", BrowserProxyConfig{
			Server: "http://proxy.example.com:8080",
			Geo:    &BrowserProxyGeoConfig{Locale: "EN-gb"},
		})
		if len(errs) == 0 {
			t.Fatal("expected validation error for bad locale")
		}
	})
	t.Run("bad webrtc IP rejected", func(t *testing.T) {
		errs := ValidateBrowserProxy("browser.proxy", BrowserProxyConfig{
			Server: "http://proxy.example.com:8080",
			Geo:    &BrowserProxyGeoConfig{WebRTCIP: "not-an-ip"},
		})
		if len(errs) == 0 {
			t.Fatal("expected validation error for bad webrtc IP")
		}
	})
	t.Run("empty geo block accepted", func(t *testing.T) {
		errs := ValidateBrowserProxy("browser.proxy", BrowserProxyConfig{
			Server: "http://proxy.example.com:8080",
			Geo:    &BrowserProxyGeoConfig{},
		})
		if len(errs) != 0 {
			t.Fatalf("empty geo should not fail validation, got %v", errs)
		}
	})
}

func TestMigrateLegacyBrowserConfig_GeoCarriesIntoSynthesizedTarget(t *testing.T) {
	bc := &BrowserConfig{
		Provider: BrowserChrome,
		Proxy: BrowserProxyConfig{
			Server: "http://proxy.example.com:8080",
			Geo: &BrowserProxyGeoConfig{
				Timezone: "Europe/London",
				Locale:   "en-GB",
			},
		},
	}
	synthesized, conflict := migrateLegacyBrowserConfig(bc)
	if !synthesized || conflict {
		t.Fatalf("expected synthesized=true conflict=false, got %v/%v", synthesized, conflict)
	}
	target, ok := bc.Targets[DefaultBrowserTargetName]
	if !ok {
		t.Fatal("default target not synthesized")
	}
	if target.Proxy.Geo == nil {
		t.Fatal("geo block missing from synthesized target")
	}
	if target.Proxy.Geo.Timezone != "Europe/London" || target.Proxy.Geo.Locale != "en-GB" {
		t.Errorf("geo not migrated correctly: %+v", target.Proxy.Geo)
	}
}

func TestBrowserProxyRedacted(t *testing.T) {
	t.Run("masks non-empty password", func(t *testing.T) {
		p := BrowserProxyConfig{
			Server:     "http://proxy.example.com:8080",
			BypassList: []string{"*.local"},
			Username:   "alice",
			Password:   "s3cr3t",
		}
		r := p.Redacted()
		if r.Password != "***" {
			t.Errorf("expected redacted password '***', got %q", r.Password)
		}
		if p.Password != "s3cr3t" {
			t.Errorf("original password mutated: %q", p.Password)
		}
		if r.Server != p.Server || r.Username != p.Username {
			t.Errorf("non-secret fields mutated: %+v", r)
		}
		if len(r.BypassList) != 1 || r.BypassList[0] != "*.local" {
			t.Errorf("bypass list lost: %+v", r.BypassList)
		}
		r.BypassList[0] = "mutated"
		if p.BypassList[0] != "*.local" {
			t.Errorf("redaction did not deep-copy bypass list")
		}
	})

	t.Run("empty password stays empty", func(t *testing.T) {
		p := BrowserProxyConfig{Server: "http://proxy.example.com:8080"}
		r := p.Redacted()
		if r.Password != "" {
			t.Errorf("expected empty password, got %q", r.Password)
		}
	})
}

func TestBrowserProxyFlags(t *testing.T) {
	t.Run("disabled proxy emits no flags", func(t *testing.T) {
		flags := BrowserProxyFlags(BrowserProxyConfig{})
		if len(flags) != 0 {
			t.Fatalf("expected no flags, got %v", flags)
		}
	})

	t.Run("server only", func(t *testing.T) {
		flags := BrowserProxyFlags(BrowserProxyConfig{Server: "http://proxy.example.com:8080"})
		if len(flags) != 1 {
			t.Fatalf("expected 1 flag, got %v", flags)
		}
		want := "--proxy-server=http://proxy.example.com:8080"
		if flags[0] != want {
			t.Errorf("want %q, got %q", want, flags[0])
		}
	})

	t.Run("with bypass list", func(t *testing.T) {
		flags := BrowserProxyFlags(BrowserProxyConfig{
			Server:     "socks5://10.0.0.1:1080",
			BypassList: []string{"*.local", "127.0.0.1"},
		})
		if len(flags) != 2 {
			t.Fatalf("expected 2 flags, got %v", flags)
		}
		if flags[0] != "--proxy-server=socks5://10.0.0.1:1080" {
			t.Errorf("unexpected flag[0]: %q", flags[0])
		}
		if flags[1] != "--proxy-bypass-list=*.local;127.0.0.1" {
			t.Errorf("unexpected flag[1]: %q", flags[1])
		}
	})

	t.Run("ipv6 host stays bracketed", func(t *testing.T) {
		flags := BrowserProxyFlags(BrowserProxyConfig{Server: "http://[2001:db8::1]:8080"})
		if len(flags) != 1 {
			t.Fatalf("expected 1 flag, got %v", flags)
		}
		if flags[0] != "--proxy-server=http://[2001:db8::1]:8080" {
			t.Errorf("unexpected IPv6 proxy flag: %q", flags[0])
		}
	})

	t.Run("credentials never leak into flag", func(t *testing.T) {
		flags := BrowserProxyFlags(BrowserProxyConfig{
			Server:   "http://proxy.example.com:8080",
			Username: "alice",
			Password: "s3cr3t",
		})
		for _, f := range flags {
			if strings.Contains(f, "alice") || strings.Contains(f, "s3cr3t") {
				t.Errorf("credential leaked into Chrome flag: %q", f)
			}
		}
	})
}

func TestMigrateLegacyBrowserConfig_ProxyCarriesIntoSynthesizedTarget(t *testing.T) {
	bc := &BrowserConfig{
		Provider: BrowserChrome,
		Proxy: BrowserProxyConfig{
			Server:   "http://proxy.example.com:8080",
			Username: "alice",
			Password: "s3cr3t",
		},
	}
	synthesized, conflict := migrateLegacyBrowserConfig(bc)
	if !synthesized || conflict {
		t.Fatalf("expected synthesized=true conflict=false, got synthesized=%v conflict=%v", synthesized, conflict)
	}
	target, ok := bc.Targets[DefaultBrowserTargetName]
	if !ok {
		t.Fatalf("default target was not synthesized")
	}
	if target.Proxy.Server != "http://proxy.example.com:8080" {
		t.Errorf("proxy server not migrated, got %q", target.Proxy.Server)
	}
	if target.Proxy.Password != "s3cr3t" {
		t.Errorf("proxy password not migrated, got %q", target.Proxy.Password)
	}
}

func TestApplyTargetOverride_TargetProxyReplacesGlobal(t *testing.T) {
	cfg := &RuntimeConfig{
		DefaultBrowser: BrowserChrome,
		Proxy: BrowserProxyConfig{
			Server:     "http://global.example.com:8080",
			Username:   "global-user",
			Password:   "global-pass",
			BypassList: []string{"*.global.test"},
		},
		Targets: BrowserTargetsConfig{
			"with-proxy": {
				Provider: BrowserChrome,
				Proxy: BrowserProxyConfig{
					Server:     "socks5://target.example.com:1080",
					BypassList: []string{"*.target.test"},
				},
			},
			"inherits": {
				Provider: BrowserChrome,
			},
		},
	}

	t.Run("target proxy replaces global entirely", func(t *testing.T) {
		out := ApplyTargetOverride(cfg, "with-proxy")
		if out.Proxy.Server != "socks5://target.example.com:1080" {
			t.Errorf("server not overridden, got %q", out.Proxy.Server)
		}
		if out.Proxy.Username != "" || out.Proxy.Password != "" {
			t.Errorf("global credentials leaked into target proxy: %+v", out.Proxy)
		}
		if len(out.Proxy.BypassList) != 1 || out.Proxy.BypassList[0] != "*.target.test" {
			t.Errorf("target bypass list not used: %+v", out.Proxy.BypassList)
		}
		if cfg.Proxy.Server != "http://global.example.com:8080" {
			t.Errorf("input cfg.Proxy mutated: %+v", cfg.Proxy)
		}
	})

	t.Run("target without proxy inherits global", func(t *testing.T) {
		out := ApplyTargetOverride(cfg, "inherits")
		if out.Proxy.Server != "http://global.example.com:8080" {
			t.Errorf("expected to inherit global, got %q", out.Proxy.Server)
		}
		if out.Proxy.Username != "global-user" {
			t.Errorf("expected to inherit global credentials")
		}
	})
}

func TestApplyFileConfigToRuntime_ProxyGeoCopied(t *testing.T) {
	cfg := &RuntimeConfig{}
	fc := &FileConfig{
		Browser: BrowserConfig{
			Proxy: BrowserProxyConfig{
				Server: "http://proxy.example.com:8080",
				Geo: &BrowserProxyGeoConfig{
					Timezone:   "Europe/London",
					Locale:     "en-GB",
					WebRTCIP:   "203.0.113.7",
					CountryISO: "GB",
				},
			},
		},
	}

	ApplyFileConfigToRuntime(cfg, fc)

	if cfg.Proxy.Geo == nil {
		t.Fatal("proxy geo was not copied to runtime config")
	}
	if *cfg.Proxy.Geo != *fc.Browser.Proxy.Geo {
		t.Fatalf("runtime proxy geo = %+v, want %+v", *cfg.Proxy.Geo, *fc.Browser.Proxy.Geo)
	}
	fc.Browser.Proxy.Geo.Timezone = "America/New_York"
	if cfg.Proxy.Geo.Timezone != "Europe/London" {
		t.Fatalf("runtime proxy geo aliases file config: %+v", cfg.Proxy.Geo)
	}
}

func TestFileConfig_ProxyRoundTrip(t *testing.T) {
	t.Run("proxy omitted when empty", func(t *testing.T) {
		fc := FileConfig{
			Browser: BrowserConfig{Provider: BrowserChrome},
		}
		raw, err := json.Marshal(fc)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if strings.Contains(string(raw), `"proxy"`) {
			t.Errorf("empty proxy should be omitted, got: %s", string(raw))
		}
	})

	t.Run("geo omitted when nil", func(t *testing.T) {
		fc := FileConfig{
			Browser: BrowserConfig{
				Provider: BrowserChrome,
				Proxy: BrowserProxyConfig{
					Server: "http://proxy.example.com:8080",
				},
			},
		}
		raw, err := json.Marshal(fc)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if strings.Contains(string(raw), `"geo"`) {
			t.Errorf("nil geo should be omitted, got: %s", string(raw))
		}
	})

	t.Run("geo round-trips when configured", func(t *testing.T) {
		fc := FileConfig{
			Browser: BrowserConfig{
				Provider: BrowserChrome,
				Proxy: BrowserProxyConfig{
					Server: "http://proxy.example.com:8080",
					Geo: &BrowserProxyGeoConfig{
						Timezone:   "Europe/London",
						Locale:     "en-GB",
						WebRTCIP:   "203.0.113.7",
						CountryISO: "GB",
					},
				},
			},
		}
		raw, err := json.Marshal(fc)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var rt FileConfig
		if err := json.Unmarshal(raw, &rt); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if rt.Browser.Proxy.Geo == nil {
			t.Fatal("geo lost in round-trip")
		}
		if *rt.Browser.Proxy.Geo != *fc.Browser.Proxy.Geo {
			t.Errorf("geo did not round-trip: got %+v want %+v", *rt.Browser.Proxy.Geo, *fc.Browser.Proxy.Geo)
		}
	})

	t.Run("proxy emitted when configured", func(t *testing.T) {
		fc := FileConfig{
			Browser: BrowserConfig{
				Provider: BrowserChrome,
				Proxy: BrowserProxyConfig{
					Server:   "http://proxy.example.com:8080",
					Username: "alice",
					Password: "s3cr3t",
				},
			},
		}
		raw, err := json.Marshal(fc)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		// On-disk format intentionally stores raw password; redaction happens at log/HTTP boundaries.
		if !strings.Contains(string(raw), `"password":"s3cr3t"`) {
			t.Errorf("on-disk config should contain raw password, got: %s", string(raw))
		}

		var rt FileConfig
		if err := json.Unmarshal(raw, &rt); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if rt.Browser.Proxy.Server != fc.Browser.Proxy.Server ||
			rt.Browser.Proxy.Username != fc.Browser.Proxy.Username ||
			rt.Browser.Proxy.Password != fc.Browser.Proxy.Password {
			t.Errorf("proxy did not round-trip: got %+v want %+v", rt.Browser.Proxy, fc.Browser.Proxy)
		}
	})
}
