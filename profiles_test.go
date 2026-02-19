package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProfileManagerCreateAndList(t *testing.T) {
	dir := t.TempDir()
	pm := NewProfileManager(dir)

	if err := pm.Create("test-profile"); err != nil {
		t.Fatal(err)
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

	prefsPath := filepath.Join(dir, "acc-pref", "Default", "Preferences")
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

	localStatePath := filepath.Join(dir, "acc-local", "Local State")
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

	sessDir := filepath.Join(dir, "reset-me", "Default", "Sessions")
	_ = os.MkdirAll(sessDir, 0755)
	_ = os.WriteFile(filepath.Join(sessDir, "session1"), []byte("data"), 0644)

	cacheDir := filepath.Join(dir, "reset-me", "Default", "Cache")
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

	if _, err := os.Stat(filepath.Join(dir, "reset-me")); err != nil {
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

	for i := 0; i < 5; i++ {
		at.Record("prof1", ActionRecord{
			Timestamp:  time.Now().Add(time.Duration(i) * time.Second),
			Method:     "GET",
			Endpoint:   "/snapshot",
			URL:        "https://example.com",
			DurationMs: 100,
			Status:     200,
		})
	}

	logs := at.GetLogs("prof1", 3)
	if len(logs) != 3 {
		t.Errorf("expected 3 logs, got %d", len(logs))
	}

	report := at.Analyze("prof1")
	if report.TotalActions != 5 {
		t.Errorf("expected 5 total actions, got %d", report.TotalActions)
	}
	if len(report.TopEndpoints) == 0 {
		t.Error("expected at least one top endpoint")
	}
}

func TestActionTrackerRepeatDetection(t *testing.T) {
	at := NewActionTracker()

	for i := 0; i < 10; i++ {
		at.Record("poller", ActionRecord{
			Timestamp:  time.Now().Add(time.Duration(i) * 3 * time.Second),
			Endpoint:   "/snapshot",
			URL:        "https://example.com/page",
			DurationMs: 50,
			Status:     200,
		})
	}

	report := at.Analyze("poller")
	if len(report.RepeatPatterns) == 0 {
		t.Error("expected repeat patterns to be detected")
	}
	if len(report.Suggestions) == 0 || report.Suggestions[0] == "No optimization suggestions — usage looks efficient." {
		t.Error("expected optimization suggestions for rapid polling")
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

	var profiles []ProfileInfo
	_ = json.NewDecoder(w.Body).Decode(&profiles)
	if len(profiles) != 2 {
		t.Errorf("expected 2 profiles, got %d", len(profiles))
	}
}

func TestProfileHandlerCreate(t *testing.T) {
	pm := NewProfileManager(t.TempDir())
	mux := http.NewServeMux()
	pm.RegisterHandlers(mux)

	body := `{"name": "new-profile"}`
	req := httptest.NewRequest("POST", "/profiles/create", strings.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 201 {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
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
