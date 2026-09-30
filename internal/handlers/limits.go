package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/pinchtab/pinchtab/internal/httpx"
)

const maxBodySize = httpx.DefaultMaxJSONBodyBytes

func decodeRequiredJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(dst)
	if err == nil {
		return true
	}
	writeJSONDecodeError(w, err)
	return false
}

func decodeOptionalJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodySize)).Decode(dst)
	if err == nil || errors.Is(err, io.EOF) {
		return true
	}
	writeJSONDecodeError(w, err)
	return false
}

func writeJSONDecodeError(w http.ResponseWriter, err error) {
	httpx.Error(w, httpx.StatusForJSONDecodeError(err), fmt.Errorf("decode: %w", err))
}
