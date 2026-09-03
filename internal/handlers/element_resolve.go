package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

var ErrElementNotFound = errors.New("element not found")

// ErrSemanticMatcherUnavailable is the one server fault in the semantic selector path:
// the feature is not configured, which is not the caller's mistake and not a missing
// element. It carries its own sentinel so the mapping below can keep answering 501 for
// it without asking what the message says.
var ErrSemanticMatcherUnavailable = errors.New("semantic selectors require a matcher (not configured)")

func (h *Handlers) resolveElementNodeID(ctx context.Context, tabID, sel string) (int64, error) {
	nodeID, err := h.resolveSelectorNodeID(ctx, tabID, sel)
	if err != nil {
		// Only a genuine "selector matched no element" is a 404. CDP/transport
		// faults, unsupported selector kinds, and internal routing errors must
		// stay 5xx so real bridge failures don't masquerade as a missing element.
		if errors.Is(err, bridge.ErrSelectorNoMatch) {
			return 0, fmt.Errorf("%w: %q: %v", ErrElementNotFound, sel, err)
		}
		return 0, err
	}
	if nodeID == 0 {
		return 0, fmt.Errorf("%w: %q", ErrElementNotFound, sel)
	}
	return nodeID, nil
}

// selectorFailureStatus is the ONE owner of "what status does a failure to resolve a
// selector get". It is named for the question rather than for one of its callers,
// because a second mapper used to exist beside it: the two read DIFFERENT sentinels
// at different wrapping depths — the wrapped ErrElementNotFound here, the raw
// bridge.ErrSelectorNoMatch there — and agreed on 404 by coincidence rather than by
// construction, while the inspect endpoints the old comment claimed to cover
// answered 500 and the capture endpoints answered 400.
//
// It reads SENTINELS, never message text: a status keyed on wording changes the
// moment someone rewrites a sentence, and the semantic messages are rewritten often.
// Both no-match sentinels are handled here so a call site never has to choose which
// mapper to consult.
//
// A selector that matched nothing is the caller's problem (404: re-snapshot and pick
// another) — the request was well formed and the page simply lacks the element, which
// is why it is neither a 400 nor a 500. Everything else is the server's (500:
// retrying may help). Getting that backwards on a read endpoint sends retry storms at
// a page that is fine, and files caller-side probes as server faults in the failure
// telemetry an operator watches.
func selectorFailureStatus(err error) int {
	switch {
	case err == nil:
		return http.StatusOK
	case errors.Is(err, ErrSemanticMatcherUnavailable):
		return http.StatusNotImplemented
	case errors.Is(err, ErrElementNotFound), errors.Is(err, bridge.ErrSelectorNoMatch):
		return http.StatusNotFound
	case errors.Is(err, bridge.ErrSelectorOutsideScope):
		return http.StatusBadRequest
	case errors.Is(err, context.DeadlineExceeded):
		return http.StatusGatewayTimeout
	default:
		return http.StatusInternalServerError
	}
}

// CodeElementNotFound is the machine-readable name for the one condition this card
// is about, so an agent can branch on it instead of matching prose. Every surface
// that resolves a selector answers with it, rather than the generic "error" code
// each of them used to produce with a different status beside it.
const CodeElementNotFound = "element_not_found"

// respondSelectorFailure is the ONE way a selector failure leaves a handler: the
// status from the owner above, and a named code for the miss so the class is
// readable without parsing the message. Handlers that must return rather than write
// (the screenshot clip path) call selectorFailureStatus directly.
func respondSelectorFailure(w http.ResponseWriter, err error) {
	status := selectorFailureStatus(err)
	if status == http.StatusNotFound {
		httpx.ErrorCode(w, status, CodeElementNotFound, err.Error(), false, nil)
		return
	}
	httpx.Error(w, status, err)
}
