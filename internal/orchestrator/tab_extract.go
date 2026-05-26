package orchestrator

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// maxBodyPeek caps how much of an inbound JSON request body the orchestrator
// will buffer to look for an explicit "tabId" field. Bodies larger than this
// (or with no Content-Length, or non-JSON content type) are passed through
// without inspection.
const maxBodyPeek = 64 * 1024

// TabIDSource identifies where the explicit tab id was found.
type TabIDSource string

const (
	TabIDSourceNone  TabIDSource = ""
	TabIDSourcePath  TabIDSource = "path"
	TabIDSourceQuery TabIDSource = "query"
	TabIDSourceBody  TabIDSource = "body"
)

// ExtractExplicitTabID inspects the request for a caller-supplied tab id in
// (in order) the routed path value `id`, the `tabId` query parameter, and
// finally a JSON body containing a `tabId` field. The first non-empty value
// wins.
//
// Body inspection is restricted to requests whose Content-Type begins with
// `application/json` and whose declared Content-Length is positive and at
// most maxBodyPeek bytes. Streaming, multipart, oversized, or
// unknown-length bodies are skipped — the body is left untouched. After a
// successful peek, the body is rewound so downstream handlers see it intact.
func ExtractExplicitTabID(r *http.Request) (string, TabIDSource) {
	if r == nil {
		return "", TabIDSourceNone
	}
	if id := strings.TrimSpace(r.PathValue("id")); id != "" {
		return id, TabIDSourcePath
	}
	if id := strings.TrimSpace(r.URL.Query().Get("tabId")); id != "" {
		return id, TabIDSourceQuery
	}
	if id := peekBodyTabID(r); id != "" {
		return id, TabIDSourceBody
	}
	return "", TabIDSourceNone
}

func ExtractRequestedBrowser(r *http.Request) string {
	if r == nil {
		return ""
	}
	return strings.TrimSpace(r.URL.Query().Get("browser"))
}

func peekBodyTabID(r *http.Request) string {
	return peekBodyStringField(r, "tabId")
}

func peekBodyStringField(r *http.Request, field string) string {
	if r == nil || r.Body == nil || r.Body == http.NoBody {
		return ""
	}
	if r.ContentLength <= 0 || r.ContentLength > maxBodyPeek {
		return ""
	}
	ct := r.Header.Get("Content-Type")
	if ct == "" {
		return ""
	}
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = ct[:i]
	}
	if !strings.EqualFold(strings.TrimSpace(ct), "application/json") {
		return ""
	}

	buf, err := io.ReadAll(io.LimitReader(r.Body, maxBodyPeek+1))
	if err != nil {
		// Body may already be partially consumed; do not attempt to repair.
		return ""
	}
	// Always restore the body so downstream handlers see the full payload.
	r.Body = io.NopCloser(bytes.NewReader(buf))
	if len(buf) > maxBodyPeek {
		return ""
	}

	var probe map[string]any
	if err := json.Unmarshal(buf, &probe); err != nil {
		return ""
	}
	raw, ok := probe[field]
	if !ok {
		return ""
	}
	value, ok := raw.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}
