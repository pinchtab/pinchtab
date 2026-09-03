package config

import "sync/atomic"

// Live is the one publication point for a runtime config that outlives the
// goroutine which built it. Holders keep the Live, never the *RuntimeConfig it
// currently answers, so a save publishes a NEW value instead of writing the
// fields of an object other goroutines are already reading.
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
