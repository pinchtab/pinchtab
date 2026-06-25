package handlers

import (
	"context"
	"errors"
	"fmt"

	"github.com/pinchtab/pinchtab/internal/bridge"
)

var ErrElementNotFound = errors.New("element not found")

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

func statusForElementErr(err error) int {
	if errors.Is(err, ErrElementNotFound) {
		return 404
	}
	return 500
}
