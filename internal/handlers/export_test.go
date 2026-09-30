package handlers

import (
	"context"
	"net/http"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/selector"
	"github.com/pinchtab/pinchtab/internal/session"
)

func (s *CurrentTabStore) SetCap(n int) {
	if s == nil || n <= 0 {
		return
	}
	s.mu.Lock()
	s.cap = n
	s.evictExcessLocked()
	s.mu.Unlock()
}

func (h *Handlers) SetEmptyPointerPolicy(p EmptyPointerPolicy) {
	if h == nil {
		return
	}
	if p == "" {
		p = EmptyPointerLazy
	}
	h.emptyPointerPolicy = p
}

func sessionRequestAllowed(r *http.Request, sess *session.Session) bool {
	_, refused := sessionRequestRefusal(r, sess)
	return !refused
}

func (h *Handlers) applySemanticActionSelector(ctx context.Context, tabID string, sel selector.Selector, req *bridge.ActionRequest) (bool, error) {
	return h.applySemanticActionSelectorInScope(ctx, tabID, h.selectorFrameID(tabID), 0, sel, req)
}
