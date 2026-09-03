package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/pinchtab/pinchtab/internal/autosolver"
	"github.com/pinchtab/pinchtab/internal/autosolver/catalog"
	"github.com/pinchtab/pinchtab/internal/browsers"
	_ "github.com/pinchtab/pinchtab/internal/browsers/chrome"
	_ "github.com/pinchtab/pinchtab/internal/browsers/cloak"
	_ "github.com/pinchtab/pinchtab/internal/browsers/ghostchrome"
	"github.com/pinchtab/pinchtab/internal/session"
)

func TestValidateFileConfig_Valid(t *testing.T) {
	maxTabs := 20
	fc := &FileConfig{
		Server: ServerConfig{
			Port: "9867",
			Bind: "127.0.0.1",
		},
		InstanceDefaults: InstanceDefaultsConfig{
			Mode:              "headless",
			MaxTabs:           &maxTabs,
			StealthLevel:      "light",
			TabEvictionPolicy: "reject",
		},
		MultiInstance: MultiInstanceConfig{
			Strategy:         "simple",
			AllocationPolicy: "fcfs",
			Restart: MultiInstanceRestartConfig{
				MaxRestarts:    intPtr(20),
				InitBackoffSec: intPtr(2),
				MaxBackoffSec:  intPtr(60),
				StableAfterSec: intPtr(300),
			},
		},
		Timeouts: TimeoutsConfig{
			ActionSec:   30,
			NavigateSec: 60,
		},
	}

	errs := ValidateFileConfig(fc)
	if len(errs) > 0 {
		t.Errorf("expected no errors for valid config, got: %v", errs)
	}
}

func TestValidateFileConfig_RestartPolicy(t *testing.T) {
	tests := []struct {
		name    string
		restart MultiInstanceRestartConfig
		wantErr bool
	}{
		{
			name: "bounded",
			restart: MultiInstanceRestartConfig{
				MaxRestarts:    intPtr(10),
				InitBackoffSec: intPtr(2),
				MaxBackoffSec:  intPtr(60),
				StableAfterSec: intPtr(300),
			},
			wantErr: false,
		},
		{
			name: "unlimited",
			restart: MultiInstanceRestartConfig{
				MaxRestarts:    intPtr(-1),
				InitBackoffSec: intPtr(2),
				MaxBackoffSec:  intPtr(60),
				StableAfterSec: intPtr(300),
			},
			wantErr: false,
		},
		{
			name: "zero max restarts valid (no restarts)",
			restart: MultiInstanceRestartConfig{
				MaxRestarts: intPtr(0),
			},
			wantErr: false,
		},
		{
			name: "negative max restarts invalid (except -1)",
			restart: MultiInstanceRestartConfig{
				MaxRestarts: intPtr(-2),
			},
			wantErr: true,
		},
		{
			name: "max backoff lower than init invalid",
			restart: MultiInstanceRestartConfig{
				InitBackoffSec: intPtr(10),
				MaxBackoffSec:  intPtr(5),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fc := &FileConfig{
				MultiInstance: MultiInstanceConfig{Restart: tt.restart},
			}
			errs := ValidateFileConfig(fc)
			hasErr := len(errs) > 0
			if hasErr != tt.wantErr {
				t.Fatalf("got error=%v, want %v (errs: %v)", hasErr, tt.wantErr, errs)
			}
		})
	}
}

func intPtr(v int) *int { return &v }

func TestValidateFileConfig_InvalidPort(t *testing.T) {
	tests := []struct {
		port    string
		wantErr bool
	}{
		{"9867", false},
		{"1", false},
		{"65535", false},
		{"0", true},
		{"65536", true},
		{"-1", true},
		{"abc", true},
		{"", false}, // empty is ok (uses default)
	}

	for _, tt := range tests {
		fc := &FileConfig{
			Server: ServerConfig{Port: tt.port},
		}
		errs := ValidateFileConfig(fc)
		hasErr := len(errs) > 0
		if hasErr != tt.wantErr {
			t.Errorf("port=%q: got error=%v, want error=%v (errs: %v)", tt.port, hasErr, tt.wantErr, errs)
		}
	}
}

func TestValidateFileConfig_InvalidNetworkBufferSize(t *testing.T) {
	zero := 0
	negative := -1
	tooLarge := MaxNetworkBufferSize + 1
	valid := MaxNetworkBufferSize

	tests := []struct {
		name    string
		size    *int
		wantErr bool
	}{
		{"nil", nil, false},
		{"valid", &valid, false},
		{"zero", &zero, true},
		{"negative", &negative, true},
		{"too_large", &tooLarge, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fc := &FileConfig{
				Server: ServerConfig{NetworkBufferSize: tt.size},
			}
			errs := ValidateFileConfig(fc)
			hasErr := len(errs) > 0
			if hasErr != tt.wantErr {
				t.Errorf("networkBufferSize=%v: got error=%v, want error=%v (errs: %v)", tt.size, hasErr, tt.wantErr, errs)
			}
		})
	}
}

func TestValidateFileConfig_InvalidStealthLevel(t *testing.T) {
	tests := []struct {
		level   string
		wantErr bool
	}{
		{"light", false},
		{"medium", false},
		{"full", false},
		{"", false}, // empty is ok
		{"none", true},
		{"max", true},
		{"LIGHT", true}, // case sensitive
	}

	for _, tt := range tests {
		fc := &FileConfig{
			InstanceDefaults: InstanceDefaultsConfig{StealthLevel: tt.level},
		}
		errs := ValidateFileConfig(fc)
		hasErr := len(errs) > 0
		if hasErr != tt.wantErr {
			t.Errorf("stealthLevel=%q: got error=%v, want error=%v", tt.level, hasErr, tt.wantErr)
		}
	}
}

func TestValidateFileConfig_InvalidEvictionPolicy(t *testing.T) {
	tests := []struct {
		policy  string
		wantErr bool
	}{
		{"reject", false},
		{"close_oldest", false},
		{"close_lru", false},
		{"", false},
		{"drop", true},
		{"lru", true},
	}

	for _, tt := range tests {
		fc := &FileConfig{
			InstanceDefaults: InstanceDefaultsConfig{TabEvictionPolicy: tt.policy},
		}
		errs := ValidateFileConfig(fc)
		hasErr := len(errs) > 0
		if hasErr != tt.wantErr {
			t.Errorf("tabEvictionPolicy=%q: got error=%v, want error=%v", tt.policy, hasErr, tt.wantErr)
		}
	}
}

