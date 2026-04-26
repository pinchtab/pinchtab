package profiles

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/pinchtab/pinchtab/internal/ids"
)

var idMgr = ids.NewManager()

var reservedWindowsProfileNames = map[string]struct{}{
	"CON":  {},
	"PRN":  {},
	"AUX":  {},
	"NUL":  {},
	"COM1": {},
	"COM2": {},
	"COM3": {},
	"COM4": {},
	"COM5": {},
	"COM6": {},
	"COM7": {},
	"COM8": {},
	"COM9": {},
	"LPT1": {},
	"LPT2": {},
	"LPT3": {},
	"LPT4": {},
	"LPT5": {},
	"LPT6": {},
	"LPT7": {},
	"LPT8": {},
	"LPT9": {},
}

func profileID(name string) string {
	return idMgr.ProfileID(name)
}

// ValidateProfileName enforces a cross-platform-safe profile name policy for
// filesystem usage and shell-adjacent process cleanup on Windows.
func ValidateProfileName(name string) error {
	if name == "" {
		return fmt.Errorf("profile name cannot be empty")
	}
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return fmt.Errorf("profile name cannot be blank")
	}
	if trimmed != name {
		return fmt.Errorf("profile name cannot start or end with whitespace")
	}
	if strings.Contains(name, "..") {
		return fmt.Errorf("profile name cannot contain '..'")
	}
	if strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("profile name cannot contain '/' or '\\'")
	}
	if strings.HasSuffix(name, ".") {
		return fmt.Errorf("profile name cannot end with '.'")
	}
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' || r == '-' || r == '_' || r == '.' {
			continue
		}
		return fmt.Errorf("profile name contains invalid character %q", r)
	}
	base := name
	if dot := strings.IndexRune(base, '.'); dot >= 0 {
		base = base[:dot]
	}
	if _, reserved := reservedWindowsProfileNames[strings.ToUpper(base)]; reserved {
		return fmt.Errorf("profile name cannot use reserved device name %q", base)
	}
	return nil
}

func isProfileNameValidationError(err error) bool {
	return err != nil && strings.HasPrefix(err.Error(), "profile name ")
}
