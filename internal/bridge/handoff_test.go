package bridge

import (
	"context"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestTabHandoffState_Expires(t *testing.T) {
	b := New(context.TODO(), nil, &config.RuntimeConfig{})
	if err := b.SetTabHandoff("tab1", "manual", 10*time.Millisecond); err != nil {
		t.Fatalf("set handoff: %v", err)
	}
	if _, ok := b.TabHandoffState("tab1"); !ok {
		t.Fatal("expected handoff state to exist immediately")
	}
	time.Sleep(20 * time.Millisecond)
	if _, ok := b.TabHandoffState("tab1"); ok {
		t.Fatal("expected handoff state to expire")
	}
}