func TestValidateFileConfig_InvalidStrategy(t *testing.T) {
	tests := []struct {
		strategy string
		wantErr  bool
	}{
		{"simple", false},
		{"explicit", false},
		{"simple-autorestart", false},
		{"always-on", false},
		{"", false},
		{"auto", true},
		{"default", true},
	}

	for _, tt := range tests {
		fc := &FileConfig{
			MultiInstance: MultiInstanceConfig{Strategy: tt.strategy},
		}
		errs := ValidateFileConfig(fc)
		hasErr := len(errs) > 0
		if hasErr != tt.wantErr {
			t.Errorf("strategy=%q: got error=%v, want error=%v", tt.strategy, hasErr, tt.wantErr)
		}
	}
}

func TestValidateFileConfig_InvalidAllocationPolicy(t *testing.T) {
	tests := []struct {
		policy  string
		wantErr bool
	}{
		{"fcfs", false},
		{"round_robin", false},
		{"random", false},
		{"", false},
		{"fifo", true},
		{"roundrobin", true}, // underscore required
	}

	for _, tt := range tests {
		fc := &FileConfig{
			MultiInstance: MultiInstanceConfig{AllocationPolicy: tt.policy},
		}
		errs := ValidateFileConfig(fc)
		hasErr := len(errs) > 0
		if hasErr != tt.wantErr {
			t.Errorf("allocationPolicy=%q: got error=%v, want error=%v", tt.policy, hasErr, tt.wantErr)
		}
	}
}

func TestValidateFileConfig_InvalidAttachScheme(t *testing.T) {
	tests := []struct {
		schemes []string
		wantErr bool
	}{
		{[]string{"ws"}, false},
		{[]string{"wss"}, false},
		{[]string{"ws", "wss"}, false},
		{[]string{"http"}, false},
		{[]string{"https"}, false},
		{[]string{"ws", "https"}, false},
		{[]string{"ftp"}, true},
		{[]string{"ws", "tcp"}, true},
	}

	for _, tt := range tests {
		fc := &FileConfig{
			Security: SecurityConfig{
				Attach: AttachConfig{AllowSchemes: tt.schemes},
			},
		}
		errs := ValidateFileConfig(fc)
		hasErr := len(errs) > 0
		if hasErr != tt.wantErr {
			t.Errorf("allowSchemes=%v: got error=%v, want error=%v", tt.schemes, hasErr, tt.wantErr)
		}
	}
}

func TestValidateFileConfig_InvalidMaxTabs(t *testing.T) {
	zero := 0
	negative := -1
	positive := 10

	tests := []struct {
		name    string
		maxTabs *int
		wantErr bool
	}{
		{"nil", nil, false},
		{"positive", &positive, false},
		{"zero", &zero, true},
		{"negative", &negative, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fc := &FileConfig{
				InstanceDefaults: InstanceDefaultsConfig{MaxTabs: tt.maxTabs},
			}
			errs := ValidateFileConfig(fc)
			hasErr := len(errs) > 0
			if hasErr != tt.wantErr {
				t.Errorf("maxTabs=%v: got error=%v, want error=%v", tt.maxTabs, hasErr, tt.wantErr)
			}
		})
	}
}

func TestValidateFileConfig_InvalidTimeouts(t *testing.T) {
	fc := &FileConfig{
		Timeouts: TimeoutsConfig{
			ActionSec:   -1,
			NavigateSec: -1,
			ShutdownSec: -1,
			WaitNavMs:   -1,
		},
	}

	errs := ValidateFileConfig(fc)
	if len(errs) != 4 {
		t.Errorf("expected 4 timeout errors, got %d: %v", len(errs), errs)
	}
}

func TestValidateFileConfig_InvalidActivityRetentionDays(t *testing.T) {
	tests := []struct {
		name    string
		value   *int
		wantErr bool
	}{
		{name: "nil", value: nil, wantErr: false},
		{name: "positive", value: intPtr(30), wantErr: false},
		{name: "zero", value: intPtr(0), wantErr: true},
		{name: "negative", value: intPtr(-1), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fc := &FileConfig{
				Observability: ObservabilityFileConfig{
					Activity: ActivityFileConfig{
						RetentionDays: tt.value,
					},
				},
			}
			errs := ValidateFileConfig(fc)
			hasErr := len(errs) > 0
			if hasErr != tt.wantErr {
				t.Fatalf("RetentionDays=%v: got error=%v, want %v (errs: %v)", tt.value, hasErr, tt.wantErr, errs)
			}
		})
	}
}

func TestValidateFileConfig_InstancePortRange(t *testing.T) {
	start := 9900
	end := 9800 // invalid: start > end

	fc := &FileConfig{
		Server: ServerConfig{},
		MultiInstance: MultiInstanceConfig{
			InstancePortStart: &start,
			InstancePortEnd:   &end,
		},
	}

	errs := ValidateFileConfig(fc)
	if len(errs) != 1 {
		t.Errorf("expected 1 error for invalid port range, got %d: %v", len(errs), errs)
	}
	if len(errs) > 0 && !strings.Contains(errs[0].Error(), "start port") {
		t.Errorf("expected port range error, got: %v", errs[0])
	}
}

func TestValidateFileConfig_MultipleErrors(t *testing.T) {
	zero := 0
	fc := &FileConfig{
		Server: ServerConfig{
			Port: "99999", // invalid
		},
		InstanceDefaults: InstanceDefaultsConfig{
			MaxTabs:           &zero,           // invalid
			StealthLevel:      "superstealth",  // invalid
			TabEvictionPolicy: "delete_oldest", // invalid
		},
		MultiInstance: MultiInstanceConfig{
			Strategy:         "magical",  // invalid
			AllocationPolicy: "balanced", // invalid
		},
	}

	errs := ValidateFileConfig(fc)
	if len(errs) < 5 {
		t.Errorf("expected at least 5 errors, got %d: %v", len(errs), errs)
	}
}

