package profiles

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/activity"
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

func TestProfileManagerImportNormalizesSourcePath(t *testing.T) {
	dir := t.TempDir()
	pm := NewProfileManager(dir)

	srcRoot := t.TempDir()
	src := filepath.Join(srcRoot, "chrome-src")
	_ = os.MkdirAll(filepath.Join(src, "Default"), 0755)
	_ = os.WriteFile(filepath.Join(src, "Default", "Preferences"), []byte(`{}`), 0644)

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	relSource, err := filepath.Rel(cwd, src)
	if err != nil {
		t.Fatal(err)
	}

	if err := pm.Import("normalized-import", relSource); err != nil {
		t.Fatal(err)
	}

	importMarker, err := os.ReadFile(filepath.Join(dir, profileID("normalized-import"), ".pinchtab-imported"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(importMarker), filepath.Clean(src); got != want {
		t.Fatalf("expected normalized source %q, got %q", want, got)
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

func TestProfileManagerImportRejectsSourceOutsideAllowedRoots(t *testing.T) {
	dir := t.TempDir()
	pm := NewProfileManager(dir)

	var outside string
	switch runtime.GOOS {
	case "windows":
		systemRoot := os.Getenv("SystemRoot")
		if systemRoot == "" {
			t.Skip("SystemRoot not set")
		}
		outside = systemRoot
	default:
		outside = string(os.PathSeparator) + "etc"
	}

	err := pm.Import("outside-root", outside)
	if err == nil || !strings.Contains(err.Error(), "must be within") {
		t.Fatalf("expected allowed root error, got %v", err)
	}
}

func TestProfileManagerImportRejectsSymlinkSource(t *testing.T) {
	dir := t.TempDir()
	pm := NewProfileManager(dir)

	src := filepath.Join(t.TempDir(), "chrome-src")
	_ = os.MkdirAll(filepath.Join(src, "Default"), 0755)
	_ = os.WriteFile(filepath.Join(src, "Default", "Preferences"), []byte(`{}`), 0644)

	link := filepath.Join(t.TempDir(), "chrome-link")
	if err := os.Symlink(src, link); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	err := pm.Import("bad-link", link)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink source import to fail, got %v", err)
	}
}

func TestProfileManagerImportRejectsIntermediateSymlinkOutsideRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("rooted symlink test requires Unix symlink semantics")
	}

	dir := t.TempDir()
	pm := NewProfileManager(dir)
	link := filepath.Join(t.TempDir(), "escape")
	if err := os.Symlink(string(os.PathSeparator), link); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	err := pm.Import("escaped-source", filepath.Join(link, "etc"))
	if err == nil || !strings.Contains(err.Error(), "source path invalid") {
		t.Fatalf("expected rooted source validation to reject the escaping symlink, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, profileID("escaped-source"))); !os.IsNotExist(statErr) {
		t.Fatalf("rejected import created a destination: %v", statErr)
	}
}

func TestProfileManagerImportRejectsSymlinkEntry(t *testing.T) {
	dir := t.TempDir()
	pm := NewProfileManager(dir)

	src := filepath.Join(t.TempDir(), "chrome-src")
	_ = os.MkdirAll(filepath.Join(src, "Default"), 0755)
	_ = os.WriteFile(filepath.Join(src, "Default", "Preferences"), []byte(`{}`), 0644)

	target := filepath.Join(t.TempDir(), "outside-cookie.txt")
	if err := os.WriteFile(target, []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(src, "Default", "link.txt")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	err := pm.Import("bad-entry", src)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink entry import to fail, got %v", err)
	}
}

func TestProfileManagerListReadsAccountFromPreferences(t *testing.T) {
	dir := t.TempDir()
	pm := NewProfileManager(dir)
	if err := pm.Create("acc-pref"); err != nil {
		t.Fatal(err)
	}

	prefsPath := filepath.Join(dir, profileID("acc-pref"), "Default", "Preferences")
	prefs := `{"account_info":[{"email":"alice@pinchtab.com","full_name":"Alice"}]}`
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
	if profiles[0].AccountEmail != "alice@pinchtab.com" {
		t.Fatalf("expected account email alice@pinchtab.com, got %q", profiles[0].AccountEmail)
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
	localState := `{"profile":{"info_cache":{"Default":{"name":"Work","user_name":"bob@pinchtab.com","gaia_name":"Bob","gaia_id":"123"}}}}`
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
	if profiles[0].AccountEmail != "bob@pinchtab.com" {
		t.Fatalf("expected account email bob@pinchtab.com, got %q", profiles[0].AccountEmail)
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

func TestProfileManagerLogsAndAnalyticsUseActivityRecorder(t *testing.T) {
	pm := NewProfileManager(t.TempDir())
	store, err := activity.NewStore(t.TempDir(), 1)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	pm.SetActivityRecorder(store)

	profileName := fmt.Sprintf("prof-%d", time.Now().UnixNano())
	now := time.Now().UTC()
	for i := 0; i < 5; i++ {
		if err := store.Record(activity.Event{
			Timestamp:   now.Add(time.Duration(i) * time.Second),
			Source:      "server",
			ProfileName: profileName,
			Method:      "GET",
			Path:        "/snapshot",
			URL:         "https://pinchtab.com/page",
			DurationMs:  100,
			Status:      200,
		}); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}

	logs := pm.Logs(profileName, 3)
	if len(logs) != 3 {
		t.Fatalf("expected 3 logs, got %d", len(logs))
	}
	if logs[0].Endpoint != "/snapshot" {
		t.Fatalf("logs[0].Endpoint = %q, want /snapshot", logs[0].Endpoint)
	}

	report := pm.Analytics(profileName)
	if report.TotalActions != 5 {
		t.Fatalf("expected 5 total actions, got %d", report.TotalActions)
	}
	if report.Last24h != 5 {
		t.Fatalf("expected 5 last24h actions, got %d", report.Last24h)
	}
	if report.CommonHosts["pinchtab.com"] != 5 {
		t.Fatalf("CommonHosts = %#v, want pinchtab.com=5", report.CommonHosts)
	}
	if report.TopEndpoints["/snapshot"] != 5 {
		t.Fatalf("TopEndpoints = %#v, want /snapshot=5", report.TopEndpoints)
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
	id := profileID("resettable")
	mux := http.NewServeMux()
	pm.RegisterHandlers(mux)

	req := httptest.NewRequest("POST", "/profiles/"+id+"/reset", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestProfileHandlerDelete(t *testing.T) {
	pm := NewProfileManager(t.TempDir())
	_ = pm.Create("deletable")
	id := profileID("deletable")
	mux := http.NewServeMux()
	pm.RegisterHandlers(mux)

	req := httptest.NewRequest("DELETE", "/profiles/"+id, nil)
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

func TestProfileUpdateMetaRejectsInvalidProfileName(t *testing.T) {
	pm := NewProfileManager(t.TempDir())
	mux := http.NewServeMux()
	pm.RegisterHandlers(mux)

	req := httptest.NewRequest(http.MethodPatch, "/profiles/meta", strings.NewReader(`{"name":"poc';calc","description":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestProfileUpdateByIDCanClearMetadata(t *testing.T) {
	pm := NewProfileManager(t.TempDir())
	mux := http.NewServeMux()
	pm.RegisterHandlers(mux)

	if err := pm.CreateWithMeta("clearable", ProfileMeta{
		UseWhen:     "Used for work",
		Description: "Has metadata",
	}); err != nil {
		t.Fatal(err)
	}

	body := `{"useWhen":"","description":""}`
	req := httptest.NewRequest("PATCH", "/profiles/"+profileID("clearable"), strings.NewReader(body))
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
	if profiles[0].UseWhen != "" {
		t.Errorf("expected empty useWhen after clear, got %q", profiles[0].UseWhen)
	}
	if profiles[0].Description != "" {
		t.Errorf("expected empty description after clear, got %q", profiles[0].Description)
	}
}

func TestProfileUpdateByIDRejectsInvalidRename(t *testing.T) {
	pm := NewProfileManager(t.TempDir())
	mux := http.NewServeMux()
	pm.RegisterHandlers(mux)

	if err := pm.Create("renameable"); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPatch, "/profiles/"+profileID("renameable"), strings.NewReader(`{"name":"poc';calc"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
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
		{"valid with spaces", "Work Profile", false, ""},
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
		{"single quote", "poc';calc", true, "contains invalid character"},
		{"semicolon", "poc;calc", true, "contains invalid character"},
		{"pipe", "poc|calc", true, "contains invalid character"},
		{"dollar", "poc$calc", true, "contains invalid character"},
		{"backtick", "poc`calc", true, "contains invalid character"},
		{"colon", "poc:calc", true, "contains invalid character"},
		{"trailing dot", "poc.", true, "cannot end with '.'"},
		{"leading whitespace", " profile", true, "cannot start or end with whitespace"},
		{"trailing whitespace", "profile ", true, "cannot start or end with whitespace"},
		{"reserved device name", "CON", true, "reserved device name"},
		{"reserved device name with extension", "con.txt", true, "reserved device name"},
		{"reserved printer name", "LPT1", true, "reserved device name"},

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
		"poc';calc",
		"bad|name",
		"CON",
		"con.txt",
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
		{"powershell metacharacter", `{"name":"poc';calc"}`, 400},
		{"reserved device name", `{"name":"CON"}`, 400},
		{"trailing dot", `{"name":"bad."}`, 400},
		{"leading whitespace", `{"name":" bad"}`, 400},
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

func TestProfileHandlerImportRejectsInvalidProfileName(t *testing.T) {
	pm := NewProfileManager(t.TempDir())
	mux := http.NewServeMux()
	pm.RegisterHandlers(mux)

	src := filepath.Join(t.TempDir(), "chrome-src")
	if err := os.MkdirAll(filepath.Join(src, "Default"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "Default", "Preferences"), []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}

	body := fmt.Sprintf(`{"name":"poc';calc","sourcePath":%q}`, src)
	req := httptest.NewRequest(http.MethodPost, "/profiles/import", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestProfileHandlerCreateReturns409OnDuplicate(t *testing.T) {
	pm := NewProfileManager(t.TempDir())
	mux := http.NewServeMux()
	pm.RegisterHandlers(mux)

	body := `{"name":"duplicate-test"}`
	req := httptest.NewRequest("POST", "/profiles/create", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("first create failed: %d %s", w.Code, w.Body.String())
	}

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

func TestProfileRename(t *testing.T) {
	dir := t.TempDir()
	pm := NewProfileManager(dir)

	if err := pm.Create("old-name"); err != nil {
		t.Fatal(err)
	}

	if err := pm.Rename("old-name", "new-name"); err != nil {
		t.Fatalf("Rename failed: %v", err)
	}

	if pm.Exists("old-name") {
		t.Error("old name should not exist after rename")
	}
	if !pm.Exists("new-name") {
		t.Error("new name should exist after rename")
	}

	profiles, _ := pm.List()
	if len(profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(profiles))
	}
	if profiles[0].Name != "new-name" {
		t.Errorf("expected name new-name, got %s", profiles[0].Name)
	}
	if profiles[0].ID != profileID("new-name") {
		t.Errorf("expected ID %s, got %s", profileID("new-name"), profiles[0].ID)
	}
}

func TestProfileRenameRejectsPathTraversal(t *testing.T) {
	pm := NewProfileManager(t.TempDir())
	_ = pm.Create("legit")

	badNames := []string{"../evil", "evil/../other", "..\\windows"}
	for _, name := range badNames {
		t.Run("to_"+name, func(t *testing.T) {
			err := pm.Rename("legit", name)
			if err == nil {
				t.Errorf("Rename to %q should have returned error", name)
			}
		})
	}
}

func TestProfileListDoesNotCreateMetaFile(t *testing.T) {
	dir := t.TempDir()
	pm := NewProfileManager(dir)

	// Build a profile directory on disk without any profile.json meta file.
	profDir := filepath.Join(dir, profileID("no-meta"))
	if err := os.MkdirAll(filepath.Join(profDir, "Default"), 0755); err != nil {
		t.Fatal(err)
	}

	metaPath := filepath.Join(profDir, "profile.json")
	if _, err := os.Stat(metaPath); !os.IsNotExist(err) {
		t.Fatalf("precondition: profile.json should not exist, stat err=%v", err)
	}

	profiles, err := pm.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(profiles))
	}
	// The read path must still backfill ID/Name in memory.
	if profiles[0].ID == "" {
		t.Errorf("expected backfilled ID in memory, got empty")
	}
	if profiles[0].Name == "" {
		t.Errorf("expected backfilled Name in memory, got empty")
	}

	if _, err := os.Stat(metaPath); !os.IsNotExist(err) {
		t.Fatalf("List() must not create profile.json on disk, stat err=%v", err)
	}
}

func TestProfileReadPathDoesNotMutateMetaFile(t *testing.T) {
	dir := t.TempDir()
	pm := NewProfileManager(dir)

	// Profile dir with a meta file that is missing ID and Name.
	profDir := filepath.Join(dir, profileID("partial-meta"))
	if err := os.MkdirAll(filepath.Join(profDir, "Default"), 0755); err != nil {
		t.Fatal(err)
	}
	metaPath := filepath.Join(profDir, "profile.json")
	// Meta with an explicit ID (so FindByID resolves) but no Name, exercising
	// the in-memory Name backfill on the read path.
	id := profileID("partial-meta")
	original := []byte(`{"id":"` + id + `","useWhen":"keep me","description":"unchanged"}`)
	if err := os.WriteFile(metaPath, original, 0644); err != nil {
		t.Fatal(err)
	}

	// Backdate mtime so any rewrite would be detectable.
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(metaPath, past, past); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(metaPath)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := pm.List(); err != nil {
		t.Fatal(err)
	}
	if _, err := pm.FindByID(id); err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}

	after, err := os.Stat(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Errorf("read path rewrote profile.json: mtime changed from %v to %v", before.ModTime(), after.ModTime())
	}

	contents, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != string(original) {
		t.Errorf("read path mutated profile.json contents: got %q, want %q", contents, original)
	}
}

func TestProfileHandlerGetByID(t *testing.T) {
	pm := NewProfileManager(t.TempDir())
	if err := pm.CreateWithMeta("gettable", ProfileMeta{
		UseWhen:     "for testing get",
		Description: "a gettable profile",
	}); err != nil {
		t.Fatal(err)
	}
	id := profileID("gettable")

	mux := http.NewServeMux()
	pm.RegisterHandlers(mux)

	req := httptest.NewRequest("GET", "/profiles/"+id, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	bodyBytes := w.Body.Bytes()

	var resp profileResponse
	if err := json.Unmarshal(bodyBytes, &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.ID != id {
		t.Errorf("expected id %q, got %q", id, resp.ID)
	}
	if resp.Name != "gettable" {
		t.Errorf("expected name gettable, got %q", resp.Name)
	}
	if resp.Path == "" {
		t.Errorf("expected path to be present")
	}
	if !resp.PathExists {
		t.Errorf("expected pathExists=true")
	}
	if resp.UseWhen != "for testing get" {
		t.Errorf("expected useWhen 'for testing get', got %q", resp.UseWhen)
	}
	if resp.Description != "a gettable profile" {
		t.Errorf("expected description 'a gettable profile', got %q", resp.Description)
	}

	// Assert the additive fields are present in the emitted JSON.
	var raw map[string]any
	if err := json.Unmarshal(bodyBytes, &raw); err != nil {
		t.Fatalf("unmarshal raw response: %v", err)
	}
	for _, key := range []string{"lastUsed", "running"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("expected JSON field %q to be emitted, got keys %v", key, raw)
		}
	}
	if running, ok := raw["running"].(bool); !ok || running {
		t.Errorf("expected running=false, got %v", raw["running"])
	}
}

func TestProfileHandlerGetByIDNotFound(t *testing.T) {
	pm := NewProfileManager(t.TempDir())
	mux := http.NewServeMux()
	pm.RegisterHandlers(mux)

	req := httptest.NewRequest("GET", "/profiles/does-not-exist", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 404 {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestProfileRenameRejectsDuplicate(t *testing.T) {
	pm := NewProfileManager(t.TempDir())
	_ = pm.Create("profile-a")
	_ = pm.Create("profile-b")

	err := pm.Rename("profile-a", "profile-b")
	if err == nil {
		t.Error("Rename to existing name should fail")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected 'already exists' error, got: %v", err)
	}
}

// seedProfileDir writes a profile directory with metadata naming metaName,
// which is how a directory copied or renamed out from under its profile.json
// looks on disk.
func seedProfileDir(t *testing.T, baseDir, dirName, metaName string) {
	t.Helper()
	dir := filepath.Join(baseDir, dirName)
	if err := os.MkdirAll(filepath.Join(dir, "Default"), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if metaName == "" {
		return
	}
	if err := writeProfileMeta(dir, ProfileMeta{ID: profileID(metaName), Name: metaName}); err != nil {
		t.Fatalf("writeProfileMeta: %v", err)
	}
}

func listedByPath(t *testing.T, pm *ProfileManager, dirName string) bridge.ProfileInfo {
	t.Helper()
	profiles, err := pm.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, p := range profiles {
		if filepath.Base(p.Path) == dirName {
			return p
		}
	}
	t.Fatalf("directory %q missing from the listing: %+v", dirName, profiles)
	return bridge.ProfileInfo{}
}

func TestListMetadataNamingAnotherProfileFallsBackToTheDirectoryName(t *testing.T) {
	base := t.TempDir()
	pm := NewProfileManager(base)
	if err := pm.Create("live"); err != nil {
		t.Fatal(err)
	}
	seedProfileDir(t, base, "live.quarantine-1785345247", "live")

	live := listedByPath(t, pm, profileID("live"))
	stale := listedByPath(t, pm, "live.quarantine-1785345247")

	if stale.Name != "live.quarantine-1785345247" {
		t.Fatalf("quarantined entry name = %q, want its directory name", stale.Name)
	}
	if stale.ID == live.ID {
		t.Fatalf("quarantined entry shares the live profile's ID %q", stale.ID)
	}
	if stale.ID != profileID("live.quarantine-1785345247") {
		t.Fatalf("quarantined ID = %q, want the ID derived from its directory name", stale.ID)
	}
}

func TestListNeverReturnsTwoProfilesWithOneID(t *testing.T) {
	base := t.TempDir()
	pm := NewProfileManager(base)
	if err := pm.Create("live"); err != nil {
		t.Fatal(err)
	}
	for _, dirName := range []string{"live.quarantine-1", "live.quarantine-2", "copy-of-live"} {
		seedProfileDir(t, base, dirName, "live")
	}

	profiles, err := pm.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 4 {
		t.Fatalf("expected all 4 directories listed, got %d", len(profiles))
	}
	seen := map[string]string{}
	for _, p := range profiles {
		if prev, dup := seen[p.ID]; dup {
			t.Fatalf("duplicate id %q shared by %q and %q", p.ID, prev, p.Path)
		}
		seen[p.ID] = p.Path
	}
}

func TestListKeepsMetadataWhenItNamesItsOwnDirectory(t *testing.T) {
	base := t.TempDir()
	pm := NewProfileManager(base)
	if err := pm.CreateWithMeta("owned", ProfileMeta{UseWhen: "reading the metadata still works"}); err != nil {
		t.Fatal(err)
	}
	// A name-named directory is the other legitimate layout; both must keep
	// reading their metadata. UseWhen only survives when the metadata is kept,
	// so it distinguishes "trusted" from "fell back to the directory name".
	seedProfileDir(t, base, "by-name", "by-name")
	if err := writeProfileMeta(filepath.Join(base, "by-name"), ProfileMeta{
		ID:      profileID("by-name"),
		Name:    "by-name",
		UseWhen: "a name-named directory keeps its metadata too",
	}); err != nil {
		t.Fatalf("writeProfileMeta: %v", err)
	}

	idNamed := listedByPath(t, pm, profileID("owned"))
	if idNamed.Name != "owned" || idNamed.UseWhen != "reading the metadata still works" {
		t.Fatalf("id-named directory lost its metadata: name=%q useWhen=%q", idNamed.Name, idNamed.UseWhen)
	}
	nameNamed := listedByPath(t, pm, "by-name")
	if nameNamed.Name != "by-name" || nameNamed.ID != profileID("by-name") {
		t.Fatalf("name-named directory: name=%q id=%q", nameNamed.Name, nameNamed.ID)
	}
	if nameNamed.UseWhen != "a name-named directory keeps its metadata too" {
		t.Fatalf("name-named directory lost its metadata: useWhen=%q", nameNamed.UseWhen)
	}
}

func TestListWithoutMetadataStillDerivesTheNameFromTheDirectory(t *testing.T) {
	base := t.TempDir()
	pm := NewProfileManager(base)
	seedProfileDir(t, base, "bare-dir", "")

	got := listedByPath(t, pm, "bare-dir")
	if got.Name != "bare-dir" || got.ID != profileID("bare-dir") {
		t.Fatalf("no-metadata fallback changed: name=%q id=%q", got.Name, got.ID)
	}
}

func TestResolutionByNameAndIDIgnoresADirectoryClaimingAnotherProfile(t *testing.T) {
	base := t.TempDir()
	pm := NewProfileManager(base)
	if err := pm.Create("live"); err != nil {
		t.Fatal(err)
	}
	seedProfileDir(t, base, "live.quarantine-1785345247", "live")
	liveDir := filepath.Join(base, profileID("live"))

	path, err := pm.ProfilePath("live")
	if err != nil {
		t.Fatalf("resolve by name: %v", err)
	}
	if path != liveDir {
		t.Fatalf("--profile live resolved to %q, want the live directory %q", path, liveDir)
	}

	name, err := pm.FindByID(profileID("live"))
	if err != nil {
		t.Fatalf("resolve by id: %v", err)
	}
	if name != "live" {
		t.Fatalf("FindByID returned %q, want live", name)
	}
	quarantined, err := pm.FindByID(profileID("live.quarantine-1785345247"))
	if err != nil {
		t.Fatalf("resolve quarantined id: %v", err)
	}
	if quarantined != "live.quarantine-1785345247" {
		t.Fatalf("quarantined ID resolved to %q, want its own directory name", quarantined)
	}
}

// Rename writes the new name into metadata before renaming the directory. If
// the rename fails it must put the old name back, or the directory is left
// claiming a profile it is not — which is the state this fix stops trusting.
func TestProfileRenameRollsBackMetadataWhenTheDirectoryRenameFails(t *testing.T) {
	base := t.TempDir()
	pm := NewProfileManager(base)
	if err := pm.Create("old-name"); err != nil {
		t.Fatal(err)
	}
	oldDir := filepath.Join(base, profileID("old-name"))

	// The destination must look free to the preflight and still be unwritable
	// at rename time, or Rename returns before it ever touches the metadata.
	if err := os.Chmod(base, 0500); err != nil {
		t.Fatalf("chmod base: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(base, 0755) })

	if err := pm.Rename("old-name", "new-name"); err == nil {
		t.Fatal("expected the rename to fail while the destination is occupied")
	}

	meta := readProfileMeta(oldDir)
	if meta.Name != "old-name" || meta.ID != profileID("old-name") {
		t.Fatalf("metadata not rolled back: name=%q id=%q", meta.Name, meta.ID)
	}
	if !pm.Exists("old-name") {
		t.Fatal("the profile must still resolve under its old name")
	}
	got := listedByPath(t, pm, profileID("old-name"))
	if got.Name != "old-name" || got.ID != profileID("old-name") {
		t.Fatalf("listing after a failed rename: name=%q id=%q", got.Name, got.ID)
	}
}

func TestProfileManagerListFlagsQuarantinedDirectories(t *testing.T) {
	dir := t.TempDir()
	pm := NewProfileManager(dir)

	if err := pm.Create("work"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "quarantine-notes", "Default"), 0o755); err != nil {
		t.Fatal(err)
	}

	quarantineSizes := map[string]int{
		profileID("work") + ".quarantine-1785343990": 1 << 20,
		"quarantine-notes.quarantine-1785343991":     2 << 20,
	}
	for name, size := range quarantineSizes {
		defaultDir := filepath.Join(dir, name, "Default")
		if err := os.MkdirAll(defaultDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(defaultDir, "History"), make([]byte, size), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	profiles, err := pm.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 4 {
		t.Fatalf("expected 4 entries (2 live + 2 quarantined), got %d", len(profiles))
	}

	live, quarantined := 0, 0
	var quarantinedTotal int64
	for _, p := range profiles {
		if !p.Quarantined {
			live++
			continue
		}
		quarantined++
		quarantinedTotal += p.DiskUsage
		if _, ok := quarantineSizes[filepath.Base(p.Path)]; !ok {
			t.Errorf("flagged %q as quarantined, but it is not a quarantine directory", p.Path)
		}
	}
	if live != 2 || quarantined != 2 {
		t.Fatalf("expected 2 live and 2 quarantined, got %d and %d", live, quarantined)
	}
	if quarantinedTotal < 3<<20 {
		t.Errorf("quarantined disk usage totals %d bytes, want at least %d", quarantinedTotal, 3<<20)
	}
	for _, p := range profiles {
		if p.Name == "quarantine-notes" && p.Quarantined {
			t.Errorf("a profile merely named %q must not be flagged as quarantined", p.Name)
		}
	}
}

func TestHandleListMarksQuarantinedEntries(t *testing.T) {
	dir := t.TempDir()
	pm := NewProfileManager(dir)

	if err := pm.Create("quarantine-notes"); err != nil {
		t.Fatal(err)
	}
	quarantineDir := filepath.Join(dir, profileID("quarantine-notes")+".quarantine-1785343990", "Default")
	if err := os.MkdirAll(quarantineDir, 0o755); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	pm.RegisterHandlers(mux)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/profiles", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("GET /profiles = %d", w.Code)
	}
	var listed []struct {
		Name        string `json:"name"`
		Quarantined bool   `json:"quarantined"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode /profiles: %v (%s)", err, w.Body.String())
	}
	if len(listed) != 2 {
		t.Fatalf("expected 2 entries, got %d: %s", len(listed), w.Body.String())
	}
	for _, entry := range listed {
		want := strings.HasSuffix(entry.Name, ".quarantine-1785343990")
		if entry.Quarantined != want {
			t.Errorf("entry %q quarantined = %v, want %v", entry.Name, entry.Quarantined, want)
		}
	}
}

func newHeldProfile(t *testing.T, pm *ProfileManager, name string) (id, cookies string) {
	t.Helper()
	if err := pm.Create(name); err != nil {
		t.Fatal(err)
	}
	cookies = filepath.Join(pm.baseDir, profileID(name), "Default", "Cookies")
	if err := os.WriteFile(cookies, []byte("session-data"), 0600); err != nil {
		t.Fatal(err)
	}
	return profileID(name), cookies
}

func requireCookiesIntact(t *testing.T, cookies string) {
	t.Helper()
	data, err := os.ReadFile(cookies)
	if err != nil {
		t.Fatalf("cookies file gone after a refused operation: %v", err)
	}
	if string(data) != "session-data" {
		t.Fatalf("cookies content changed after a refused operation: %q", data)
	}
}

// instanceLookupFrom derives the holder mapping the same way server.go composes
// it from orch.List(), so running-ness in these tests and the guard share one
// source instead of a per-test fixture.
func instanceLookupFrom(instances []bridge.Instance) func(string) (string, bool) {
	return func(profileID string) (string, bool) {
		for _, inst := range instances {
			if inst.ProfileID != profileID {
				continue
			}
			switch inst.Status {
			case "starting", "running", "stopping":
				return inst.ID, true
			}
		}
		return "", false
	}
}

func TestDeleteRefusesAProfileAnInstanceHolds(t *testing.T) {
	pm := NewProfileManager(t.TempDir())
	id, cookies := newHeldProfile(t, pm, "held")
	pm.SetInstanceLookup(instanceLookupFrom([]bridge.Instance{
		{ID: "inst_holder01", ProfileID: id, Status: "running"},
	}))
	mux := http.NewServeMux()
	pm.RegisterHandlers(mux)

	req := httptest.NewRequest("DELETE", "/profiles/"+id, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 409 {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "inst_holder01") {
		t.Fatalf("409 body does not name the holding instance: %s", w.Body.String())
	}
	requireCookiesIntact(t, cookies)
}

func TestResetRefusesAProfileAnInstanceHolds(t *testing.T) {
	pm := NewProfileManager(t.TempDir())
	id, cookies := newHeldProfile(t, pm, "held")
	pm.SetInstanceLookup(instanceLookupFrom([]bridge.Instance{
		{ID: "inst_holder02", ProfileID: id, Status: "running"},
	}))
	mux := http.NewServeMux()
	pm.RegisterHandlers(mux)

	req := httptest.NewRequest("POST", "/profiles/"+id+"/reset", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 409 {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "inst_holder02") {
		t.Fatalf("409 body does not name the holding instance: %s", w.Body.String())
	}
	requireCookiesIntact(t, cookies)
}

// The force contract is delete-and-report-orphaned: profiles cannot stop
// instances, so the holder is named in the response instead of being left
// running on a removed directory silently.
func TestForceDeleteRemovesAHeldProfileAndReportsTheOrphanedInstance(t *testing.T) {
	pm := NewProfileManager(t.TempDir())
	id, _ := newHeldProfile(t, pm, "held")
	pm.SetInstanceLookup(instanceLookupFrom([]bridge.Instance{
		{ID: "inst_holder03", ProfileID: id, Status: "running"},
	}))
	mux := http.NewServeMux()
	pm.RegisterHandlers(mux)

	req := httptest.NewRequest("DELETE", "/profiles/"+id+"?force=true", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["orphanedInstance"] != "inst_holder03" {
		t.Fatalf("force delete did not report the orphaned instance: %v", resp)
	}
	if _, err := os.Stat(filepath.Join(pm.baseDir, id)); !os.IsNotExist(err) {
		t.Fatalf("profile directory still present after force delete: %v", err)
	}
}

func TestDeleteAndResetOfIdleProfilesAreUnchanged(t *testing.T) {
	pm := NewProfileManager(t.TempDir())
	idleID, idleCookies := newHeldProfile(t, pm, "idle")
	heldID, _ := newHeldProfile(t, pm, "held")
	pm.SetInstanceLookup(instanceLookupFrom([]bridge.Instance{
		{ID: "inst_holder04", ProfileID: heldID, Status: "running"},
	}))
	mux := http.NewServeMux()
	pm.RegisterHandlers(mux)

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("POST", "/profiles/"+idleID+"/reset", nil))
	if w.Code != 200 {
		t.Fatalf("idle reset: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(idleCookies); !os.IsNotExist(err) {
		t.Fatalf("idle reset left the cookies file in place: %v", err)
	}

	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("DELETE", "/profiles/"+idleID, nil))
	if w.Code != 200 {
		t.Fatalf("idle delete: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(pm.baseDir, idleID)); !os.IsNotExist(err) {
		t.Fatalf("idle delete left the directory in place: %v", err)
	}
}

func TestListRunningComesFromTheSameSourceAsTheGuard(t *testing.T) {
	pm := NewProfileManager(t.TempDir())
	heldID, _ := newHeldProfile(t, pm, "held")
	if err := pm.Create("idle"); err != nil {
		t.Fatal(err)
	}
	instances := []bridge.Instance{{ID: "inst_holder05", ProfileID: heldID, Status: "running"}}
	pm.SetInstanceLookup(instanceLookupFrom(instances))
	mux := http.NewServeMux()
	pm.RegisterHandlers(mux)

	req := httptest.NewRequest("GET", "/profiles", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var listed []profileResponse
	if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(listed))
	}
	for _, p := range listed {
		want := false
		for _, inst := range instances {
			if inst.ProfileID == p.ID && inst.Status == "running" {
				want = true
			}
		}
		if p.Running != want {
			t.Errorf("profile %q running = %v, want %v (from the instance mapping)", p.Name, p.Running, want)
		}
	}
}

// The bridge surface has no orchestrator, so the guard's fallback is the
// pinchtab.pid lock in the profile directory; this mux is built with no
// instance lookup at all and must still refuse.
func TestDestructiveRoutesRefuseWithoutAnOrchestrator(t *testing.T) {
	pm := NewProfileManager(t.TempDir())
	id, cookies := newHeldProfile(t, pm, "held")
	pm.lockOwner = func(string) (bool, int) { return true, 4242 }
	mux := http.NewServeMux()
	pm.RegisterHandlers(mux)

	for _, req := range []*http.Request{
		httptest.NewRequest("DELETE", "/profiles/"+id, nil),
		httptest.NewRequest("POST", "/profiles/"+id+"/reset", nil),
	} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != 409 {
			t.Fatalf("%s %s: expected 409, got %d: %s", req.Method, req.URL.Path, w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "4242") {
			t.Fatalf("%s %s: 409 body does not name the holder: %s", req.Method, req.URL.Path, w.Body.String())
		}
	}
	requireCookiesIntact(t, cookies)
}

func TestDefaultLockOwnerTreatsAPidFreeDirectoryAsIdle(t *testing.T) {
	pm := NewProfileManager(t.TempDir())
	id, _ := newHeldProfile(t, pm, "no-pid-file")
	mux := http.NewServeMux()
	pm.RegisterHandlers(mux)

	req := httptest.NewRequest("DELETE", "/profiles/"+id, nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("expected 200 for a directory with no pinchtab.pid, got %d: %s", w.Code, w.Body.String())
	}
}

// An orchestrator only knows the instances IT started. A profile held by a SECOND pinchtab
// — another server, a `pinchtab bridge`, an always-on instance outside this map — makes the
// lookup answer not-running with full confidence, so consulting it alone deletes a live
// profile on exactly the surface this card was filed against: the one where a lookup IS
// installed. The two sources are therefore ORed, and this is the direction that proves it.
func TestAProfileHeldOnlyByThePidLockIsRefusedEvenWhenTheLookupSaysIdle(t *testing.T) {
	pm := NewProfileManager(t.TempDir())
	id, cookies := newHeldProfile(t, pm, "held-by-another-pinchtab")
	pm.lockOwner = func(string) (bool, int) { return true, 4242 }
	pm.SetInstanceLookup(func(string) (string, bool) { return "", false })

	mux := http.NewServeMux()
	pm.RegisterHandlers(mux)

	for _, req := range []*http.Request{
		httptest.NewRequest("DELETE", "/profiles/"+id, nil),
		httptest.NewRequest("POST", "/profiles/"+id+"/reset", nil),
	} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != 409 {
			t.Errorf("%s %s: got %d, want 409 — an installed lookup that answers not-running must not short-circuit the per-directory lock: %s",
				req.Method, req.URL.Path, w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "4242") {
			t.Errorf("%s %s: refusal does not name the pid-lock holder: %s", req.Method, req.URL.Path, w.Body.String())
		}
	}
	requireCookiesIntact(t, cookies)

	// The flag half of this card fails in the same case, from the same line.
	list, err := pm.List()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range list {
		if p.Name != "held-by-another-pinchtab" {
			continue
		}
		found = true
		if !p.Running {
			t.Errorf("running = false for a profile the pid lock says is held; the published flag and the guard must agree, since a client greys the button out from this field")
		}
	}
	if !found {
		t.Fatal("the held profile is absent from List, so this assertion would pass vacuously")
	}
}

// The other direction, and the one the suite could not express before: the OR must not
// refuse forever on a stale file. Nothing in the tree ever REMOVES pinchtab.pid — there is
// no release path — so the file outlives its process and only the liveness and is-pinchtab
// checks stop a dead pid reading as held. This is what stops a later reader simplifying the
// OR back out on the grounds that it can only over-refuse.
func TestAStaleLockWithNoLiveHolderStillDeletes(t *testing.T) {
	pm := NewProfileManager(t.TempDir())
	id, _ := newHeldProfile(t, pm, "stale-lock")
	// What the real check answers for a pid that is dead, or alive but not pinchtab: the
	// file is present and readable, and ownership is still refused.
	pm.lockOwner = func(string) (bool, int) { return false, 999999 }
	pm.SetInstanceLookup(func(string) (string, bool) { return "", false })

	mux := http.NewServeMux()
	pm.RegisterHandlers(mux)

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("DELETE", "/profiles/"+id, nil))
	if w.Code != 200 {
		t.Fatalf("got %d, want 200 — a lock whose holder is gone must not refuse forever; there is no release path, so the file always outlives the process: %s",
			w.Code, w.Body.String())
	}
}

// A freshly created profile must look like a real Chromium user-data-dir on
// disk. External profile-volume janitors delete "no Preferences" directories
// as corrupt; on 2026-08-17 that destroyed two live bank profiles.
func TestCreateSeedsChromiumPreferences(t *testing.T) {
	dir := t.TempDir()
	pm := NewProfileManager(dir)

	if err := pm.Create("stdbank-luke"); err != nil {
		t.Fatal(err)
	}

	prefs := filepath.Join(dir, profileID("stdbank-luke"), "Default", "Preferences")
	data, err := os.ReadFile(prefs)
	if err != nil {
		t.Fatalf("Preferences not seeded at %s: %v", prefs, err)
	}
	if len(data) == 0 {
		t.Fatal("seeded Preferences must not be empty")
	}
}

// Seeding must never clobber a real Chromium Preferences file (import path).
func TestSeedChromiumPreferencesPreservesExisting(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "Default"), 0755); err != nil {
		t.Fatal(err)
	}
	prefs := filepath.Join(dir, "Default", "Preferences")
	if err := os.WriteFile(prefs, []byte(`{"real":"prefs"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := seedChromiumPreferences(dir); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(prefs)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"real":"prefs"}` {
		t.Fatalf("existing Preferences was overwritten: %s", data)
	}
}
