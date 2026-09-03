package handlers

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

// EmptyPointerPolicy controls behavior when an identified caller omits
// `tabId` and has no stored scoped current tab. Configured server-side at
// startup; not exposed as a per-request header.
type EmptyPointerPolicy string

const (
	// EmptyPointerLazy is the default. Read-only requests return
	// 409 no_current_tab; POST /navigate creates a new tab and stores it
	// as the caller's current tab.
	EmptyPointerLazy EmptyPointerPolicy = "lazy"

	// EmptyPointerStrict treats every unscoped identified request as an
	// error, including POST /navigate. Callers must pin a tab explicitly.
	EmptyPointerStrict EmptyPointerPolicy = "strict"
)

// ErrNoCurrentTab signals that an identified caller asked an unscoped
// handler to operate on the current tab, but no current tab is stored for
// the caller's scope. Handlers map this to 409 with code `no_current_tab`.
var ErrNoCurrentTab = errors.New("no current tab")

// noCurrentTabError wraps ErrNoCurrentTab with a scope description so the
// resulting message is useful while still matching errors.Is(err, ErrNoCurrentTab).
func noCurrentTabError(scope string) error {
	if scope == "" {
		return ErrNoCurrentTab
	}
	return fmt.Errorf("%w for %s", ErrNoCurrentTab, scope)
}

// IsNoCurrentTab reports whether err signals an empty scoped current tab.
func IsNoCurrentTab(err error) bool {
	return errors.Is(err, ErrNoCurrentTab)
}

// WriteTabContextError maps tabContext errors to the right HTTP response.
// ErrNoCurrentTab → 409 with code "no_current_tab"; everything else falls
// back to notFoundStatus (or 404 when zero) for the historical
// tab-not-found behavior.
func WriteTabContextError(w http.ResponseWriter, err error, notFoundStatus int) {
	if err == nil {
		return
	}
	if IsNoCurrentTab(err) {
		httpx.ErrorCode(w, http.StatusConflict, "no_current_tab", err.Error(), false, nil)
		return
	}
	if notFoundStatus == 0 {
		notFoundStatus = http.StatusNotFound
	}
	var lost *bridge.TabNotFoundError
	if errors.As(err, &lost) && lost.Crash != nil {
		message, details := crashAnnotation(err.Error(), nil, *lost.Crash,
			"this tab died with it, along with its logins, form state and refs; open a new tab and navigate again")
		httpx.ErrorCode(w, notFoundStatus, "browser_crashed", message, false, details)
		return
	}
	httpx.Error(w, notFoundStatus, err)
}
