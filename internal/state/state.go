// Package state manages browser state persistence (cookies, storage, metadata)
// independently from the handler layer. State files are saved to the configured
// sessions directory with optional AES-256-GCM encryption.
package state

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	pbkdf2Iterations = 100_000
	pbkdf2SaltSize   = 16
)

// Version is the current state file format version.
const Version = 1

// StateFile represents a saved browser session state.
type StateFile struct {
	Version   int                      `json:"version"`
	Name      string                   `json:"name"`
	SavedAt   time.Time                `json:"savedAt"`
	Origins   []string                 `json:"origins"`
	Cookies   []Cookie                 `json:"cookies"`
	Storage   map[string]OriginStorage `json:"storage"`
	Metadata  map[string]interface{}   `json:"metadata"`
	Encrypted bool                     `json:"encrypted"`
}

// OriginStorage holds localStorage and sessionStorage key-value pairs
// for a single origin.
type OriginStorage struct {
	Local   map[string]string `json:"local"`
	Session map[string]string `json:"session"`
}

// Cookie mirrors the essential fields from a CDP network cookie.
type Cookie struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain"`
	Path     string  `json:"path"`
	Secure   bool    `json:"secure"`
	HTTPOnly bool    `json:"httpOnly"`
	SameSite string  `json:"sameSite"`
	Expires  float64 `json:"expires,omitempty"`
}

// StateEntry is a summary of a saved state file (for listing).
type StateEntry struct {
	Name      string    `json:"name"`
	SavedAt   time.Time `json:"savedAt"`
	Origins   []string  `json:"origins"`
	Encrypted bool      `json:"encrypted"`
	SizeBytes int64     `json:"sizeBytes"`
}

// SessionsDir returns the resolved sessions directory path.
func SessionsDir(stateDir string) string {
	return filepath.Join(stateDir, "sessions")
}

// EnsureSessionsDir creates the sessions directory if it does not exist.
func EnsureSessionsDir(stateDir string) (string, error) {
	dir := SessionsDir(stateDir)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("create sessions dir: %w", err)
	}
	return dir, nil
}

func fileExtension(encrypted bool) string {
	if encrypted {
		return ".json.enc"
	}
	return ".json"
}

// Save writes a StateFile to disk. If encryptionKey is non-empty, the payload
// is encrypted with AES-256-GCM before writing. Encrypted files use the
// .json.enc extension; plaintext files use .json.
func Save(stateDir string, sf *StateFile, encryptionKey string) (string, error) {
	dir, err := EnsureSessionsDir(stateDir)
	if err != nil {
		return "", err
	}

	sf.Version = Version
	if sf.Name == "" {
		sf.Name = fmt.Sprintf("state-%d", time.Now().Unix())
	}

	// Set Encrypted before marshaling so the flag is actually persisted into
	// the (then-encrypted) payload; otherwise Load+decrypt always reports false.
	sf.Encrypted = encryptionKey != ""

	data, err := json.MarshalIndent(sf, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal state: %w", err)
	}

	if encryptionKey != "" {
		encrypted, encErr := Encrypt(data, encryptionKey)
		if encErr != nil {
			return "", fmt.Errorf("encrypt state: %w", encErr)
		}
		data = encrypted
	}

	ext := fileExtension(encryptionKey != "")
	filename := sanitizeFilename(sf.Name) + ext
	path := filepath.Clean(filepath.Join(dir, filename))
	if !strings.HasPrefix(path, filepath.Clean(dir)+string(os.PathSeparator)) {
		return "", fmt.Errorf("invalid state file name: resolved path escapes state directory")
	}

	// Atomic write: Save overwrites by name, so a torn write would destroy the
	// previously saved state as well as the new one — and a truncated encrypted
	// payload fails GCM authentication, losing the session entirely.
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return "", fmt.Errorf("write state file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("write state file: %w", err)
	}

	// One name is one file. Save picks its extension from whether a key was
	// supplied, so saving a name again with the encryption flag flipped used to
	// leave the earlier file beside the new one — and the earlier one is the
	// plaintext copy of the same cookies whenever the flip was towards
	// encryption. Removing the sibling is the difference between "this state is
	// encrypted now" and "there is also an unencrypted copy of it on disk".
	if err := removeSibling(dir, sanitizeFilename(sf.Name), ext); err != nil {
		return path, err
	}

	return path, nil
}

