package httpx

import (
	"fmt"
	"net/http"
)

// DisabledEndpointMessage returns a consistent message for locked endpoint families.
func DisabledEndpointMessage(feature, setting string) string {
	return fmt.Sprintf("%s endpoint is disabled; enable %s in config to use this endpoint", feature, setting)
}

// DisabledEndpointHandler returns a handler that reports a feature gate lock.
func DisabledEndpointHandler(feature, setting, code string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		ErrorCode(w, http.StatusForbidden, code, DisabledEndpointMessage(feature, setting), false, map[string]any{
			"setting": setting,
		})
	}
}
