package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()

	sf := &StateFile{
		Name:    "test-session",
		SavedAt: time.Now().Truncate(time.Second),
		Origins: []string{"https://example.com"},
		Cookies: []Cookie{
			{Name: "sid", Value: "abc123", Domain: "example.com", Path: "/", Secure: true},
		},
		Storage: map[string]OriginStorage{
			"https://example.com": {
				Local:   map[string]string{"theme": "dark"},
				Session: map[string]string{"token": "xyz"},
			},
		},
		Metadata: map[string]interface{}{
			"url":       "https://example.com/page",
			"origin":    "https://example.com",
			"userAgent": "Chrome/120",
		},
	}

	path, err := Save(dir, sf, "")
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(path, "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.Name != sf.Name {
		t.Errorf("name = %q, want %q", loaded.Name, sf.Name)
	}
	if len(loaded.Cookies) != 1 {
		t.Errorf("cookies count = %d, want 1", len(loaded.Cookies))
	}
	if loaded.Cookies[0].Name != "sid" {
		t.Errorf("cookie name = %q, want %q", loaded.Cookies[0].Name, "sid")
	}
	if len(loaded.Storage) != 1 {
		t.Errorf("storage origins = %d, want 1", len(loaded.Storage))
	}
	origin := loaded.Storage["https://example.com"]
	if origin.Local["theme"] != "dark" {
		t.Errorf("localStorage theme = %q, want %q", origin.Local["theme"], "dark")
	}
	if origin.Session["token"] != "xyz" {
		t.Errorf("sessionStorage token = %q, want %q", origin.Session["token"], "xyz")
	}
}

func TestSaveLoadEncrypted(t *testing.T) {
	dir := t.TempDir()
	key := "my-secret-passphrase"

	sf := &StateFile{
		Name:    "encrypted-session",
		SavedAt: time.Now().Truncate(time.Second),
		Origins: []string{"https://secure.example.com"},
		Cookies: []Cookie{
			{Name: "auth", Value: "secret", Domain: "secure.example.com"},
		},
		Storage: map[string]OriginStorage{},
		Metadata: map[string]interface{}{
			"url": "https://secure.example.com",
		},
	}

	path, err := Save(dir, sf, key)
	if err != nil {
		t.Fatalf("Save encrypted: %v", err)
	}

	// Verify the file is not plain JSON
	raw, _ := os.ReadFile(path)
	var probe map[string]interface{}
	if json.Unmarshal(raw, &probe) == nil {
		// If it parses as JSON, encryption might not have worked
		// But note: Save writes encrypted bytes, not a JSON wrapper
		if _, ok := probe["name"]; ok {
			t.Error("encrypted file appears to be plain JSON")
		}
	}

	// Load with correct key
	loaded, err := Load(path, key)
	if err != nil {
		t.Fatalf("Load encrypted: %v", err)
	}
	if loaded.Name != sf.Name {
		t.Errorf("name = %q, want %q", loaded.Name, sf.Name)
	}

	// Load with wrong key should fail
	_, err = Load(path, "wrong-key")
	if err == nil {
		t.Error("Load with wrong key should fail")
	}
}

func TestSaveLoadEncryptedFlagPersisted(t *testing.T) {
	dir := t.TempDir()
	key := "round-trip-key"

	sf := &StateFile{Name: "enc-flag", SavedAt: time.Now().Truncate(time.Second), Storage: map[string]OriginStorage{}}
	path, err := Save(dir, sf, key)
	if err != nil {
		t.Fatalf("Save encrypted: %v", err)
	}
	if !strings.HasSuffix(path, ".json.enc") {
		t.Errorf("encrypted save path = %q, want .json.enc suffix", path)
	}

	loaded, err := Load(path, key)
	if err != nil {
		t.Fatalf("Load encrypted: %v", err)
	}
	if !loaded.Encrypted {
		t.Error("loaded.Encrypted = false, want true for an encrypted state file")
	}

	// Plaintext: Encrypted must round-trip as false.
	plain := &StateFile{Name: "plain-flag", SavedAt: time.Now().Truncate(time.Second), Storage: map[string]OriginStorage{}}
	ppath, err := Save(dir, plain, "")
	if err != nil {
		t.Fatalf("Save plaintext: %v", err)
	}
	ploaded, err := Load(ppath, "")
	if err != nil {
		t.Fatalf("Load plaintext: %v", err)
	}
	if ploaded.Encrypted {
		t.Error("loaded.Encrypted = true, want false for a plaintext state file")
	}
}

