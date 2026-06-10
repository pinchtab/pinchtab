package bridgekit

import (
	"fmt"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
)

func (a *BridgeAdapter) FocusTab(tabID string) error {
	if chromeID, ok := a.proxy.ChromeTabID(tabID); ok {
		return a.BridgeAPI.FocusTab(chromeID)
	}
	return a.BridgeAPI.FocusTab(tabID)
}

func (a *BridgeAdapter) CloseTab(tabID string) error {
	if chromeID, ok := a.proxy.ChromeTabID(tabID); ok {
		err := a.BridgeAPI.CloseTab(chromeID)
		// Scrub the mapping even if the Chrome close errored — the caller
		// asked for the tab to be gone; a stale mapping would resurrect it
		// on the next TabContext.
		a.proxy.ReleaseTab(tabID)
		return err
	}
	if sb := a.StaticBrowser(); sb != nil && sb.CloseTab(tabID) {
		a.proxy.ReleaseTab(tabID)
		return nil
	}
	return a.BridgeAPI.CloseTab(tabID)
}

func (a *BridgeAdapter) AvailableActions() []string {
	return a.proxy.AvailableActions()
}

func (a *BridgeAdapter) GetDocumentReadyState(tabID string) (string, error) {
	type readyStater interface {
		GetDocumentReadyState(string) (string, error)
	}
	if rs, ok := a.BridgeAPI.(readyStater); ok {
		return rs.GetDocumentReadyState(tabID)
	}
	return "", nil
}

func (a *BridgeAdapter) IsNetworkIdle(tabID string) (bool, bool) {
	type idleChecker interface {
		IsNetworkIdle(string) (bool, bool)
	}
	if ic, ok := a.BridgeAPI.(idleChecker); ok {
		return ic.IsNetworkIdle(tabID)
	}
	return false, false
}

func (a *BridgeAdapter) SetFingerprintRotateActive(tabID string, active bool) {
	type setter interface {
		SetFingerprintRotateActive(string, bool)
	}
	if s, ok := a.BridgeAPI.(setter); ok {
		s.SetFingerprintRotateActive(tabID, active)
	}
}

func (a *BridgeAdapter) FingerprintRotateActive(tabID string) bool {
	type getter interface {
		FingerprintRotateActive(string) bool
	}
	g, ok := a.BridgeAPI.(getter)
	if !ok {
		return false
	}
	if g.FingerprintRotateActive(tabID) {
		return true
	}
	// The rotate handler stores the flag under the resolved Chrome tab ID,
	// but callers may query with the original lite tab ID.
	if chromeID, mapped := a.proxy.ChromeTabID(tabID); mapped {
		return g.FingerprintRotateActive(chromeID)
	}
	return false
}

func (a *BridgeAdapter) GetFrameScope(tabID string) (bridge.FrameScope, bool) {
	type frameScopeAPI interface {
		GetFrameScope(string) (bridge.FrameScope, bool)
	}
	if fs, ok := a.BridgeAPI.(frameScopeAPI); ok {
		return fs.GetFrameScope(tabID)
	}
	return bridge.FrameScope{}, false
}

func (a *BridgeAdapter) SetFrameScope(tabID string, scope bridge.FrameScope) {
	type frameScopeAPI interface {
		SetFrameScope(string, bridge.FrameScope)
	}
	if fs, ok := a.BridgeAPI.(frameScopeAPI); ok {
		fs.SetFrameScope(tabID, scope)
	}
}

func (a *BridgeAdapter) ClearFrameScope(tabID string) {
	type frameScopeAPI interface {
		ClearFrameScope(string)
	}
	if fs, ok := a.BridgeAPI.(frameScopeAPI); ok {
		fs.ClearFrameScope(tabID)
	}
}

func (a *BridgeAdapter) SetTabHandoff(tabID, reason string, timeout time.Duration) error {
	type handoffAPI interface {
		SetTabHandoff(string, string, time.Duration) error
	}
	if h, ok := a.BridgeAPI.(handoffAPI); ok {
		return h.SetTabHandoff(tabID, reason, timeout)
	}
	return fmt.Errorf("bridge does not support handoff state")
}

func (a *BridgeAdapter) ResumeTabHandoff(tabID string) error {
	type handoffAPI interface {
		ResumeTabHandoff(string) error
	}
	if h, ok := a.BridgeAPI.(handoffAPI); ok {
		return h.ResumeTabHandoff(tabID)
	}
	return fmt.Errorf("bridge does not support handoff state")
}

func (a *BridgeAdapter) TabHandoffState(tabID string) (bridge.TabHandoffState, bool) {
	type handoffAPI interface {
		TabHandoffState(string) (bridge.TabHandoffState, bool)
	}
	if h, ok := a.BridgeAPI.(handoffAPI); ok {
		return h.TabHandoffState(tabID)
	}
	return bridge.TabHandoffState{}, false
}
