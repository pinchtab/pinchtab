package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateTransactionPolicy(t *testing.T) {
	valid := TransactionPolicyConfig{Enabled: true, Hosts: []string{"supplier.example"}, DenyRules: []TransactionPolicyRule{{Method: "POST", PathPrefix: "/checkout"}}}
	if errs := ValidateFileConfig(&FileConfig{Security: SecurityConfig{TransactionPolicy: valid}}); len(errs) != 0 {
		t.Fatalf("valid policy errors: %v", errs)
	}
	invalid := []TransactionPolicyConfig{
		{Enabled: true},
		{Enabled: true, Hosts: []string{"https://supplier.example"}, DenyRules: []TransactionPolicyRule{{Method: "POST", PathPrefix: "/checkout"}}},
		{Enabled: true, Hosts: []string{"supplier.example:443"}, DenyRules: []TransactionPolicyRule{{Method: "POST", PathPrefix: "/checkout"}}},
		{Enabled: true, Hosts: []string{"*.supplier.example"}, DenyRules: []TransactionPolicyRule{{Method: "POST", PathPrefix: "/checkout"}}},
		{Enabled: true, Hosts: []string{"supplier..example"}, DenyRules: []TransactionPolicyRule{{Method: "POST", PathPrefix: "/checkout"}}},
		{Enabled: true, Hosts: []string{"supplier.example"}, DenyRules: []TransactionPolicyRule{{Method: "CONNECT", PathPrefix: "checkout?x=1"}}},
		{Enabled: true, Hosts: []string{"supplier.example"}, DenyRules: []TransactionPolicyRule{{Method: "POST", PathPrefix: "/checkout", QueryParam: "wc-ajax"}}},
		{Enabled: true, Hosts: []string{"supplier.example"}, DenyRules: []TransactionPolicyRule{{Method: "POST", PathPrefix: "/checkout", QueryParam: "wc ajax", QueryValue: "checkout"}}},
		{Enabled: true, Hosts: []string{"supplier.example"}, DenyRules: []TransactionPolicyRule{{Method: "POST", PathPrefix: "/", PathSegment: "orders/cancel"}}},
	}
	for _, policy := range invalid {
		if errs := ValidateFileConfig(&FileConfig{Security: SecurityConfig{TransactionPolicy: policy}}); len(errs) == 0 {
			t.Fatalf("invalid policy %+v unexpectedly passed", policy)
		}
	}
	if errs := ValidateFileConfig(&FileConfig{Security: SecurityConfig{TransactionPolicy: TransactionPolicyConfig{Enabled: false, Hosts: []string{"https://not-a-host"}}}}); len(errs) != 0 {
		t.Fatalf("disabled policy should be a no-op: %v", errs)
	}
	withExtension := FileConfig{Browser: BrowserConfig{ExtensionPaths: []string{"/operator-extension"}}, Security: SecurityConfig{TransactionPolicy: valid}}
	if errs := ValidateFileConfig(&withExtension); len(errs) == 0 {
		t.Fatal("enabled policy accepted operator extension paths")
	}
}

func TestLoadRejectsInvalidEnabledTransactionPolicy(t *testing.T) {
	clearConfigEnvVars(t)
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"security":{"transactionPolicy":{"enabled":true,"hosts":[],"denyRules":[]}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PINCHTAB_CONFIG", path)
	defer func() {
		if recover() == nil {
			t.Fatal("Load did not reject invalid enabled transaction policy")
		}
	}()
	Load()
}

func TestLoadDoesNotPanicForUnrelatedValidationError(t *testing.T) {
	clearConfigEnvVars(t)
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"server":{"port":"invalid"},"security":{"transactionPolicy":{"enabled":true,"hosts":["supplier.example"],"denyRules":[{"method":"POST","pathPrefix":"/checkout"}]}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PINCHTAB_CONFIG", path)
	Load()
}

func TestApplyFileConfigToRuntimeDoesNotToggleTransactionPolicy(t *testing.T) {
	cfg := &RuntimeConfig{TransactionPolicy: TransactionPolicyConfig{Enabled: true}}
	fc := DefaultFileConfig()
	fc.Security.TransactionPolicy.Enabled = false
	ApplyFileConfigToRuntime(cfg, &fc)
	if !cfg.TransactionPolicy.Enabled {
		t.Fatal("runtime transaction policy changed without a restart")
	}
}

func TestTransactionPolicyConfigRoundTrip(t *testing.T) {
	fc := DefaultFileConfig()
	fc.Security.TransactionPolicy = TransactionPolicyConfig{Enabled: true, Hosts: []string{"supplier.example"}, DenyRules: []TransactionPolicyRule{{Method: "POST", PathPrefix: "/checkout"}}, AllowRules: []TransactionPolicyRule{{Method: "POST", PathPrefix: "/cart"}}}
	cfg := &RuntimeConfig{}
	applyFileConfig(cfg, &fc) // Load uses this startup-only path.
	if !cfg.TransactionPolicy.Enabled || len(cfg.TransactionPolicy.AllowRules) != 1 {
		t.Fatalf("runtime policy = %+v", cfg.TransactionPolicy)
	}
	roundTripped := FileConfigFromRuntime(cfg)
	if got := roundTripped.Security.TransactionPolicy; !got.Enabled || len(got.DenyRules) != 1 {
		t.Fatalf("round-trip policy = %+v", got)
	}
	data, err := json.Marshal(roundTripped)
	if err != nil {
		t.Fatal(err)
	}
	var parsed FileConfig
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}
	if !parsed.Security.TransactionPolicy.Enabled || len(parsed.Security.TransactionPolicy.AllowRules) != 1 {
		t.Fatalf("JSON round-trip policy = %+v", parsed.Security.TransactionPolicy)
	}
}