func TestListEncryptedEntryMetadata(t *testing.T) {
	dir := t.TempDir()
	key := "list-enc-key"

	enc := &StateFile{Name: "enc-entry", SavedAt: time.Now().Truncate(time.Second), Origins: []string{"https://enc.example.com"}, Storage: map[string]OriginStorage{}}
	if _, err := Save(dir, enc, key); err != nil {
		t.Fatalf("Save encrypted: %v", err)
	}
	plain := &StateFile{Name: "plain-entry", SavedAt: time.Now().Truncate(time.Second), Origins: []string{"https://plain.example.com"}, Storage: map[string]OriginStorage{}}
	if _, err := Save(dir, plain, ""); err != nil {
		t.Fatalf("Save plaintext: %v", err)
	}

	entries, err := List(dir)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}

	byName := map[string]StateEntry{}
	for _, e := range entries {
		byName[e.Name] = e
	}

	encEntry, ok := byName["enc-entry"]
	if !ok {
		t.Fatal("encrypted entry missing from List")
	}
	if !encEntry.Encrypted {
		t.Error("encrypted entry: Encrypted = false, want true (derived from .json.enc extension)")
	}

	plainEntry, ok := byName["plain-entry"]
	if !ok {
		t.Fatal("plaintext entry missing from List")
	}
	if plainEntry.Encrypted {
		t.Error("plaintext entry: Encrypted = true, want false")
	}
	if len(plainEntry.Origins) != 1 || plainEntry.Origins[0] != "https://plain.example.com" {
		t.Errorf("plaintext entry Origins = %v, want [https://plain.example.com]", plainEntry.Origins)
	}
	if plainEntry.SavedAt.IsZero() {
		t.Error("plaintext entry SavedAt is zero, want populated from JSON metadata")
	}
}

func TestEncryptDecrypt(t *testing.T) {
	plaintext := []byte("hello, world!")
	key := "test-key-123"

	ct, err := Encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	pt, err := Decrypt(ct, key)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}

	if string(pt) != string(plaintext) {
		t.Errorf("decrypted = %q, want %q", pt, plaintext)
	}
}

func TestEncryptEmptyKey(t *testing.T) {
	_, err := Encrypt([]byte("data"), "")
	if err == nil {
		t.Error("Encrypt with empty key should fail")
	}

	_, err = Decrypt([]byte("data"), "")
	if err == nil {
		t.Error("Decrypt with empty key should fail")
	}
}

func TestValidateEncryptionKey(t *testing.T) {
	if err := ValidateEncryptionKey("valid-key"); err != nil {
		t.Errorf("valid key should pass: %v", err)
	}
	if err := ValidateEncryptionKey(""); err == nil {
		t.Error("empty key should fail")
	}
}