func TestValidateFileConfig_BrowserExtraFlags(t *testing.T) {
	tests := []struct {
		name    string
		flags   string
		wantErr bool
		wantMsg string
	}{
		{
			name:  "safe flags",
			flags: "--disable-gpu --ash-no-nudges --disable-focus-on-load",
		},
		{
			name:    "unsafe no sandbox",
			flags:   "--no-sandbox",
			wantErr: true,
			wantMsg: "runtime compatibility",
		},
		{
			name:    "reserved user agent",
			flags:   "--user-agent=PinchTab-Test/1.0",
			wantErr: true,
			wantMsg: "instanceDefaults.userAgent",
		},
		{
			name:    "site isolation disabled",
			flags:   "--disable-features=Translate,SitePerProcess",
			wantErr: true,
			wantMsg: "site isolation",
		},
		{
			name:    "malformed token",
			flags:   "disable-gpu",
			wantErr: true,
			wantMsg: "--name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fc := &FileConfig{
				Browser: BrowserConfig{BrowserExtraFlags: tt.flags},
			}
			errs := ValidateFileConfig(fc)
			hasErr := len(errs) > 0
			if hasErr != tt.wantErr {
				t.Fatalf("BrowserExtraFlags=%q: got error=%v, want %v (errs: %v)", tt.flags, hasErr, tt.wantErr, errs)
			}
			if tt.wantErr && !strings.Contains(errs[0].Error(), tt.wantMsg) {
				t.Fatalf("expected error containing %q, got %v", tt.wantMsg, errs[0])
			}
		})
	}
}

func TestValidateFileConfig_CloakBrowser(t *testing.T) {
	quota := 2048
	fontsDir := t.TempDir()
	// Valid cloak config with browsers.default instead of browser.provider.
	valid := &FileConfig{
		Browsers: BrowsersConfig{Default: BrowserCloak},
		Browser: BrowserConfig{
			BrowserBinary: "/opt/cloakbrowser/chrome",
			Cloak: CloakBrowserConfig{
				FingerprintSeed: "42069",
				Platform:        "windows",
				Locale:          "en-GB",
				Timezone:        "Europe/London",
				WebRTCIP:        "auto",
				FontsDir:        fontsDir,
				StorageQuotaMB:  &quota,
			},
		},
	}
	for _, e := range ValidateFileConfig(valid) {
		t.Fatalf("ValidateFileConfig(valid cloak config) unexpected error = %v", e)
	}

	// browser.provider presence triggers a hard error regardless of value.
	providerFC := &FileConfig{Browser: BrowserConfig{Provider: "firefox"}}
	providerErrs := ValidateFileConfig(providerFC)
	found := false
	for _, err := range providerErrs {
		if strings.Contains(err.Error(), "browser.provider is no longer supported") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected browser.provider removal error, got: %v", providerErrs)
	}

	negativeQuota := -1
	tests := []struct {
		name string
		fc   *FileConfig
		want string
	}{
		{
			name: "invalid platform",
			fc: &FileConfig{Browser: BrowserConfig{
				BrowserBinary: "/opt/cloakbrowser/chrome",
				Cloak:         CloakBrowserConfig{Platform: "ios"},
			}},
			want: "browser.cloak.platform",
		},
		{
			name: "negative storage quota",
			fc: &FileConfig{Browser: BrowserConfig{
				BrowserBinary: "/opt/cloakbrowser/chrome",
				Cloak:         CloakBrowserConfig{StorageQuotaMB: &negativeQuota},
			}},
			want: "browser.cloak.storageQuotaMB",
		},
		{
			name: "fingerprint seed whitespace",
			fc: &FileConfig{Browser: BrowserConfig{
				BrowserBinary: "/opt/cloakbrowser/chrome",
				Cloak:         CloakBrowserConfig{FingerprintSeed: "42 069"},
			}},
			want: "browser.cloak.fingerprintSeed",
		},
		{
			name: "invalid timezone",
			fc: &FileConfig{Browser: BrowserConfig{
				BrowserBinary: "/opt/cloakbrowser/chrome",
				Cloak:         CloakBrowserConfig{Timezone: "Not/AZone"},
			}},
			want: "browser.cloak",
		},
		{
			name: "invalid locale",
			fc: &FileConfig{Browser: BrowserConfig{
				BrowserBinary: "/opt/cloakbrowser/chrome",
				Cloak:         CloakBrowserConfig{Locale: "en-gb"},
			}},
			want: "browser.cloak",
		},
		{
			name: "invalid webrtc ip",
			fc: &FileConfig{Browser: BrowserConfig{
				BrowserBinary: "/opt/cloakbrowser/chrome",
				Cloak:         CloakBrowserConfig{WebRTCIP: "not-an-ip"},
			}},
			want: "browser.cloak.webrtcIP",
		},
		{
			name: "fonts dir must exist",
			fc: &FileConfig{Browser: BrowserConfig{
				BrowserBinary: "/opt/cloakbrowser/chrome",
				Cloak:         CloakBrowserConfig{FontsDir: "/definitely/not/a/real/fonts/dir"},
			}},
			want: "browser.cloak.fontsDir",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateFileConfig(tt.fc)
			if len(errs) == 0 {
				t.Fatalf("ValidateFileConfig() returned no errors, want %s", tt.want)
			}
			found := false
			for _, err := range errs {
				if strings.Contains(err.Error(), tt.want) {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("ValidateFileConfig() errors = %v, want field %s", errs, tt.want)
			}
		})
	}
}

func TestValidationError_Error(t *testing.T) {
	err := ValidationError{
		Field:   "server.port",
		Message: "port out of range",
	}
	expected := "server.port: port out of range"
	if err.Error() != expected {
		t.Errorf("got %q, want %q", err.Error(), expected)
	}
}

func TestValidEnumValues(t *testing.T) {
	for _, level := range ValidStealthLevels() {
		if !isValidStealthLevel(level) {
			t.Errorf("ValidStealthLevels contains %q but isValidStealthLevel returns false", level)
		}
	}

	for _, policy := range ValidEvictionPolicies() {
		if !isValidEvictionPolicy(policy) {
			t.Errorf("ValidEvictionPolicies contains %q but isValidEvictionPolicy returns false", policy)
		}
	}

	for _, strategy := range ValidStrategies() {
		if !isValidStrategy(strategy) {
			t.Errorf("ValidStrategies contains %q but isValidStrategy returns false", strategy)
		}
	}

	for _, policy := range ValidAllocationPolicies() {
		if !isValidAllocationPolicy(policy) {
			t.Errorf("ValidAllocationPolicies contains %q but isValidAllocationPolicy returns false", policy)
		}
	}

	for _, scheme := range ValidAttachSchemes() {
		if !isValidAttachScheme(scheme) {
			t.Errorf("ValidAttachSchemes contains %q but isValidAttachScheme returns false", scheme)
		}
	}

	for _, policy := range ValidLifecyclePolicies() {
		if !isValidLifecyclePolicy(policy) {
			t.Errorf("ValidLifecyclePolicies contains %q but isValidLifecyclePolicy returns false", policy)
		}
	}

	// Known-bad values must be rejected by every membership check.
	if isValidStealthLevel("bogus") || isValidEvictionPolicy("bogus") ||
		isValidStrategy("bogus") || isValidAllocationPolicy("bogus") ||
		isValidAttachScheme("bogus") || isValidLifecyclePolicy("bogus") {
		t.Error("a bogus value was accepted by a membership check")
	}
}

