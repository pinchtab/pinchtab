package config

import (
	"strings"
	"testing"
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
		{"full", false},
		{"", false},      // empty is ok
		{"medium", true}, // removed, no longer valid
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
	// Test that the valid values match the validation functions
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
}

// --- IDPI validation tests ---

// TestValidateIDPIConfig_Disabled verifies that a disabled IDPI config produces
// no errors regardless of what fields are set.
func TestValidateIDPIConfig_Disabled(t *testing.T) {
	errs := validateIDPIConfig(IDPIConfig{
		Enabled:        false,
		AllowedDomains: []string{"", "  ", "file:///etc/passwd"},
		CustomPatterns: []string{"", "  "},
	})
	if len(errs) != 0 {
		t.Errorf("expected no errors when IDPI disabled, got: %v", errs)
	}
}

// TestValidateIDPIConfig_ValidConfig verifies that a well-formed enabled config
// produces no errors.
func TestValidateIDPIConfig_ValidConfig(t *testing.T) {
	errs := validateIDPIConfig(IDPIConfig{
		Enabled:        true,
		AllowedDomains: []string{"example.com", "*.example.com", "*"},
		CustomPatterns: []string{"exfiltrate this", "data leak"},
	})
	if len(errs) != 0 {
		t.Errorf("expected no errors for valid IDPI config, got: %v", errs)
	}
}

// TestValidateIDPIConfig_EmptyDomain verifies that an empty or whitespace-only
// domain pattern is rejected.
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
				Enabled:        true,
				AllowedDomains: []string{tc.domain},
			})
			if len(errs) == 0 {
				t.Errorf("expected error for empty domain %q, got none", tc.domain)
			}
			if len(errs) > 0 && !strings.Contains(errs[0].Error(), "security.idpi.allowedDomains") {
				t.Errorf("expected field name in error, got: %v", errs[0])
			}
		})
	}
}

// TestValidateIDPIConfig_DomainWithInternalWhitespace verifies that a domain
// pattern containing internal spaces is rejected.
func TestValidateIDPIConfig_DomainWithInternalWhitespace(t *testing.T) {
	errs := validateIDPIConfig(IDPIConfig{
		Enabled:        true,
		AllowedDomains: []string{"example .com"},
	})
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
			Enabled:        true,
			AllowedDomains: []string{pattern},
		})
		if len(errs) == 0 {
			t.Errorf("expected error for file:// pattern %q, got none", pattern)
		}
		if len(errs) > 0 && !strings.Contains(errs[0].Error(), "file://") {
			t.Errorf("expected file:// mention in error, got: %v", errs[0])
		}
	}
}

// TestValidateIDPIConfig_EmptyCustomPattern verifies that an empty or
// whitespace-only custom pattern is rejected.
func TestValidateIDPIConfig_EmptyCustomPattern(t *testing.T) {
	cases := []string{"", "  ", "\t"}
	for _, p := range cases {
		errs := validateIDPIConfig(IDPIConfig{
			Enabled:        true,
			CustomPatterns: []string{p},
		})
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
		AllowedDomains: []string{"", "file:///bad"},
		CustomPatterns: []string{"", "   "},
	})
	if len(errs) < 4 {
		t.Errorf("expected at least 4 IDPI errors, got %d: %v", len(errs), errs)
	}
}

// TestValidateFileConfig_IDPIPassthrough verifies that ValidateFileConfig
// surfaces IDPI errors alongside other config errors.
func TestValidateFileConfig_IDPIPassthrough(t *testing.T) {
	fc := &FileConfig{
		Security: SecurityConfig{
			IDPI: IDPIConfig{
				Enabled:        true,
				AllowedDomains: []string{""},   // invalid
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
		})
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
		})
		if len(errs) != 0 {
			t.Errorf("expected no error for scanTimeoutSec=0, got: %v", errs)
		}
	})

	t.Run("PositiveIsValid", func(t *testing.T) {
		errs := validateIDPIConfig(IDPIConfig{
			Enabled:        true,
			ScanTimeoutSec: 10,
		})
		if len(errs) != 0 {
			t.Errorf("expected no error for scanTimeoutSec=10, got: %v", errs)
		}
	})
}
