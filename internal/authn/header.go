package authn

import (
	"net/http"
	"strings"
)

func AuthorizationHeaderValue(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	if strings.HasPrefix(token, "ses_") {
		return "Session " + token
	}
	return "Bearer " + token
}

func SetAuthorization(h http.Header, token string) {
	if value := AuthorizationHeaderValue(token); value != "" {
		h.Set("Authorization", value)
	}
}
