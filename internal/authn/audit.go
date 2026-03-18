package authn

import (
	"log/slog"
	"net/http"
	"strings"
)

// AuditLog records security-sensitive actions without logging raw credentials
// or session identifiers.
func AuditLog(r *http.Request, event string, attrs ...any) {
	slog.Info("audit", append(auditAttrs(r, event), attrs...)...)
}

// AuditWarn records denied or suspicious security-relevant actions.
func AuditWarn(r *http.Request, event string, attrs ...any) {
	slog.Warn("audit", append(auditAttrs(r, event), attrs...)...)
}

func auditAttrs(r *http.Request, event string) []any {
	attrs := []any{"event", strings.TrimSpace(event)}
	if r == nil {
		return attrs
	}
	attrs = append(attrs,
		"requestId", strings.TrimSpace(r.Header.Get("X-Request-Id")),
		"method", r.Method,
		"path", r.URL.Path,
		"clientIP", ClientIP(r),
	)
	if creds := CredentialsFromRequest(r); creds.Method != MethodNone {
		attrs = append(attrs, "authMethod", string(creds.Method))
	}
	if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" {
		attrs = append(attrs, "origin", origin)
	}
	return attrs
}
