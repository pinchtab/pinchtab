package config

import "sync/atomic"

// Live is the one publication point for a runtime config that outlives the
// goroutine which built it. Holders keep the Live, never the *RuntimeConfig it
// currently answers, so a save publishes a NEW value instead of writing the
// fields of an object other goroutines are already reading.
//
// That is also the rule for anything BUILT ONCE and served for the life of the
// process — an HTTP middleware chain above all. A component that captured the
// *RuntimeConfig it was constructed with used to see saves, because a save wrote
// that object's fields; now it would enforce boot values forever while the
// dashboard reports the save as applied. Take the Live and resolve it per
// request; a nil Live answers nil, which every reader already handles.
//
// Two fields are excluded from that rule on provable properties rather than on
// expectations, and they are named here so the next holder knows which side it is
// on. BackgroundMarker exists only on RuntimeConfig, so no config file can carry
// it and no save can change it. Token cannot be written through the dashboard at
// all — PUT /api/config refuses a token write outright (token_write_only) — and
// the whole Security section is separately declared restart-required. Both are
// resolved per request anyway now that the chain reads through the Live; that is
// a consequence of one contract for the chain, not a change in what either can
// do.
type Live struct {
	current atomic.Pointer[RuntimeConfig]
}

func NewLive(cfg *RuntimeConfig) *Live {
	live := &Live{}
	live.Publish(cfg)
	return live
}

// Get answers the value in effect. The result is read-only by contract: mutating
// it puts the process back in the race this type exists to close.
func (l *Live) Get() *RuntimeConfig {
	if l == nil {
		return nil
	}
	return l.current.Load()
}

func (l *Live) Publish(cfg *RuntimeConfig) {
	if l == nil {
		return
	}
	l.current.Store(cfg)
}

// NextRuntimeConfig derives the value a save publishes: the file config applied to
// a COPY of what is in effect, so the object readers hold is never written to.
func NextRuntimeConfig(base *RuntimeConfig, fc *FileConfig) *RuntimeConfig {
	next := CloneRuntimeConfig(base)
	if next == nil {
		return nil
	}
	ApplyFileConfigToRuntime(next, fc)
	return next
}
