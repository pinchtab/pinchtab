package bridgekit

import (
	"testing"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
)

// recordingFetchPauseBridge records SetFetchPauseSuppressed calls; all other
// BridgeAPI methods are the embedded nil (unused here).
type recordingFetchPauseBridge struct {
	bridge.BridgeAPI
	calls []struct {
		tabID string
		v     bool
	}
}

func (r *recordingFetchPauseBridge) SetFetchPauseSuppressed(tabID string, v bool) {
	r.calls = append(r.calls, struct {
		tabID string
		v     bool
	}{tabID, v})
}

// TestBridgeAdapterForwardsSetFetchPauseSuppressed guards the contract that a
// decorated (ghost-chrome) bridge still forwards fetch-pause suppression to the
// wrapped chrome bridge — previously dropped because the method was not on
// bridge.BridgeAPI and the adapter's interface type did not expose it.
func TestBridgeAdapterForwardsSetFetchPauseSuppressed(t *testing.T) {
	rec := &recordingFetchPauseBridge{}
	adapter := NewBridgeAdapter(rec, &config.RuntimeConfig{})

	adapter.SetFetchPauseSuppressed("tab-1", true)
	adapter.SetFetchPauseSuppressed("tab-1", false)

	if len(rec.calls) != 2 {
		t.Fatalf("forwarded %d calls, want 2", len(rec.calls))
	}
	if rec.calls[0] != (struct {
		tabID string
		v     bool
	}{"tab-1", true}) {
		t.Errorf("first call = %+v, want {tab-1 true}", rec.calls[0])
	}
	if rec.calls[1].v != false {
		t.Errorf("second call v = %v, want false", rec.calls[1].v)
	}
}