// TestValidateIDPIConfig_Disabled verifies that a disabled IDPI config produces
// no errors regardless of what fields are set.
func TestValidateIDPIConfig_Disabled(t *testing.T) {
	errs := validateIDPIConfig(IDPIConfig{
		Enabled:        false,
		CustomPatterns: []string{"", "  "},
	}, []string{"", "  ", "file:///etc/passwd"})
	if len(errs) != 0 {
		t.Errorf("expected no errors when IDPI disabled, got: %v", errs)
	}
}

func TestValidateIDPIConfig_ValidConfig(t *testing.T) {
	errs := validateIDPIConfig(IDPIConfig{
		Enabled:        true,
		CustomPatterns: []string{"exfiltrate this", "data leak"},
	}, []string{"example.com", "*.example.com", "*"})
	if len(errs) != 0 {
		t.Errorf("expected no errors for valid IDPI config, got: %v", errs)
	}
}

func TestValidateIDPIConfig_EmptyDomain(t *testing.T) {
	cases := []struct {
		name   string
		domain string
	}{
		{"empty string", ""},
		{"spaces only", "   "},
		{"tab only", "\t"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			errs := validateIDPIConfig(IDPIConfig{
				Enabled: true,
			}, []string{tc.domain})
			if len(errs) == 0 {
				t.Errorf("expected error for empty domain %q, got none", tc.domain)
			}
			if len(errs) > 0 && !strings.Contains(errs[0].Error(), "security.allowedDomains") {
				t.Errorf("expected field name in error, got: %v", errs[0])
			}
		})
	}
}

func TestValidateIDPIConfig_DomainWithInternalWhitespace(t *testing.T) {
	errs := validateIDPIConfig(IDPIConfig{
		Enabled: true,
	}, []string{"example .com"})
	if len(errs) == 0 {
		t.Error("expected error for domain with internal whitespace, got none")
	}
}

// TestValidateIDPIConfig_FileSchemeBlocked verifies that file:// domain patterns
// are rejected because they cannot represent a valid network host.
func TestValidateIDPIConfig_FileSchemeBlocked(t *testing.T) {
	for _, pattern := range []string{
		"file:///etc/passwd",
		"file://localhost/etc/passwd",
	} {
		errs := validateIDPIConfig(IDPIConfig{
			Enabled: true,
		}, []string{pattern})
		if len(errs) == 0 {
			t.Errorf("expected error for file:// pattern %q, got none", pattern)
		}
		if len(errs) > 0 && !strings.Contains(errs[0].Error(), "file://") {
			t.Errorf("expected file:// mention in error, got: %v", errs[0])
		}
	}
}

func TestValidateFileConfig_DownloadAllowedDomains(t *testing.T) {
	tests := []struct {
		name    string
		domains []string
		wantErr bool
	}{
		{"empty list", nil, false},
		{"valid entries", []string{"pinchtab.com", "*.pinchtab.com"}, false},
		{"whitespace only", []string{"   "}, true},
		{"internal whitespace", []string{"pinchtab .com"}, true},
		{"file scheme", []string{"file:///tmp/file.txt"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fc := &FileConfig{
				Security: SecurityConfig{
					DownloadAllowedDomains: tt.domains,
				},
			}
			errs := ValidateFileConfig(fc)
			hasErr := len(errs) > 0
			if hasErr != tt.wantErr {
				t.Fatalf("ValidateFileConfig(downloadAllowedDomains=%v) error=%v, want %v (errs: %v)", tt.domains, hasErr, tt.wantErr, errs)
			}
			if tt.wantErr && !strings.Contains(errs[0].Error(), "security.downloadAllowedDomains") {
				t.Fatalf("expected security.downloadAllowedDomains error, got %v", errs[0])
			}
		})
	}
}

func TestValidateFileConfig_TrustedCIDRs(t *testing.T) {
	tests := []struct {
		name    string
		proxy   []string
		resolve []string
		wantErr bool
		field   string
	}{
		{
			name:    "valid IPv4 CIDR and IP",
			proxy:   []string{"10.0.0.0/8", "10.1.2.3"},
			resolve: []string{"198.18.0.0/15"},
			wantErr: false,
		},
		{
			name:    "valid IPv6 host and prefix",
			proxy:   []string{"fd00::1234", "fd00::/64"},
			resolve: []string{"2001:db8::1/128"},
			wantErr: false,
		},
		{
			name:    "invalid trusted proxy entry",
			proxy:   []string{"not-a-cidr"},
			wantErr: true,
			field:   "security.trustedProxyCIDRs",
		},
		{
			name:    "invalid trusted resolve entry",
			resolve: []string{"10.0.0.0/99"},
			wantErr: true,
			field:   "security.trustedResolveCIDRs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fc := &FileConfig{
				Security: SecurityConfig{
					TrustedProxyCIDRs:   tt.proxy,
					TrustedResolveCIDRs: tt.resolve,
				},
			}
			errs := ValidateFileConfig(fc)
			hasErr := len(errs) > 0
			if hasErr != tt.wantErr {
				t.Fatalf("ValidateFileConfig(proxy=%v, resolve=%v) error=%v, want %v (errs: %v)", tt.proxy, tt.resolve, hasErr, tt.wantErr, errs)
			}
			if tt.field != "" && len(errs) > 0 && !strings.Contains(errs[0].Error(), tt.field) {
				t.Fatalf("expected error to mention %s, got %v", tt.field, errs[0])
			}
		})
	}
}

