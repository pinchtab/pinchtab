package profiles

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadChromeProfileIdentity(t *testing.T) {
	tmpDir := t.TempDir()
	
	// 1. Test empty profile
	name, email, accName, hasAcc := readChromeProfileIdentity(tmpDir)
	if name != "" || email != "" || accName != "" || hasAcc != false {
		t.Errorf("expected empty results for empty dir, got %q, %q, %q, %v", name, email, accName, hasAcc)
	}

	// 2. Test with Local State only
	localStateDir := tmpDir
	localStatePath := filepath.Join(localStateDir, "Local State")
	localStateContent := `{
		"profile": {
			"info_cache": {
				"Default": {
					"name": "Work Profile",
					"user_name": "work@example.com",
					"gaia_name": "Work User",
					"gaia_id": "12345",
					"is_consented_primary_account": true
				}
			}
		}
	}`
	if err := os.WriteFile(localStatePath, []byte(localStateContent), 0644); err != nil {
		t.Fatal(err)
	}

	name, email, accName, hasAcc = readChromeProfileIdentity(tmpDir)
	if name != "Work Profile" || email != "work@example.com" || accName != "Work User" || !hasAcc {
		t.Errorf("Local State parsing failed: got %q, %q, %q, %v", name, email, accName, hasAcc)
	}

	// 3. Test with Preferences (should override/supplement)
	prefsDir := filepath.Join(tmpDir, "Default")
	if err := os.MkdirAll(prefsDir, 0755); err != nil {
		t.Fatal(err)
	}
	prefsPath := filepath.Join(prefsDir, "Preferences")
	prefsContent := `{
		"account_info": [
			{
				"email": "pref@example.com",
				"full_name": "Pref User",
				"gaia": "67890"
			}
		]
	}`
	if err := os.WriteFile(prefsPath, []byte(prefsContent), 0644); err != nil {
		t.Fatal(err)
	}

	name, email, accName, hasAcc = readChromeProfileIdentity(tmpDir)
	// email and accName should come from Preferences if available
	if name != "Work Profile" || email != "pref@example.com" || accName != "Pref User" || !hasAcc {
		t.Errorf("Preferences parsing/override failed: got %q, %q, %q, %v", name, email, accName, hasAcc)
	}
}

func TestReadJSON_Malformed(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "bad.json")
	if err := os.WriteFile(path, []byte(`{ "invalid": json `), 0644); err != nil {
		t.Fatal(err)
	}

	var out map[string]any
	if readJSON(path, &out) {
		t.Error("readJSON should return false for malformed JSON")
	}
}

func TestProfileMeta_ReadWrite(t *testing.T) {
	tmpDir := t.TempDir()
	meta := ProfileMeta{
		ID:          "test-id",
		UseWhen:     "testing",
		Description: "a test profile",
	}

	if err := writeProfileMeta(tmpDir, meta); err != nil {
		t.Fatalf("failed to write meta: %v", err)
	}

	readMeta := readProfileMeta(tmpDir)
	if readMeta.ID != meta.ID || readMeta.UseWhen != meta.UseWhen || readMeta.Description != meta.Description {
		t.Errorf("read meta does not match written meta: got %+v, want %+v", readMeta, meta)
	}
}
