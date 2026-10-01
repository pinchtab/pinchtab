package handlers

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/pinchtab/pinchtab/internal/httpx"
)

const maxBodySize = httpx.DefaultMaxJSONBodyBytes

const (
	minActionTimeout = 100 * time.Millisecond
	maxActionTimeout = 60 * time.Second
)

func parseTimeoutSeconds(name, v string) (time.Duration, error) {
	seconds, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be a number of seconds, got %q", name, v)
	}
	return timeoutFromSeconds(name, seconds)
}

func timeoutFromSeconds(name string, seconds float64) (time.Duration, error) {
	if math.IsNaN(seconds) || seconds < minActionTimeout.Seconds() || seconds > maxActionTimeout.Seconds() {
		return 0, fmt.Errorf("%s must be between %g and %g seconds, got %g", name, minActionTimeout.Seconds(), maxActionTimeout.Seconds(), seconds)
	}
	return time.Duration(seconds * float64(time.Second)), nil
}

func writeInvalidTimeout(w http.ResponseWriter, err error) {
	httpx.ErrorCode(w, http.StatusBadRequest, "invalid_timeout", err.Error(), false, map[string]any{
		"minSeconds": minActionTimeout.Seconds(),
		"maxSeconds": maxActionTimeout.Seconds(),
	})
}

func decodeRequiredJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	return decodeLenientJSON(w, r, dst, httpx.JSONBodyOptions{MaxBytes: maxBodySize, Lenient: true})
}

func decodeOptionalJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	return decodeLenientJSON(w, r, dst, httpx.JSONBodyOptions{MaxBytes: maxBodySize, Lenient: true, Optional: true})
}

func decodeLenientJSON(w http.ResponseWriter, r *http.Request, dst any, opts httpx.JSONBodyOptions) bool {
	err := httpx.DecodeJSON(w, r, dst, opts)
	if err == nil {
		return true
	}
	writeJSONDecodeError(w, err)
	return false
}

func writeJSONDecodeError(w http.ResponseWriter, err error) {
	httpx.Error(w, httpx.StatusForJSONDecodeError(err), fmt.Errorf("decode: %w", err))
}
