package idutil

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// Manager generates stable hash-based IDs for profiles, instances, and tabs
type Manager struct{}

// NewManager creates a new ID manager
func NewManager() *Manager {
	return &Manager{}
}

// ProfileID generates a stable hash-based ID for a profile from its name
// Format: prof_XXXXXXXX (12 chars total)
func (m *Manager) ProfileID(name string) string {
	return hashID("prof", name)
}

// InstanceID generates a stable hash-based ID for an instance
// Uses profile ID, instance name, and creation timestamp for uniqueness
// Format: inst_XXXXXXXX (12 chars total)
func (m *Manager) InstanceID(profileID, instanceName string) string {
	data := fmt.Sprintf("%s:%s:%d", profileID, instanceName, time.Now().UnixNano())
	return hashID("inst", data)
}

// TabID generates a stable hash-based ID for a tab within an instance
// Uses instance ID and tab number for uniqueness
// Format: tab_XXXXXXXX (12 chars total)
func (m *Manager) TabID(instanceID string, tabIndex int) string {
	data := fmt.Sprintf("%s:%d", instanceID, tabIndex)
	return hashID("tab", data)
}

// hashID creates a short hash-based ID with the given prefix
// Format: {prefix}_{first 8 hex chars of SHA256}
func hashID(prefix, data string) string {
	hash := sha256.Sum256([]byte(data))
	hexHash := hex.EncodeToString(hash[:])
	// Take first 8 characters of hash for readability (still extremely collision-resistant)
	return fmt.Sprintf("%s_%s", prefix, hexHash[:8])
}

// IsValidID checks if an ID matches the expected prefix format
func IsValidID(id, prefix string) bool {
	if len(id) < len(prefix)+1 {
		return false
	}
	return id[:len(prefix)] == prefix && id[len(prefix)] == '_'
}

// ExtractPrefix extracts the prefix from an ID
func ExtractPrefix(id string) string {
	for i, c := range id {
		if c == '_' {
			return id[:i]
		}
	}
	return ""
}