// removeSibling deletes the same state name under the extension Save did not
// write. A failure is returned rather than logged: the caller has just been told
// the state is encrypted, and the whole point is that the other copy is gone.
func removeSibling(dir, base, writtenExt string) error {
	for _, ext := range stateExtensions {
		if ext == writtenExt {
			continue
		}
		path, ok := statePathWithin(dir, base, ext)
		if !ok {
			continue
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove previous %s copy of state %q: %w", ext, base, err)
		}
	}
	return nil
}

// stateExtensions are the two spellings one state name can have on disk,
// encrypted first because that is the one ResolvePath prefers.
var stateExtensions = []string{".json.enc", ".json"}

// statePathWithin joins a state file path and reports whether it stayed inside
// dir, so every caller applies the same containment rule.
func statePathWithin(dir, base, ext string) (string, bool) {
	path := filepath.Clean(filepath.Join(dir, base+ext))
	return path, strings.HasPrefix(path, filepath.Clean(dir)+string(os.PathSeparator))
}

// Load reads a StateFile from disk. If encryptionKey is non-empty, the file
// is decrypted before parsing.
func Load(path, encryptionKey string) (*StateFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read state file: %w", err)
	}

	// The extension records what Save actually did, so trust it over the
	// caller's key: a plaintext file must still load when a key happens to be
	// configured. json.Valid also catches encrypted payloads written by older
	// versions that did not use the .enc extension.
	if strings.HasSuffix(path, fileExtension(true)) || !json.Valid(data) {
		if encryptionKey == "" {
			return nil, fmt.Errorf("state file is encrypted: an encryption key is required")
		}
		decrypted, decErr := Decrypt(data, encryptionKey)
		if decErr != nil {
			return nil, fmt.Errorf("decrypt state file: %w", decErr)
		}
		data = decrypted
	}

	var sf StateFile
	if err := json.Unmarshal(data, &sf); err != nil {
		return nil, fmt.Errorf("parse state file: %w", err)
	}

	return &sf, nil
}

func isStateFile(name string) bool {
	return strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".json.enc")
}

func trimStateExt(name string) string {
	if strings.HasSuffix(name, ".json.enc") {
		return strings.TrimSuffix(name, ".json.enc")
	}
	return strings.TrimSuffix(name, ".json")
}

// List returns summaries of all saved state files in the sessions directory.
func List(stateDir string) ([]StateEntry, error) {
	dir := SessionsDir(stateDir)

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []StateEntry{}, nil
		}
		return nil, fmt.Errorf("read sessions dir: %w", err)
	}

	result := make([]StateEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !isStateFile(entry.Name()) {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		encrypted := strings.HasSuffix(entry.Name(), ".json.enc")
		se := StateEntry{
			Name:      trimStateExt(entry.Name()),
			SizeBytes: info.Size(),
			Encrypted: encrypted,
		}

		// Only plaintext files expose JSON metadata; encrypted (.json.enc)
		// payloads are ciphertext, so probing them with json.Unmarshal always
		// fails and would silently drop SavedAt/Origins. List has no decryption
		// key, so encrypted entries legitimately list with empty SavedAt/Origins
		// but a correct Encrypted flag (derived from the extension above).
		if !encrypted {
			data, readErr := os.ReadFile(path)
			if readErr == nil {
				var probe struct {
					SavedAt time.Time `json:"savedAt"`
					Origins []string  `json:"origins"`
				}
				if json.Unmarshal(data, &probe) == nil {
					se.SavedAt = probe.SavedAt
					se.Origins = probe.Origins
				}
			}
		}

		result = append(result, se)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].SavedAt.After(result[j].SavedAt)
	})

	return result, nil
}

// FindByPrefix returns state entries whose name starts with the given prefix,
// sorted newest first. Used by --name prefix loading in state load.
func FindByPrefix(stateDir, prefix string) ([]StateEntry, error) {
	if prefix == "" {
		return nil, fmt.Errorf("prefix must not be empty")
	}
	all, err := List(stateDir)
	if err != nil {
		return nil, err
	}
	var matched []StateEntry
	for _, e := range all {
		if strings.HasPrefix(e.Name, prefix) {
			matched = append(matched, e)
		}
	}
	return matched, nil
}

