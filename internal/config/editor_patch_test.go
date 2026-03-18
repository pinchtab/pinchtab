package config

import "testing"

func TestPatchConfigJSON(t *testing.T) {
	fc := &FileConfig{
		Server: ServerConfig{
			Port: "9867",
			Bind: "127.0.0.1",
		},
		InstanceDefaults: InstanceDefaultsConfig{
			StealthLevel: "light",
		},
	}

	// Patch to change port and add token
	patch := `{"server": {"port": "8080", "token": "secret"}}`
	if err := PatchConfigJSON(fc, patch); err != nil {
		t.Fatalf("PatchConfigJSON() error = %v", err)
	}

	if fc.Server.Port != "8080" {
		t.Errorf("port = %v, want 8080", fc.Server.Port)
	}
	if fc.Server.Token != "secret" {
		t.Errorf("token = %v, want secret", fc.Server.Token)
	}
	// Bind should be preserved
	if fc.Server.Bind != "127.0.0.1" {
		t.Errorf("bind = %v, want 127.0.0.1 (should be preserved)", fc.Server.Bind)
	}
	// InstanceDefaults.StealthLevel should be preserved
	if fc.InstanceDefaults.StealthLevel != "light" {
		t.Errorf("stealthLevel = %v, want light (should be preserved)", fc.InstanceDefaults.StealthLevel)
	}
}

func TestPatchConfigJSON_NestedMerge(t *testing.T) {
	fc := &FileConfig{
		InstanceDefaults: InstanceDefaultsConfig{
			StealthLevel:      "light",
			TabEvictionPolicy: "reject",
		},
	}

	// Patch instanceDefaults section, should merge not replace
	patch := `{"instanceDefaults": {"stealthLevel": "full"}}`
	if err := PatchConfigJSON(fc, patch); err != nil {
		t.Fatalf("PatchConfigJSON() error = %v", err)
	}

	if fc.InstanceDefaults.StealthLevel != "full" {
		t.Errorf("stealthLevel = %v, want full", fc.InstanceDefaults.StealthLevel)
	}
	// tabEvictionPolicy should be preserved
	if fc.InstanceDefaults.TabEvictionPolicy != "reject" {
		t.Errorf("tabEvictionPolicy = %v, want reject (should be preserved)", fc.InstanceDefaults.TabEvictionPolicy)
	}
}

func TestPatchConfigJSON_InvalidJSON(t *testing.T) {
	fc := &FileConfig{}
	err := PatchConfigJSON(fc, "not json")
	if err == nil {
		t.Error("PatchConfigJSON() should fail on invalid JSON")
	}
}
