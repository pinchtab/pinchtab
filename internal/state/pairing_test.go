package state

import (
	"os"
	"path/filepath"
	"testing"
)

func statePath(stateDir, name string) string {
	return filepath.Join(SessionsDir(stateDir), name)
}

func exists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("stat %s: %v", path, err)
	}
	return err == nil
}

func saveNamed(t *testing.T, dir, name, key string) {
	t.Helper()
	sf := &StateFile{
		Name:    name,
		Origins: []string{"https://example.com"},
		Cookies: []Cookie{{Name: "session", Value: "secret-cookie", Domain: "example.com"}},
	}
	if _, err := Save(dir, sf, key); err != nil {
		t.Fatalf("Save(%q, key=%t): %v", name, key != "", err)
	}
}

// A state name has two possible spellings on disk and Save picks one from
// whether a key was supplied. Saving the same name again with encryption turned
// on left the earlier file in place — and that file is a plaintext copy of the
// same cookies, sitting beside the encrypted one the caller was told about.
func TestSavingEncryptedRemovesThePlaintextCopyOfTheSameName(t *testing.T) {
	dir := t.TempDir()
	saveNamed(t, dir, "github", "")
	if !exists(t, statePath(dir, "github.json")) {
		t.Fatal("the plaintext save did not land")
	}

	saveNamed(t, dir, "github", "passphrase")

	if exists(t, statePath(dir, "github.json")) {
		t.Error("the plaintext copy survived an encrypted save of the same name; its cookies are still readable on disk")
	}
	if !exists(t, statePath(dir, "github.json.enc")) {
		t.Error("the encrypted save did not land")
	}
}

// The other direction matters too: a name saved without a key must not keep
// answering from a stale encrypted file, which is the one ResolvePath prefers.
func TestSavingPlaintextRemovesTheEncryptedCopyOfTheSameName(t *testing.T) {
	dir := t.TempDir()
	saveNamed(t, dir, "github", "passphrase")
	saveNamed(t, dir, "github", "")

	if exists(t, statePath(dir, "github.json.enc")) {
		t.Error("the encrypted copy survived a plaintext save; ResolvePath prefers it, so loads would answer from the stale file")
	}
	resolved := ResolvePath(dir, "github")
	if filepath.Base(resolved) != "github.json" {
		t.Errorf("ResolvePath returned %q, want the file just written", resolved)
	}
}

// One name, one delete. Stopping at the first successful removal reported the
// state deleted while leaving the other spelling on disk.
func TestDeleteRemovesBothSpellingsOfAName(t *testing.T) {
	dir := t.TempDir()
	sessions, err := EnsureSessionsDir(dir)
	if err != nil {
		t.Fatalf("EnsureSessionsDir: %v", err)
	}
	// Both on disk at once is what an older build left behind, so deletion has to
	// cope with it rather than only with what today's Save produces.
	for _, name := range []string{"github.json", "github.json.enc"} {
		if err := os.WriteFile(filepath.Join(sessions, name), []byte("{}"), 0600); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}

	if err := Delete(dir, "github"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	for _, name := range []string{"github.json", "github.json.enc"} {
		if exists(t, statePath(dir, name)) {
			t.Errorf("%s survived the delete; the state was reported deleted while a copy remained", name)
		}
	}
}

// Deleting a name that was never saved must still say so.
func TestDeleteReportsAMissingName(t *testing.T) {
	dir := t.TempDir()
	if _, err := EnsureSessionsDir(dir); err != nil {
		t.Fatalf("EnsureSessionsDir: %v", err)
	}
	if err := Delete(dir, "never-saved"); err == nil {
		t.Fatal("Delete reported success for a name that does not exist")
	}
}