func TestValidateFileConfig_TransferLimits(t *testing.T) {
	tests := []struct {
		name    string
		apply   func(*SecurityConfig)
		wantErr string
	}{
		{
			name: "valid limits",
			apply: func(s *SecurityConfig) {
				s.DownloadMaxBytes = intPtr(DefaultDownloadMaxBytes)
				s.UploadMaxRequestBytes = intPtr(DefaultUploadMaxRequestBytes)
				s.UploadMaxFiles = intPtr(DefaultUploadMaxFiles)
				s.UploadMaxFileBytes = intPtr(DefaultUploadMaxFileBytes)
				s.UploadMaxTotalBytes = intPtr(DefaultUploadMaxTotalBytes)
			},
		},
		{
			name: "download max bytes too large",
			apply: func(s *SecurityConfig) {
				s.DownloadMaxBytes = intPtr(MaxDownloadMaxBytes + 1)
			},
			wantErr: "security.downloadMaxBytes",
		},
		{
			name: "upload max files invalid",
			apply: func(s *SecurityConfig) {
				s.UploadMaxFiles = intPtr(0)
			},
			wantErr: "security.uploadMaxFiles",
		},
		{
			name: "upload max file bytes too large",
			apply: func(s *SecurityConfig) {
				s.UploadMaxFileBytes = intPtr(MaxUploadMaxFileBytes + 1)
			},
			wantErr: "security.uploadMaxFileBytes",
		},
		{
			name: "upload max file bytes exceeds total",
			apply: func(s *SecurityConfig) {
				s.UploadMaxFileBytes = intPtr(8 << 20)
				s.UploadMaxTotalBytes = intPtr(4 << 20)
			},
			wantErr: "security.uploadMaxFileBytes/uploadMaxTotalBytes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fc := &FileConfig{}
			tt.apply(&fc.Security)
			errs := ValidateFileConfig(fc)
			if tt.wantErr == "" {
				if len(errs) > 0 {
					t.Fatalf("expected no errors, got %v", errs)
				}
				return
			}
			if len(errs) == 0 {
				t.Fatalf("expected validation error containing %q, got none", tt.wantErr)
			}
			if !strings.Contains(errs[0].Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, errs[0])
			}
		})
	}
}

func TestValidateFileConfig_AutoSolverValidation(t *testing.T) {
	maxAttempts := 0
	solverTimeout := 0
	retryBase := 2000
	retryMax := 1000

	fc := &FileConfig{
		AutoSolver: AutoSolverFileConfig{
			MaxAttempts:      &maxAttempts,
			SolverTimeoutSec: &solverTimeout,
			RetryBaseDelayMs: &retryBase,
			RetryMaxDelayMs:  &retryMax,
			Solvers:          []string{"cloudflare", ""},
		},
	}

	errs := ValidateFileConfig(fc)
	if len(errs) == 0 {
		t.Fatal("expected autosolver validation errors, got none")
	}

	joined := ""
	for _, err := range errs {
		joined += err.Error() + "\n"
	}

	for _, want := range []string{
		"autoSolver.maxAttempts",
		"autoSolver.solverTimeoutSec",
		"autoSolver.retryBaseDelayMs/retryMaxDelayMs",
		"autoSolver.solvers",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected error containing %q, got:\n%s", want, joined)
		}
	}
}

func TestValidateIDPIConfig_EmptyCustomPattern(t *testing.T) {
	cases := []string{"", "  ", "\t"}
	for _, p := range cases {
		errs := validateIDPIConfig(IDPIConfig{
			Enabled:        true,
			CustomPatterns: []string{p},
		}, nil)
		if len(errs) == 0 {
			t.Errorf("expected error for empty custom pattern %q, got none", p)
		}
		if len(errs) > 0 && !strings.Contains(errs[0].Error(), "customPatterns") {
			t.Errorf("expected customPatterns field in error, got: %v", errs[0])
		}
	}
}

// TestValidateIDPIConfig_MultipleErrors ensures all IDPI violations are
// accumulated rather than short-circuited.
func TestValidateIDPIConfig_MultipleErrors(t *testing.T) {
	errs := validateIDPIConfig(IDPIConfig{
		Enabled:        true,
		CustomPatterns: []string{"", "   "},
	}, []string{"", "file:///bad"})
	if len(errs) < 4 {
		t.Errorf("expected at least 4 IDPI errors, got %d: %v", len(errs), errs)
	}
}

// TestValidateFileConfig_IDPIPassthrough verifies that ValidateFileConfig
// surfaces IDPI errors alongside other config errors.
func TestValidateFileConfig_IDPIPassthrough(t *testing.T) {
	fc := &FileConfig{
		Security: SecurityConfig{
			AllowedDomains: []string{""}, // invalid
			IDPI: IDPIConfig{
				Enabled:        true,
				CustomPatterns: []string{"  "}, // invalid
			},
		},
	}
	errs := ValidateFileConfig(fc)
	if len(errs) < 2 {
		t.Errorf("expected at least 2 IDPI errors via ValidateFileConfig, got %d: %v", len(errs), errs)
	}
}

// TestValidateIDPIConfig_ScanTimeoutSec verifies that negative values are rejected
// and zero/positive values are accepted (zero means use the default).
func TestValidateIDPIConfig_ScanTimeoutSec(t *testing.T) {
	t.Run("NegativeIsInvalid", func(t *testing.T) {
		errs := validateIDPIConfig(IDPIConfig{
			Enabled:        true,
			ScanTimeoutSec: -1,
		}, nil)
		if len(errs) == 0 {
			t.Error("expected error for negative scanTimeoutSec, got none")
		}
		if len(errs) > 0 && !strings.Contains(errs[0].Error(), "scanTimeoutSec") {
			t.Errorf("expected scanTimeoutSec field in error, got: %v", errs[0])
		}
	})

	t.Run("ZeroIsValid", func(t *testing.T) {
		errs := validateIDPIConfig(IDPIConfig{
			Enabled:        true,
			ScanTimeoutSec: 0, // zero → use built-in default of 5s
		}, nil)
		if len(errs) != 0 {
			t.Errorf("expected no error for scanTimeoutSec=0, got: %v", errs)
		}
	})

	t.Run("PositiveIsValid", func(t *testing.T) {
		errs := validateIDPIConfig(IDPIConfig{
			Enabled:        true,
			ScanTimeoutSec: 10,
		}, nil)
		if len(errs) != 0 {
			t.Errorf("expected no error for scanTimeoutSec=10, got: %v", errs)
		}
	})
}

