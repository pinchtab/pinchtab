package engine

import "strings"

// sensitiveNameHints are substrings of an element's name/id/autocomplete that
// mark it credential-bearing even when its input type is not "password".
var sensitiveNameHints = []string{
	"password", "passwd", "pwd", "passphrase",
	"secret", "token", "apikey", "api-key", "api_key",
	"otp", "totp", "mfa", "2fa", "one-time-code", "onetimecode",
	"cvv", "cvc", "cardnumber", "card-number", "card_number",
	"security-code", "securitycode",
	"private-key", "privatekey", "credential",
}

// IsSensitiveFieldAttrs decides sensitivity from element attributes.
func IsSensitiveFieldAttrs(inputType, autocomplete, name, id, ariaLabel string) bool {
	if strings.EqualFold(strings.TrimSpace(inputType), "password") {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(autocomplete)) {
	case "current-password", "new-password", "one-time-code", "cc-number", "cc-csc":
		return true
	}
	haystack := strings.ToLower(strings.Join([]string{autocomplete, name, id, ariaLabel}, " "))
	haystack = strings.ReplaceAll(haystack, " ", "")
	for _, hint := range sensitiveNameHints {
		if strings.Contains(haystack, hint) {
			return true
		}
	}
	return false
}
