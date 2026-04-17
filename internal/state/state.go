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

// fileExtension returns the appropriate file extension based on whether the file
// is encrypted. Encrypted files use .json.enc; plaintext files use .json.
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
		sf.Encrypted = true
	}

	ext := fileExtension(encryptionKey != "")
	filename := sanitizeFilename(sf.Name) + ext
	path := filepath.Clean(filepath.Join(dir, filename))
	if !strings.HasPrefix(path, filepath.Clean(dir)+string(os.PathSeparator)) {
		return "", fmt.Errorf("invalid state file name: resolved path escapes state directory")
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return "", fmt.Errorf("write state file: %w", err)
	}

	return path, nil
}

// Load reads a StateFile from disk. If encryptionKey is non-empty, the file
// is decrypted before parsing.
func Load(path, encryptionKey string) (*StateFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read state file: %w", err)
	}

	if encryptionKey != "" {
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

// isStateFile reports whether a directory entry is a recognised state file
// (either .json or .json.enc).
func isStateFile(name string) bool {
	return strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".json.enc")
}

// trimStateExt removes the state file extension (.json or .json.enc) from a filename.
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

	var result []StateEntry
	for _, entry := range entries {
		if entry.IsDir() || !isStateFile(entry.Name()) {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		se := StateEntry{
			Name:      trimStateExt(entry.Name()),
			SizeBytes: info.Size(),
		}

		// Try to read metadata without full decryption
		data, readErr := os.ReadFile(path)
		if readErr == nil {
			var probe struct {
				SavedAt   time.Time `json:"savedAt"`
				Origins   []string  `json:"origins"`
				Encrypted bool      `json:"encrypted"`
			}
			if json.Unmarshal(data, &probe) == nil {
				se.SavedAt = probe.SavedAt
				se.Origins = probe.Origins
				se.Encrypted = probe.Encrypted
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

// Delete removes a named state file. Tries .json.enc first, falls back to .json.
func Delete(stateDir, name string) error {
	dir := SessionsDir(stateDir)
	base := sanitizeFilename(name)
	cleanDir := filepath.Clean(dir) + string(os.PathSeparator)
	// Try encrypted extension first.
	for _, ext := range []string{".json.enc", ".json"} {
		path := filepath.Clean(filepath.Join(dir, base+ext))
		if !strings.HasPrefix(path, cleanDir) {
			return fmt.Errorf("resolved path escapes sessions dir")
		}
		if err := os.Remove(path); err == nil {
			return nil
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("delete state file: %w", err)
		}
	}
	return fmt.Errorf("state file %q not found", name)
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

	// Try PBKDF2-based decryption (new format: salt || nonce || ciphertext).
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

	// Fall back to legacy SHA-256 key derivation.
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

	// Drop any directory components.
	name = filepath.Base(name)

	// Remove leading dot-segments.
	for strings.HasPrefix(name, "../") {
		name = strings.TrimPrefix(name, "../")
	}
	name = strings.TrimPrefix(name, "./")

	// Replace disallowed characters.
	name = strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			return '_'
		default:
			return r
		}
	}, name)

	// Final safety.
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
	// Try encrypted extension first, then plaintext.
	for _, ext := range []string{".json.enc", ".json"} {
		resolved := filepath.Clean(filepath.Join(dir, base+ext))
		cleanDir := filepath.Clean(dir) + string(os.PathSeparator)
		if !strings.HasPrefix(resolved, cleanDir) {
			return ""
		}
		if _, err := os.Stat(resolved); err == nil {
			return resolved
		}
	}
	// File doesn't exist yet — return the .json path (for new saves).
	resolved := filepath.Clean(filepath.Join(dir, base+".json"))
	cleanDir := filepath.Clean(dir) + string(os.PathSeparator)
	if !strings.HasPrefix(resolved, cleanDir) {
		return ""
	}
	return resolved
}