func TestValidateFileConfig_TabPolicyBlock(t *testing.T) {
	negative := -1
	zero := 0
	positive := 100
	tests := []struct {
		name    string
		tp      TabPolicyDefaults
		wantErr bool
	}{
		{"all empty", TabPolicyDefaults{}, false},
		{"valid lifecycle", TabPolicyDefaults{Lifecycle: "close_idle"}, false},
		{"keep lifecycle", TabPolicyDefaults{Lifecycle: "keep"}, false},
		{"valid eviction", TabPolicyDefaults{Eviction: "reject"}, false},
		{"bad lifecycle", TabPolicyDefaults{Lifecycle: "burn"}, true},
		{"bad eviction", TabPolicyDefaults{Eviction: "drop"}, true},
		{"negative delay", TabPolicyDefaults{Lifecycle: "close_idle", CloseDelaySec: &negative}, true},
		{"zero delay ok", TabPolicyDefaults{Lifecycle: "close_idle", CloseDelaySec: &zero}, false},
		{"positive delay ok", TabPolicyDefaults{Lifecycle: "close_idle", CloseDelaySec: &positive}, false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			fc := &FileConfig{
				InstanceDefaults: InstanceDefaultsConfig{TabPolicy: &tt.tp},
			}
			errs := ValidateFileConfig(fc)
			hasErr := len(errs) > 0
			if hasErr != tt.wantErr {
				t.Errorf("got errs=%v, want error=%v", errs, tt.wantErr)
			}
		})
	}
}

// testStubBrowser is a minimal Browser implementation for testing that
// config validation accepts any registered provider without config code edits.
type testStubBrowser struct{}

func (testStubBrowser) ID() string                                                  { return "validationstub" }
func (testStubBrowser) DisplayName() string                                         { return "ValidationStub" }
func (testStubBrowser) Capabilities() browsers.CapabilitySet                        { return browsers.CapabilitySet{} }
func (testStubBrowser) DiscoverBinary() browsers.BinaryDiscovery                    { return browsers.BinaryDiscovery{} }
func (testStubBrowser) DoctorChecks(_ browsers.TargetConfig) []browsers.DoctorCheck { return nil }
func (testStubBrowser) BuildLaunchArgs(_ browsers.LaunchConfig) ([]string, []string, error) {
	return nil, nil, nil
}
func (testStubBrowser) SupportsRemoteCDP() bool { return false }
func (testStubBrowser) GeoAlignment(_ browsers.GeoConfig) browsers.GeoStrategy {
	return browsers.GeoStrategy{}
}
func (testStubBrowser) ValidateTarget(_ browsers.TargetConfig) error { return nil }
func (testStubBrowser) ClassifyLaunchError(_ browsers.LaunchFailure) browsers.LaunchErrorKind {
	return browsers.LaunchErrorUnknown
}
func (testStubBrowser) CanHandle(_ browsers.RequestIntent) browsers.HandleDecision {
	return browsers.HandleDecision{Decision: browsers.DecisionHandle}
}

var registerStubOnce sync.Once

func TestValidationAcceptsRegisteredStubProvider(t *testing.T) {
	// Register a stub provider that config code has never heard of.
	registerStubOnce.Do(func() {
		browsers.Register(&testStubBrowser{})
	})

	// As browsers.default: should not produce an "unknown browser" error.
	fc := &FileConfig{Browsers: BrowsersConfig{Default: "validationstub"}}
	errs := ValidateFileConfig(fc)
	for _, e := range errs {
		if strings.Contains(e.Error(), "validationstub") && strings.Contains(e.Error(), "unknown") {
			t.Fatalf("stub provider should be accepted as browsers.default; got error: %s", e.Error())
		}
	}

	// browser.provider is rejected regardless of value.
	fcLegacy := &FileConfig{Browser: BrowserConfig{Provider: "validationstub"}}
	legacyErrs := ValidateFileConfig(fcLegacy)
	found := false
	for _, e := range legacyErrs {
		if strings.Contains(e.Error(), "browser.provider is no longer supported") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected browser.provider removal error for stub provider, got: %v", legacyErrs)
	}

	// As a target provider: should not produce an "unknown provider" error.
	fc2 := &FileConfig{
		Browser: BrowserConfig{
			Targets: BrowserTargetsConfig{
				"stub-target": {Provider: "validationstub"},
			},
		},
	}
	errs2 := ValidateFileConfig(fc2)
	for _, e := range errs2 {
		if strings.Contains(e.Error(), "stub-target") && strings.Contains(e.Error(), "unknown") {
			t.Fatalf("stub target provider should be accepted; got error: %s", e.Error())
		}
	}
}

// A misspelled solver name never failed at run time; it changed which solvers
// ran, and in opposite directions depending on how many other names matched.
func TestValidateFileConfig_UnknownSolverName(t *testing.T) {
	for _, tc := range []struct {
		name    string
		solvers []string
		bad     string
	}{
		{
			name:    "one typo beside a good name silently dropped that solver",
			solvers: []string{"cloudlfare", "semantic"},
			bad:     "cloudlfare",
		},
		{
			name:    "every name a typo silently ran the whole registry",
			solvers: []string{"cloudlfare"},
			bad:     "cloudlfare",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			errs := ValidateFileConfig(&FileConfig{
				AutoSolver: AutoSolverFileConfig{Solvers: tc.solvers},
			})

			var msg string
			for _, err := range errs {
				if strings.Contains(err.Error(), tc.bad) {
					msg = err.Error()
				}
			}
			if msg == "" {
				t.Fatalf("solvers %v produced no error naming %q: %v", tc.solvers, tc.bad, errs)
			}
			if !strings.Contains(msg, "autoSolver.solvers") {
				t.Errorf("error does not name the field: %s", msg)
			}
			for _, known := range catalog.Names() {
				if !strings.Contains(msg, known) {
					t.Errorf("error does not list the valid name %q: %s", known, msg)
				}
			}
		})
	}
}

// Key-gated solvers are valid names whether or not their key is set — naming
// one without its key is a different mistake, not an unknown solver.
func TestValidateFileConfig_KnownSolverNamesAccepted(t *testing.T) {
	for _, tc := range []struct {
		name    string
		solvers []string
	}{
		{"full set", catalog.Names()},
		{"subset", []string{"cloudflare", "semantic"}},
		{"reordering", []string{"semantic", "jschallenge", "cloudflare"}},
		{"key-gated without any key configured", catalog.KeyGated()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			errs := ValidateFileConfig(&FileConfig{
				AutoSolver: AutoSolverFileConfig{Solvers: tc.solvers},
			})
			for _, err := range errs {
				if strings.Contains(err.Error(), "autoSolver.solvers") {
					t.Errorf("solvers %v rejected: %s", tc.solvers, err)
				}
			}
		})
	}
}

