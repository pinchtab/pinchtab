package bridge

import (
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestDisableAnimationsCSS(t *testing.T) {
	if !strings.Contains(DisableAnimationsCSS, "animation: none !important") {
		t.Error("CSS missing animation: none")
	}
	if !strings.Contains(DisableAnimationsCSS, "transition: none !important") {
		t.Error("CSS missing transition: none")
	}
	if !strings.Contains(DisableAnimationsCSS, "scroll-behavior: auto !important") {
		t.Error("CSS missing scroll-behavior: auto")
	}
}

func TestInjectNoAnimations(t *testing.T) {
	cfg := &config.RuntimeConfig{NoAnimations: true}
	b := &Bridge{Config: cfg}
	if b.Config.NoAnimations != true {
		t.Error("Expected NoAnimations to be true")
	}
}
