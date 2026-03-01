package profiles

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
)

func TestProfileManagerCreateAndList(t *testing.T) {
	dir := t.TempDir()
	pm := NewProfileManager(dir)

	if err := pm.Create("test-profile"); err != nil {
		t.Fatal(err)
	}

	profileDir := filepath.Join(dir, profileID("test-profile"))
	if _, err := os.Stat(profileDir); err != nil {
		t.Fatalf("profile directory not created: %s", profileDir)
	}

	defaultDir := filepath.Join(profileDir, "Default")
	if _, err := os.Stat(defaultDir); err != nil {
		t.Fatalf("Default directory not created: %s", defaultDir)
	}

	profiles, err := pm.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(profiles))
	}
	if profiles[0].Name != "test-profile" {
		t.Errorf("expected name test-profile, got %s", profiles[0].Name)
	}
	if profiles[0].Source != "created" {
		t.Errorf("expected source created, got %s", profiles[0].Source)
	}
	if !profiles[0].PathExists {
		t.Errorf("profile path should exist")
	}
	if profiles[0].Path != profileDir {
		t.Errorf("expected path %s, got %s", profileDir, profiles[0].Path)
	}
}

func TestProfileManagerCreateDuplicate(t *testing.T) {
	dir := t.TempDir()
	pm := NewProfileManager(dir)

	_ = pm.Create("dup")
	err := pm.Create("dup")
	if err == nil {
		t.Fatal("expected error on duplicate create")
	}
}

func TestProfileManagerImport(t *testing.T) {
	dir := t.TempDir()
	pm := NewProfileManager(dir)

	src := filepath.Join(t.TempDir(), "chrome-src")
	_ = os.MkdirAll(filepath.Join(src, "Default"), 0755)
	_ = os.WriteFile(filepath.Join(src, "Default", "Preferences"), []byte(`{}`), 0644)

	if err := pm.Import("imported-profile", src); err != nil {
		t.Fatal(err)
	}

	profiles, _ := pm.List()
	if len(profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(profiles))
	}
	if profiles[0].Source != "imported" {
		t.Errorf("expected source imported, got %s", profiles[0].Source)
	}
}

func TestProfileManagerImportBadSource(t *testing.T) {
	dir := t.TempDir()
	pm := NewProfileManager(dir)

	err := pm.Import("bad", "/nonexistent/path")
	if err == nil {
		t.Fatal("expected error on bad source")
	}
}

func TestProfileManagerListReadsAccountFromPreferences(t *testing.T) {
	dir := t.TempDir()
	pm := NewProfileManager(dir)
	if err := pm.Create("acc-pref"); err != nil {
		t.Fatal(err)
	}

	prefsPath := filepath.Join(dir, profileID("acc-pref"), "Default", "Preferences")
	prefs := `{"account_info":[{"email":"alice@example.com","full_name":"Alice"}]}`
	if err := os.WriteFile(prefsPath, []byte(prefs), 0644); err != nil {
		t.Fatal(err)
	}

	profiles, err := pm.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(profiles))
	}
	if profiles[0].AccountEmail != "alice@example.com" {
		t.Fatalf("expected account email alice@example.com, got %q", profiles[0].AccountEmail)
	}
	if profiles[0].AccountName != "Alice" {
		t.Fatalf("expected account name Alice, got %q", profiles[0].AccountName)
	}
	if !profiles[0].HasAccount {
		t.Fatal("expected hasAccount=true")
	}
}

func TestProfileManagerListReadsLocalStateIdentity(t *testing.T) {
	dir := t.TempDir()
	pm := NewProfileManager(dir)
	if err := pm.Create("acc-local"); err != nil {
		t.Fatal(err)
	}

	localStatePath := filepath.Join(dir, profileID("acc-local"), "Local State")
	localState := `{"profile":{"info_cache":{"Default":{"name":"Work","user_name":"bob@example.com","gaia_name":"Bob","gaia_id":"123"}}}}`
	if err := os.WriteFile(localStatePath, []byte(localState), 0644); err != nil {
		t.Fatal(err)
	}

	profiles, err := pm.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(profiles))
	}
	if profiles[0].ChromeProfileName != "Work" {
		t.Fatalf("expected chrome profile name Work, got %q", profiles[0].ChromeProfileName)
	}
	if profiles[0].AccountEmail != "bob@example.com" {
		t.Fatalf("expected account email bob@example.com, got %q", profiles[0].AccountEmail)
	}
	if profiles[0].AccountName != "Bob" {
		t.Fatalf("expected account name Bob, got %q", profiles[0].AccountName)
	}
	if !profiles[0].HasAccount {
		t.Fatal("expected hasAccount=true")
	}
}

