package server

import (
	"net/http"

	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/remedy"
	"github.com/pinchtab/pinchtab/internal/session"
)

// Three states used to be one bare mux 404, so the CLI mapped all of them onto the only
// remedy that fits one: a config edit. On a bridge that edit can never work, and the user
// re-ran it forever. These codes exist so the caller can tell the states apart; the CLI
// branches on them rather than on the status, because keying off 404 is what conflated
// them and would conflate the next mode too.
const (
	// CodeSessionsUnavailableInBridgeMode: the family is structurally absent. No config
	// mounts it, so the remedy is a different process, not a setting.
	CodeSessionsUnavailableInBridgeMode = "sessions_unavailable_bridge_mode"
	// CodeSessionsDisabled: mounted by config and switched off, where the config remedy
	// this family always printed is the correct one.
	CodeSessionsDisabled = session.CodeDisabled
)

const (
	msgSessionsUnavailableInBridgeMode = "agent sessions are unavailable in bridge mode"
	hintSessionsUnavailableInBridge    = "no config value mounts the session family on a bridge; the full server is what serves it."

	msgSessionsDisabled  = session.MsgDisabled
	hintSessionsDisabled = session.HintDisabled
)

// Running the full server is the bridge's whole remedy. The verb it used to lead with —
// "run the full server instead: pinchtab server" — put the command behind prose.
var runFullServer = remedy.Declare("pinchtab server")

// registerSessionsUnavailable answers the whole session family with one coded refusal.
// Both modes that do not mount the family call it, and it ranges over the same
// session.RoutePatterns() the live registration uses — that shared list is what stops a
// route added later being reachable in server mode and a bare 404 everywhere else.
//
// The status stays 404: the family genuinely is not here. What changes is that the body is
// the product's error envelope with a code and guidance instead of net/http's bare
// "404 page not found", which escaped the envelope entirely because no route was
// registered at all.
func registerSessionsUnavailable(mux *http.ServeMux, code, message, hint string, r remedy.Remedy) {
	handler := func(w http.ResponseWriter, _ *http.Request) {
		httpx.ErrorCode(w, http.StatusNotFound, code, message, false, remedy.Details(hint, r))
	}
	for _, pattern := range session.RoutePatterns() {
		mux.HandleFunc(pattern, handler)
	}
}

// RegisterSessionsUnavailableInBridgeMode is the bridge's answer for the family.
func RegisterSessionsUnavailableInBridgeMode(mux *http.ServeMux) {
	registerSessionsUnavailable(mux, CodeSessionsUnavailableInBridgeMode,
		msgSessionsUnavailableInBridgeMode, hintSessionsUnavailableInBridge, runFullServer.Remedy())
}

// RegisterSessionsDisabled is the full server's answer when sessions.agent.enabled is off.
func RegisterSessionsDisabled(mux *http.ServeMux) {
	registerSessionsUnavailable(mux, CodeSessionsDisabled,
		msgSessionsDisabled, hintSessionsDisabled, remedy.None)
}