// The shipped default must satisfy the validator it now passes through.
func TestValidateFileConfig_DefaultSolversAreKnown(t *testing.T) {
	for _, name := range autosolver.DefaultConfig().Solvers {
		if !catalog.IsKnown(name) {
			t.Errorf("default config names solver %q, which validation rejects (known: %v)", name, catalog.Names())
		}
	}
}

// A file that already carries the key keeps loading — validation errors are non-fatal
// diagnostics here — but it no longer does so in silence: the same reason the setter gives
// is reported, so the operator learns the value is dead rather than discovering it never
// took effect.
// A file that already carries the key keeps loading and still says so — but from the
// non-gating list. It rode the gating list once, and because every caller that decides
// whether a write may proceed gates on that list, one inert key aborted every later config
// write. Which list the diagnostic lives in IS the behaviour here.
func TestActivityStateDirIsAdvisoryRatherThanGating(t *testing.T) {
	root := t.TempDir()
	body := `{"server":{"stateDir":` + quoteJSON(root) + `},
		"observability":{"activity":{"stateDir":"/tmp/elsewhere"}}}`
	writeConfigForGet(t, body)

	fc, _, err := LoadFileConfig()
	if err != nil {
		t.Fatalf("LoadFileConfig() error = %v; a file carrying the key must still load", err)
	}

	for _, e := range ValidateFileConfig(fc) {
		if strings.Contains(e.Error(), "observability.activity.stateDir") {
			t.Errorf("gating validation reported %q; an inert key must not gate a save", e)
		}
	}

	advisories := FileConfigAdvisories(fc)
	found := ""
	for _, advisory := range advisories {
		if strings.Contains(advisory, "observability.activity.stateDir") {
			found = advisory
		}
	}
	if found == "" {
		t.Fatalf("advisories = %v, want one naming the ignored key: silently swallowing it is not the fix", advisories)
	}
	if found != ActivityStateDirAdvisory {
		t.Errorf("advisory = %q, want %q", found, ActivityStateDirAdvisory)
	}

	cfg, diags, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v; the advisory must not make an existing config unloadable", err)
	}
	if !hasDiagnostic(diags, slog.LevelWarn, "config setting has no effect") {
		t.Errorf("load diagnostics = %v, want the advisory reported at load", diags)
	}
	if want := filepath.Join(root, "activity"); cfg.ActivityLogDir() != want {
		t.Errorf("ActivityLogDir() = %q, want the derived %q", cfg.ActivityLogDir(), want)
	}
}

// The severity split has to hold in both directions: carrying the key in a file is an
// advisory, and SETTING it is still an error on that write.
func TestSettingTheIgnoredKeyIsStillRefusedWhileCarryingItIsNot(t *testing.T) {
	fc := DefaultFileConfig()
	if err := SetConfigValue(&fc, "observability.activity.stateDir", "/tmp/elsewhere"); err == nil {
		t.Fatal("config set observability.activity.stateDir was accepted")
	}

	carrying := DefaultFileConfig()
	carrying.Observability.Activity.StateDir = "/tmp/elsewhere"
	if errs := ValidateFileConfig(&carrying); len(errs) > 0 {
		t.Errorf("carrying the key produced gating errors %v, want none", errs)
	}
	if len(FileConfigAdvisories(&carrying)) != 1 {
		t.Errorf("advisories = %v, want exactly the ignored-key notice", FileConfigAdvisories(&carrying))
	}
}

// The guardrail on the tier: a genuinely invalid value must keep gating, or this fix trades
// a blocked agent for a config that saves nonsense.
func TestARealValidationErrorStillGates(t *testing.T) {
	fc := DefaultFileConfig()
	fc.Server.Port = "99999"
	fc.Observability.Activity.StateDir = "/tmp/elsewhere"

	errs := ValidateFileConfig(&fc)
	if len(errs) == 0 {
		t.Fatal("an out-of-range port produced no gating error")
	}
	for _, e := range errs {
		if strings.Contains(e.Error(), "observability.activity.stateDir") {
			t.Errorf("gating errors include the advisory %q", e)
		}
	}
}

// The advisory instructs nothing, which is the point of the rewording: the key is inert, so
// there is nothing to do, and the text it replaced named an action no command performs. The
// refusal DOES instruct, and what it names has to be a real, settable key.
func TestTheIgnoredKeyTextsInstructOnlyWhatTheProductCanDo(t *testing.T) {
	for _, banned := range []string{"remove", "delete", "unset"} {
		if strings.Contains(strings.ToLower(ActivityStateDirAdvisory), banned) {
			t.Errorf("advisory = %q, want no %q instruction: no shipped command does that to a key", ActivityStateDirAdvisory, banned)
		}
	}
	for _, want := range []string{"observability.activity.stateDir", "ignored", "<server.stateDir>/activity"} {
		if !strings.Contains(ActivityStateDirAdvisory, want) {
			t.Errorf("advisory = %q, want it to carry %q", ActivityStateDirAdvisory, want)
		}
	}

	if !strings.Contains(ActivityStateDirRefusal, "set server.stateDir") {
		t.Fatalf("refusal = %q, want it to name the setting that moves the logs", ActivityStateDirRefusal)
	}
	// The remedy the refusal names must be a key `config set` accepts, or the text
	// dead-ends the same way "remove the key" did.
	fc := DefaultFileConfig()
	if err := SetConfigValue(&fc, "server.stateDir", t.TempDir()); err != nil {
		t.Errorf("the refusal names server.stateDir but config set rejects it: %v", err)
	}
}