func TestProfileManagerReset(t *testing.T) {
	dir := t.TempDir()
	pm := NewProfileManager(dir)
	_ = pm.Create("reset-me")

	sessDir := filepath.Join(dir, profileID("reset-me"), "Default", "Sessions")
	_ = os.MkdirAll(sessDir, 0755)
	_ = os.WriteFile(filepath.Join(sessDir, "session1"), []byte("data"), 0644)

	cacheDir := filepath.Join(dir, profileID("reset-me"), "Default", "Cache")
	_ = os.MkdirAll(cacheDir, 0755)

	if err := pm.Reset("reset-me"); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(sessDir); !os.IsNotExist(err) {
		t.Error("Sessions dir should be removed after reset")
	}
	if _, err := os.Stat(cacheDir); !os.IsNotExist(err) {
		t.Error("Cache dir should be removed after reset")
	}

	if _, err := os.Stat(filepath.Join(dir, profileID("reset-me"))); err != nil {
		t.Error("Profile dir should still exist after reset")
	}
}

func TestProfileManagerResetNotFound(t *testing.T) {
	pm := NewProfileManager(t.TempDir())
	err := pm.Reset("nope")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestProfileManagerDelete(t *testing.T) {
	dir := t.TempDir()
	pm := NewProfileManager(dir)
	_ = pm.Create("delete-me")

	if err := pm.Delete("delete-me"); err != nil {
		t.Fatal(err)
	}

	profiles, _ := pm.List()
	if len(profiles) != 0 {
		t.Errorf("expected 0 profiles after delete, got %d", len(profiles))
	}
}

func TestActionTracker(t *testing.T) {
	at := NewActionTracker()
	profName := fmt.Sprintf("prof-%d", time.Now().UnixNano())

	for i := 0; i < 5; i++ {
		at.Record(profName, bridge.ActionRecord{
			Timestamp:  time.Now().Add(time.Duration(i) * time.Second),
			Method:     "GET",
			Endpoint:   "/snapshot",
			URL:        "https://example.com",
			DurationMs: 100,
			Status:     200,
		})
	}

	logs := at.GetLogs(profName, 3)
	if len(logs) != 3 {
		t.Errorf("expected 3 logs, got %d", len(logs))
	}

	report := at.Analyze(profName)
	if report.TotalActions != 5 {
		t.Errorf("expected 5 total actions, got %d", report.TotalActions)
	}
}

func TestProfileHandlerList(t *testing.T) {
	pm := NewProfileManager(t.TempDir())
	_ = pm.Create("a")
	_ = pm.Create("b")

	mux := http.NewServeMux()
	pm.RegisterHandlers(mux)

	req := httptest.NewRequest("GET", "/profiles", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var profiles []bridge.ProfileInfo
	_ = json.NewDecoder(w.Body).Decode(&profiles)
	if len(profiles) != 2 {
		t.Errorf("expected 2 profiles, got %d", len(profiles))
	}
	for _, p := range profiles {
		if p.Path == "" {
			t.Fatalf("expected path to be present for profile %q", p.Name)
		}
		if !p.PathExists {
			t.Fatalf("expected pathExists=true for profile %q", p.Name)
		}
	}
}

func TestProfileHandlerCreate(t *testing.T) {
	baseDir := t.TempDir()
	pm := NewProfileManager(baseDir)
	mux := http.NewServeMux()
	pm.RegisterHandlers(mux)

	body := `{"name": "new-profile"}`
	req := httptest.NewRequest("POST", "/profiles/create", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	idDir := filepath.Join(baseDir, profileID("new-profile"))
	if _, err := os.Stat(idDir); err != nil {
		t.Fatalf("expected id-based directory to exist: %s", idDir)
	}
	nameDir := filepath.Join(baseDir, "new-profile")
	if _, err := os.Stat(nameDir); !os.IsNotExist(err) {
		t.Fatalf("expected name-based directory not to exist: %s", nameDir)
	}
}

func TestProfileHandlerReset(t *testing.T) {
	pm := NewProfileManager(t.TempDir())
	_ = pm.Create("resettable")
	mux := http.NewServeMux()
	pm.RegisterHandlers(mux)

	req := httptest.NewRequest("POST", "/profiles/resettable/reset", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestProfileHandlerDelete(t *testing.T) {
	pm := NewProfileManager(t.TempDir())
	_ = pm.Create("deletable")
	mux := http.NewServeMux()
	pm.RegisterHandlers(mux)

	req := httptest.NewRequest("DELETE", "/profiles/deletable", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestProfileMetaReadWrite(t *testing.T) {
	dir := t.TempDir()
	pm := NewProfileManager(dir)

	meta := ProfileMeta{
		UseWhen:     "I need to access work email",
		Description: "Work profile for corporate tasks",
	}
	if err := pm.CreateWithMeta("work-profile", meta); err != nil {
		t.Fatal(err)
	}

	readMeta := readProfileMeta(filepath.Join(dir, profileID("work-profile")))
	if readMeta.UseWhen != "I need to access work email" {
		t.Errorf("expected useWhen 'I need to access work email', got %q", readMeta.UseWhen)
	}
	if readMeta.Description != "Work profile for corporate tasks" {
		t.Errorf("expected description 'Work profile for corporate tasks', got %q", readMeta.Description)
	}
}

func TestProfileUpdateMeta(t *testing.T) {
	pm := NewProfileManager(t.TempDir())
	mux := http.NewServeMux()
	pm.RegisterHandlers(mux)

	_ = pm.Create("updatable")

	body := `{"name":"updatable","useWhen":"Updated use case","description":"Updated description"}`
	req := httptest.NewRequest("PATCH", "/profiles/meta", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestProfileCreateWithUseWhen(t *testing.T) {
	pm := NewProfileManager(t.TempDir())
	mux := http.NewServeMux()
	pm.RegisterHandlers(mux)

	body := `{"name":"test-usewhen","useWhen":"For testing purposes"}`
	req := httptest.NewRequest("POST", "/profiles/create", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	profiles, err := pm.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(profiles))
	}
	if profiles[0].UseWhen != "For testing purposes" {
		t.Errorf("expected useWhen 'For testing purposes', got %q", profiles[0].UseWhen)
	}
}

func TestProfileListIncludesUseWhen(t *testing.T) {
	pm := NewProfileManager(t.TempDir())

	meta := ProfileMeta{UseWhen: "Personal browsing"}
	_ = pm.CreateWithMeta("personal", meta)

	profiles, err := pm.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(profiles))
	}
	if profiles[0].UseWhen != "Personal browsing" {
		t.Errorf("expected useWhen 'Personal browsing', got %q", profiles[0].UseWhen)
	}
}

// === Security Validation Tests ===

func TestValidateProfileName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		errMsg  string
	}{
		// Valid names
		{"valid simple", "my-profile", false, ""},
		{"valid with numbers", "profile123", false, ""},
		{"valid with underscore", "my_profile", false, ""},
		{"valid with dots", "my.profile", false, ""},
		{"valid single char", "a", false, ""},

		// Empty name
		{"empty", "", true, "cannot be empty"},

		// Path traversal attempts
		{"double dot", "..", true, "cannot contain '..'"},
		{"double dot prefix", "../test", true, "cannot contain '..'"},
		{"double dot suffix", "test/..", true, "cannot contain '..'"},
		{"double dot middle", "test/../other", true, "cannot contain '..'"},
		{"triple dot", "...", true, "cannot contain '..'"},
		{"double dot no slash", "..test", true, "cannot contain '..'"},

		// Path separator attempts
		{"forward slash", "test/profile", true, "cannot contain '/'"},
		{"forward slash prefix", "/test", true, "cannot contain '/'"},
		{"forward slash suffix", "test/", true, "cannot contain '/'"},
		{"backslash", "test\\profile", true, "cannot contain '/'"},
		{"backslash prefix", "\\test", true, "cannot contain '/'"},

		// Combined attacks
		{"traversal with slash", "../../../etc/passwd", true, "cannot contain"},
		{"traversal windows", "..\\..\\system32", true, "cannot contain"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateProfileName(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateProfileName(%q) = nil, want error containing %q", tt.input, tt.errMsg)
				} else if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("ValidateProfileName(%q) = %q, want error containing %q", tt.input, err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateProfileName(%q) = %v, want nil", tt.input, err)
				}
			}
		})
	}
}

func TestProfileCreateRejectsPathTraversal(t *testing.T) {
	pm := NewProfileManager(t.TempDir())

	badNames := []string{
		"../test",
		"..\\test",
		"test/../other",
		"../../etc/passwd",
		"test/subdir",
		"/absolute",
	}

	for _, name := range badNames {
		t.Run(name, func(t *testing.T) {
			err := pm.Create(name)
			if err == nil {
				t.Errorf("Create(%q) should have returned error", name)
			}
		})
	}
}

func TestProfileHandlerCreateRejectsPathTraversal(t *testing.T) {
	pm := NewProfileManager(t.TempDir())
	mux := http.NewServeMux()
	pm.RegisterHandlers(mux)

	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{"path traversal ..", `{"name":"../malicious"}`, 400},
		{"path traversal /", `{"name":"test/nested"}`, 400},
		{"path traversal backslash", `{"name":"test\\nested"}`, 400},
		{"empty name", `{"name":""}`, 400},
		{"valid name", `{"name":"valid-profile"}`, 200},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/profiles/create", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("POST /profiles/create with %s: got status %d, want %d. Body: %s",
					tt.body, w.Code, tt.wantStatus, w.Body.String())
			}
		})
	}
}

