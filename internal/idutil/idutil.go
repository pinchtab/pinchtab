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

// TabIDFromCDPTarget converts a CDP target ID to a semantic tab ID
// Used when creating tabs dynamically via CDP
// Format: tab_<CDP_TARGET_ID> (e.g., tab_D25F4C74...)
// This is a zero-state design: the CDP ID is embedded in the tab ID,
// so no mapping table is needed across processes.
func (m *Manager) TabIDFromCDPTarget(cdpTargetID string) string {
	// Simply prefix the CDP ID - no hashing needed
	return fmt.Sprintf("tab_%s", cdpTargetID)
}

// StripTabPrefix removes the "tab_" prefix from a semantic tab ID
// Returns the raw CDP target ID
func StripTabPrefix(tabID string) string {
	const prefix = "tab_"
	if len(tabID) > len(prefix) && tabID[:len(prefix)] == prefix {
		return tabID[len(prefix):]
	}
	// Already a CDP ID (no prefix)
	return tabID
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