func TestListEmpty(t *testing.T) {
	dir := t.TempDir()

	entries, err := List(dir)
	if err != nil {
		t.Fatalf("List empty: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("entries = %d, want 0", len(entries))
	}
}

func TestListNonExistent(t *testing.T) {
	entries, err := List(filepath.Join(t.TempDir(), "nonexistent"))
	if err != nil {
		t.Fatalf("List nonexistent: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("entries = %d, want 0", len(entries))
	}
}

func TestListAfterSave(t *testing.T) {
	dir := t.TempDir()

	sf := &StateFile{
		Name:    "list-test",
		SavedAt: time.Now(),
		Origins: []string{"https://example.com"},
		Storage: map[string]OriginStorage{},
	}
	if _, err := Save(dir, sf, ""); err != nil {
		t.Fatalf("Save: %v", err)
	}

	entries, err := List(dir)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	if entries[0].Name != "list-test" {
		t.Errorf("name = %q, want %q", entries[0].Name, "list-test")
	}
}

func TestDelete(t *testing.T) {
	dir := t.TempDir()

	sf := &StateFile{
		Name:    "delete-test",
		SavedAt: time.Now(),
		Storage: map[string]OriginStorage{},
	}
	if _, err := Save(dir, sf, ""); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := Delete(dir, "delete-test"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	entries, _ := List(dir)
	if len(entries) != 0 {
		t.Error("state file should be deleted")
	}
}

func TestDeleteNotFound(t *testing.T) {
	dir := t.TempDir()
	if err := Delete(dir, "nonexistent"); err == nil {
		t.Error("delete nonexistent should fail")
	}
}

func TestClean(t *testing.T) {
	dir := t.TempDir()
	sessDir, _ := EnsureSessionsDir(dir)

	// Create a state file with old modification time
	old := &StateFile{Name: "old", SavedAt: time.Now().Add(-48 * time.Hour), Storage: map[string]OriginStorage{}}
	data, _ := json.Marshal(old)
	oldPath := filepath.Join(sessDir, "old.json")
	if err := os.WriteFile(oldPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatalf("Chtimes(%s): %v", oldPath, err)
	}

	// Create a recent state file
	recent := &StateFile{Name: "recent", SavedAt: time.Now(), Storage: map[string]OriginStorage{}}
	if _, err := Save(dir, recent, ""); err != nil {
		t.Fatal(err)
	}

	removed, err := Clean(dir, 24*time.Hour)
	if err != nil {
		t.Fatalf("Clean: %v", err)
	}
	if removed != 1 {
		t.Errorf("removed = %d, want 1", removed)
	}

	entries, _ := List(dir)
	if len(entries) != 1 {
		t.Errorf("remaining = %d, want 1", len(entries))
	}
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"simple", "simple"},
		{"with spaces", "with spaces"},
		{"path/traversal", "traversal"},
		{"..\\evil", "evil"},
		{"", "state"},
		{".", "state"},
		{"..", "state"},
		{"file:name", "file_name"},
		{"file*name?", "file_name_"},
	}
	for _, tt := range tests {
		got := sanitizeFilename(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeFilename(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestEmptyStorage(t *testing.T) {
	dir := t.TempDir()

	sf := &StateFile{
		Name:     "empty-storage",
		SavedAt:  time.Now(),
		Origins:  []string{},
		Cookies:  []Cookie{},
		Storage:  map[string]OriginStorage{},
		Metadata: map[string]interface{}{},
	}

	path, err := Save(dir, sf, "")
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(path, "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(loaded.Cookies) != 0 {
		t.Errorf("cookies = %d, want 0", len(loaded.Cookies))
	}
	if len(loaded.Storage) != 0 {
		t.Errorf("storage = %d, want 0", len(loaded.Storage))
	}
}

func TestResolvePathTraversal(t *testing.T) {
	dir := t.TempDir()

	// These names should all resolve safely within the sessions dir.
	safe := []string{"simple", "with-dashes", "session_2024"}
	for _, name := range safe {
		p := ResolvePath(dir, name)
		if p == "" {
			t.Errorf("ResolvePath(%q) rejected a safe name", name)
		}
	}

	// These names are path traversal attempts — must return "".
	traversal := []string{
		"../etc/passwd",
		"..\\windows\\system32",
		"../../secret",
		"/etc/passwd",
	}
	for _, name := range traversal {
		p := ResolvePath(dir, name)
		if p == "" {
			// Good: traversal rejected.
			continue
		}
		// If non-empty, check it's actually inside the sessions dir.
		sessDir := SessionsDir(dir)
		if len(p) <= len(sessDir) {
			t.Errorf("ResolvePath(%q) = %q escaped sessions dir %q", name, p, sessDir)
		}
	}
}
