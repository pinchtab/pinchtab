package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/session"
)

// HandleRecordStart starts a recording session for a tab.
func (h *Handlers) HandleRecordStart(w http.ResponseWriter, r *http.Request) {
	if !h.Config.AllowScreencast {
		httpx.ErrorCode(w, 403, "recording_disabled",
			httpx.DisabledEndpointMessage("recording", "security.allowScreencast"), false,
			map[string]any{
				"setting": "security.allowScreencast",
				"hint":    "Recording requires screen capture to be enabled.",
				"remedy":  "pinchtab config set security.allowScreencast true",
			})
		return
	}

	if !h.ensureBrowserOrRespond(w, h.Config) {
		return
	}

	var req struct {
		TabID   string  `json:"tabId"`
		Format  string  `json:"format"`
		FPS     int     `json:"fps"`
		Quality int     `json:"quality"`
		Scale   float64 `json:"scale"`
	}
	if err := httpx.DecodeJSONBody(w, r, 0, &req); err != nil {
		httpx.Error(w, httpx.StatusForJSONDecodeError(err), err)
		return
	}

	if req.Format == "" {
		req.Format = "gif"
	}
	if req.FPS <= 0 {
		req.FPS = 5
	}
	if req.FPS > maxFPS {
		req.FPS = maxFPS
	}
	if req.Quality <= 0 {
		req.Quality = 80
	}
	if req.Quality > maxQuality {
		req.Quality = maxQuality
	}
	if req.Scale <= 0 {
		req.Scale = 1.0
	}
	if req.Scale > maxScale {
		req.Scale = maxScale
	}

	switch req.Format {
	case "gif":
	case "webm", "mp4":
		if !ffmpegAvailable() {
			httpx.ErrorCode(w, 400, "ffmpeg_required",
				fmt.Sprintf("recording to .%s requires ffmpeg; install it or use .gif", req.Format),
				false, nil)
			return
		}
	default:
		httpx.ErrorCode(w, 400, "invalid_format",
			"supported formats: gif, webm, mp4", false, nil)
		return
	}

	ctx, resolvedTabID, err := h.tabContext(r, req.TabID)
	if err != nil {
		httpx.Problem(w, http.StatusNotFound, "tab_not_found", "tab not found", false, nil)
		return
	}

	owner := authenticatedOwner(r)

	if err := h.recorder.start(ctx, resolvedTabID, owner, req.Format, req.FPS, req.Quality, req.Scale); err != nil {
		httpx.ErrorCode(w, 409, "recording_error", err.Error(), false, nil)
		return
	}

	slog.Info("recording started", "tab", resolvedTabID, "format", req.Format, "fps", req.FPS)
	httpx.JSON(w, 200, map[string]any{
		"status":  "recording",
		"format":  req.Format,
		"fps":     req.FPS,
		"quality": req.Quality,
		"tabId":   resolvedTabID,
	})
}

// HandleRecordStop stops the active recording. If discard is false (default),
// encoding runs in the background into a server-controlled recordings directory
// and the endpoint returns the path immediately. If discard is true, frames are
// dropped without encoding. Use /record/status to check encoding progress.
func (h *Handlers) HandleRecordStop(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Discard bool `json:"discard"`
	}
	_ = httpx.DecodeJSONBody(w, r, 0, &req)

	var outputPath string
	if !req.Discard {
		var err error
		outputPath, err = h.recordingsOutputPath()
		if err != nil {
			httpx.ErrorCode(w, 500, "recording_error", err.Error(), false, nil)
			return
		}
	}

	owner := authenticatedOwner(r)
	result, err := h.recorder.stop(owner, outputPath)
	if err != nil {
		httpx.ErrorCode(w, 400, "recording_error", err.Error(), false, nil)
		return
	}

	if req.Discard {
		httpx.JSON(w, 200, map[string]any{
			"status": "discarded",
			"format": result.Format,
			"frames": result.Frames,
		})
		return
	}

	slog.Info("recording stopped, encoding in background",
		"format", result.Format, "frames", result.Frames, "path", result.OutputPath)
	httpx.JSON(w, 200, map[string]any{
		"status": "encoding",
		"path":   result.OutputPath,
		"format": result.Format,
		"frames": result.Frames,
		"hint":   fmt.Sprintf("Encoding %d frames to %s. Use `record status` to check progress — the file will appear at the path once encoding completes.", result.Frames, result.OutputPath),
	})
}

// recordingsOutputPath returns a unique output path inside the server-controlled
// recordings directory. The caller never chooses the path — only the server does.
func (h *Handlers) recordingsOutputPath() (string, error) {
	dir := filepath.Join(h.Config.StateDir, "recordings")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("create recordings dir: %w", err)
	}
	format := h.recorder.activeFormat()
	ext := "." + format
	name := fmt.Sprintf("rec_%s%s", time.Now().Format("20060102_150405"), ext)
	return filepath.Join(dir, name), nil
}

// HandleRecordStatus returns the current recording status.
func (h *Handlers) HandleRecordStatus(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, 200, h.recorder.status())
}

// recordingIdentity is the transport-extracted caller identity for a recording
// request, carried independently of the owner-key policy in authenticatedOwner.
type recordingIdentity struct {
	sessionID string
	agentID   string
	proxy     bool // derived from trusted-proxy headers rather than an authed session
}

// recordingOwnerIdentity extracts the caller identity from the request transport:
// the authenticated session if present, else the trusted-internal-proxy headers.
// It performs NO owner-key policy — that lives in authenticatedOwner.
func recordingOwnerIdentity(r *http.Request) (recordingIdentity, bool) {
	if sess, ok := session.FromRequest(r); ok && sess != nil {
		return recordingIdentity{
			sessionID: strings.TrimSpace(sess.ID),
			agentID:   strings.TrimSpace(sess.AgentID),
		}, true
	}
	if IsTrustedInternalProxy(r) {
		return recordingIdentity{
			sessionID: strings.TrimSpace(r.Header.Get(activity.HeaderPTSessionID)),
			agentID:   strings.TrimSpace(r.Header.Get(activity.HeaderAgentID)),
			proxy:     true,
		}, true
	}
	return recordingIdentity{}, false
}

// authenticatedOwner derives a non-secret, in-memory owner key from the request.
// Recordings are SESSION-SCOPED: the session ID wins so two browser sessions for
// the same agent get distinct owners and cannot stop each other's recordings. The
// agent ID is only a fallback when no session ID is available (e.g. a proxy that
// forwards an agent header but no session header). Returns "" for anonymous
// (unauthenticated) requests — anonymous recordings can be stopped by any caller,
// intentional for the single-user local model.
func authenticatedOwner(r *http.Request) string {
	id, ok := recordingOwnerIdentity(r)
	if !ok {
		return ""
	}
	sessionPrefix, agentPrefix := "session:", "agent:"
	if id.proxy {
		sessionPrefix, agentPrefix = "proxy-session:", "proxy-agent:"
	}
	if id.sessionID != "" {
		return sessionPrefix + id.sessionID
	}
	if id.agentID != "" {
		return agentPrefix + id.agentID
	}
	return ""
}
