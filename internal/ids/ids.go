package ids

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"
)

func ProfileID(name string) string {
	return hashID("prof", name)
}

func InstanceID(profileID, instanceName string) string {
	return hashID("inst", fmt.Sprintf("%s:%s:%d", profileID, instanceName, time.Now().UnixNano()))
}

func hashID(prefix, data string) string {
	hash := sha256.Sum256([]byte(data))
	return prefix + "_" + hex.EncodeToString(hash[:])[:8]
}