func TestProfileHandlerCreateReturns409OnDuplicate(t *testing.T) {
	pm := NewProfileManager(t.TempDir())
	mux := http.NewServeMux()
	pm.RegisterHandlers(mux)

	// Create first profile
	body := `{"name":"duplicate-test"}`
	req := httptest.NewRequest("POST", "/profiles/create", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("first create failed: %d %s", w.Code, w.Body.String())
	}

	// Try to create duplicate
	req = httptest.NewRequest("POST", "/profiles/create", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 409 {
		t.Errorf("duplicate create: got status %d, want 409. Body: %s", w.Code, w.Body.String())
	}
}

func TestProfileImportRejectsPathTraversal(t *testing.T) {
	pm := NewProfileManager(t.TempDir())

	// Create a valid source directory
	src := filepath.Join(t.TempDir(), "chrome-src")
	_ = os.MkdirAll(filepath.Join(src, "Default"), 0755)
	_ = os.WriteFile(filepath.Join(src, "Default", "Preferences"), []byte(`{}`), 0644)

	badNames := []string{
		"../imported",
		"test/nested",
		"..\\windows",
	}

	for _, name := range badNames {
		t.Run(name, func(t *testing.T) {
			err := pm.Import(name, src)
			if err == nil {
				t.Errorf("Import(%q, ...) should have returned error", name)
			}
		})
	}
}

func TestProfileResetRejectsPathTraversal(t *testing.T) {
	pm := NewProfileManager(t.TempDir())
	_ = pm.Create("legit")

	badNames := []string{
		"../legit",
		"legit/../other",
	}

	for _, name := range badNames {
		t.Run(name, func(t *testing.T) {
			err := pm.Reset(name)
			if err == nil {
				t.Errorf("Reset(%q) should have returned error", name)
			}
		})
	}
}

func TestProfileDeleteRejectsPathTraversal(t *testing.T) {
	pm := NewProfileManager(t.TempDir())
	_ = pm.Create("legit")

	badNames := []string{
		"../legit",
		"legit/../other",
	}

	for _, name := range badNames {
		t.Run(name, func(t *testing.T) {
			err := pm.Delete(name)
			if err == nil {
				t.Errorf("Delete(%q) should have returned error", name)
			}
		})
	}
}