// mode used to be accepted whatever it said and read by nothing, so
// `config set sessions.agent.mode nonsense` succeeded and changed nothing.
// "required" is refused separately from a typo because it is not a typo: it names
// a posture this server does not enforce, and accepting it leaves an operator
// believing they have session-only auth.
func TestValidateFileConfig_AgentSessionMode(t *testing.T) {
	for _, tt := range []struct {
		mode     string
		wantErr  bool
		wantSays string
	}{
		{"", false, ""},
		{"off", false, ""},
		{"preferred", false, ""},
		{"required", true, "not implemented"},
		{"nonsense", true, "invalid value"},
		{"prefered", true, "invalid value"},
		// Case and surrounding space are read the way every other enum in this
		// file is read, so a capitalisation slip in an auth switch means what the
		// operator wrote instead of being refused as a typo.
		{"Off", false, ""},
		{"OFF", false, ""},
		{" off ", false, ""},
		{" Preferred ", false, ""},
	} {
		fc := &FileConfig{Sessions: SessionsFileConfig{Agent: AgentSessionFileConfig{Mode: tt.mode}}}
		errs := ValidateFileConfig(fc)

		if hasErr := len(errs) > 0; hasErr != tt.wantErr {
			t.Errorf("mode=%q: got error=%v (%v), want error=%v", tt.mode, hasErr, errs, tt.wantErr)
			continue
		}
		if !tt.wantErr {
			continue
		}
		message := errs[0].Error()
		if !strings.Contains(message, "sessions.agent.mode") {
			t.Errorf("mode=%q: refusal %q does not name the field", tt.mode, message)
		}
		if !strings.Contains(message, tt.wantSays) {
			t.Errorf("mode=%q: refusal %q does not say %q", tt.mode, message, tt.wantSays)
		}
		var ve ValidationError
		if !errors.As(errs[0], &ve) || !ve.FatalAtLoad {
			t.Errorf("mode=%q: the refusal does not declare itself fatal at load, so a server would warn and then pick a posture on the operator's behalf", tt.mode)
		}
	}
}

// The two readers of ModeServes answer at different points — the live store and the
// raw file value the restart-reason comparison comes from — so they are asserted
// together. A fold applied at only one of them would disagree for "Off", and the
// disagreement is invisible from either side alone.
func TestBothModeConsumersAgreeOnEveryValue(t *testing.T) {
	enabled := true
	for _, tt := range []struct {
		mode string
		want bool
	}{
		{"", true},
		{"preferred", true},
		{" Preferred ", true},
		{"off", false},
		{"Off", false},
		{"OFF", false},
		{" off ", false},
		{"required", false},
		{"prefered", false},
	} {
		fromFile := SessionsFileConfig{Agent: AgentSessionFileConfig{Enabled: &enabled, Mode: tt.mode}}.AgentEnabled()
		fromStore := session.NewStore(session.Config{Enabled: true, Mode: tt.mode}).Enabled()
		if fromFile != tt.want || fromStore != tt.want {
			t.Errorf("mode=%q: file config says %v, the live store says %v, want %v — one predicate, two readers, and a value the validator refuses must never serve",
				tt.mode, fromFile, fromStore, tt.want)
		}
	}
}

// The claim the previous card published to operators and the load path did not keep:
// a config the server cannot interpret stops it, rather than warning and then reading
// the value as the default. Driven through a real file and a real load, because the
// validator's return value was already correct while the outcome was not.
func TestAModeTheServerCannotInterpretStopsTheLoad(t *testing.T) {
	for _, tt := range []struct {
		mode      string
		wantFatal bool
	}{
		{"required", true},
		{"nonsense", true},
		{"Off", false},
		{"preferred", false},
	} {
		path := filepath.Join(t.TempDir(), "config.json")
		body := fmt.Sprintf(`{"server":{"port":"9867"},"sessions":{"agent":{"mode":%q}}}`, tt.mode)
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PINCHTAB_CONFIG", path)

		cfg, _, err := LoadConfig()
		if !tt.wantFatal {
			if err != nil {
				t.Errorf("mode=%q refused the load: %v", tt.mode, err)
			}
			continue
		}
		if err == nil {
			t.Errorf("mode=%q loaded and the server would run with agent sessions %v; a posture it cannot interpret must not be chosen on the operator's behalf",
				tt.mode, cfg.Sessions.Agent.Enabled)
			continue
		}
		if !strings.Contains(err.Error(), "sessions.agent.mode") {
			t.Errorf("mode=%q: the fatal error %q does not name the field to fix", tt.mode, err)
		}
	}
}

// A fatal load must not lock the operator out of the command that repairs it. The
// editor reads and writes the file directly and never builds a RuntimeConfig, so
// `config set sessions.agent.mode off` works on a config the server refuses to
// start on — pinned here because routing the editor through the runtime load would
// be an invisible change that leaves a hand edit as the only way out.
func TestTheEditorStillRepairsAConfigTheLoadRefuses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"server":{"port":"9867"},"sessions":{"agent":{"mode":"required"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PINCHTAB_CONFIG", path)

	if _, _, err := LoadConfig(); err == nil {
		t.Fatal("the load accepted the unusable mode, so this test is not standing on the state it is about")
	}

	fc, loadedPath, err := LoadFileConfig()
	if err != nil {
		t.Fatalf("the editor cannot even read the file it must repair: %v", err)
	}
	if err := SetConfigValue(fc, "sessions.agent.mode", session.ModeOff); err != nil {
		t.Fatalf("the editor refused the repair: %v", err)
	}
	if err := SaveFileConfig(fc, loadedPath); err != nil {
		t.Fatalf("the editor could not save the repair: %v", err)
	}
	if _, _, err := LoadConfig(); err != nil {
		t.Fatalf("the repaired config still refuses to load: %v", err)
	}
}

// The predicate the whole card turns on, at the file-config layer: mode off and
// enabled false must answer the same, or the restart-reason comparison built on
// this misses the transition through the other field.
func TestAgentEnabledFoldsBothFields(t *testing.T) {
	enabled, disabled := true, false
	for _, tt := range []struct {
		name string
		fc   SessionsFileConfig
		want bool
	}{
		{"absent keys", SessionsFileConfig{}, true},
		{"enabled true", SessionsFileConfig{Agent: AgentSessionFileConfig{Enabled: &enabled}}, true},
		{"enabled false", SessionsFileConfig{Agent: AgentSessionFileConfig{Enabled: &disabled}}, false},
		{"mode off", SessionsFileConfig{Agent: AgentSessionFileConfig{Mode: "off"}}, false},
		{"enabled true, mode off", SessionsFileConfig{Agent: AgentSessionFileConfig{Enabled: &enabled, Mode: "off"}}, false},
		{"mode preferred", SessionsFileConfig{Agent: AgentSessionFileConfig{Mode: "preferred"}}, true},
	} {
		if got := tt.fc.AgentEnabled(); got != tt.want {
			t.Errorf("%s: AgentEnabled() = %v, want %v", tt.name, got, tt.want)
		}
	}
}