// Delete removes a named state file under BOTH spellings. Stopping at the first
// success reported the state deleted while leaving the other copy on disk, and
// the copy left behind was the plaintext one whenever both existed.
func Delete(stateDir, name string) error {
	dir := SessionsDir(stateDir)
	base := sanitizeFilename(name)
	removed := false
	for _, ext := range stateExtensions {
		path, ok := statePathWithin(dir, base, ext)
		if !ok {
			return fmt.Errorf("resolved path escapes sessions dir")
		}
		err := os.Remove(path)
		switch {
		case err == nil:
			removed = true
		case os.IsNotExist(err):
		default:
			return fmt.Errorf("delete state file: %w", err)
		}
	}
	if !removed {
		return fmt.Errorf("state file %q not found", name)
	}
	return nil
}

// Clean removes state files older than the given duration.
// Handles both .json and .json.enc extensions.
func Clean(stateDir string, olderThan time.Duration) (int, error) {
	dir := SessionsDir(stateDir)
	cutoff := time.Now().Add(-olderThan)

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("read sessions dir: %w", err)
	}

	removed := 0
	for _, entry := range entries {
		if entry.IsDir() || !isStateFile(entry.Name()) {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoff) {
			path := filepath.Join(dir, entry.Name())
			if os.Remove(path) == nil {
				removed++
			}
		}
	}

	return removed, nil
}

// Encrypt encrypts plaintext using AES-256-GCM. The key is derived from the
// passphrase using PBKDF2 with a random salt.
// Output format: salt (16 bytes) || nonce || ciphertext.
func Encrypt(plaintext []byte, passphrase string) ([]byte, error) {
	if passphrase == "" {
		return nil, fmt.Errorf("encryption key required")
	}

	salt := make([]byte, pbkdf2SaltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("generate salt: %w", err)
	}

	key, err := deriveKey(passphrase, salt)
	if err != nil {
		return nil, fmt.Errorf("derive key: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}

	sealed := gcm.Seal(nonce, nonce, plaintext, nil)
	return append(salt, sealed...), nil
}

// Decrypt decrypts ciphertext encrypted with Encrypt.
// It tries PBKDF2-derived key first (new format), then falls back to the
// legacy SHA-256 derivation for backward compatibility with older files.
func Decrypt(ciphertext []byte, passphrase string) ([]byte, error) {
	if passphrase == "" {
		return nil, fmt.Errorf("encryption key required")
	}

	if len(ciphertext) > pbkdf2SaltSize {
		salt := ciphertext[:pbkdf2SaltSize]
		rest := ciphertext[pbkdf2SaltSize:]
		key, err := deriveKey(passphrase, salt)
		if err == nil {
			if pt, err := decryptWithKey(rest, key); err == nil {
				return pt, nil
			}
		}
	}

	legacyHash := sha256.Sum256([]byte(passphrase))
	return decryptWithKey(ciphertext, legacyHash[:])
}

func decryptWithKey(ciphertext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ct := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}

	return plaintext, nil
}

// ValidateEncryptionKey checks that the key is non-empty.
func ValidateEncryptionKey(key string) error {
	if key == "" {
		return fmt.Errorf("security.stateEncryptionKey must be set for encrypted state operations")
	}
	return nil
}

// deriveKey produces a 32-byte AES-256 key from a passphrase using PBKDF2.
func deriveKey(passphrase string, salt []byte) ([]byte, error) {
	return pbkdf2.Key(sha256.New, passphrase, salt, pbkdf2Iterations, 32)
}

// sanitizeFilename strips path separators and problematic characters.
func sanitizeFilename(name string) string {
	// Normalize Windows path separators so filepath.Base works on all OSes.
	name = strings.ReplaceAll(name, "\\", "/")

	name = filepath.Base(name)

	for strings.HasPrefix(name, "../") {
		name = strings.TrimPrefix(name, "../")
	}
	name = strings.TrimPrefix(name, "./")

	name = strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			return '_'
		default:
			return r
		}
	}, name)

	name = strings.TrimLeft(name, ".")
	if name == "" || name == "." || name == ".." {
		return "state"
	}
	return name
}

// ResolvePath returns the full canonical path for a named state file.
// Tries .json.enc first (encrypted), then .json (plaintext).
// Returns an empty string if the resolved path attempts to escape stateDir.
func ResolvePath(stateDir, name string) string {
	dir := SessionsDir(stateDir)
	base := sanitizeFilename(name)
	for _, ext := range stateExtensions {
		resolved, ok := statePathWithin(dir, base, ext)
		if !ok {
			return ""
		}
		if _, err := os.Stat(resolved); err == nil {
			return resolved
		}
	}
	// File doesn't exist yet — return the .json path (for new saves).
	resolved, ok := statePathWithin(dir, base, ".json")
	if !ok {
		return ""
	}
	return resolved
}
