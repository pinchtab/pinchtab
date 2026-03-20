package ids

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

// Manager generates stable hash-based IDs for profiles and instances.
// Runtime tab IDs use raw CDP target IDs via TabIDFromCDPTarget.
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

// TabID generates a synthetic hash-based ID for callers that need one.
// Runtime browser tab routing uses raw CDP target IDs via TabIDFromCDPTarget.
func (m *Manager) TabID(instanceID string, tabIndex int) string {
	data := fmt.Sprintf("%s:%d", instanceID, tabIndex)
	return hashID("tab", data)
}

// TabIDFromCDPTarget returns the CDP target ID as-is.
// Raw CDP IDs are used directly — no prefixing or hashing.
func (m *Manager) TabIDFromCDPTarget(cdpTargetID string) string {
	return cdpTargetID
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
