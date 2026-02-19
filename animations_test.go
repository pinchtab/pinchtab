package main

import (
	"strings"
	"testing"
)

func TestDisableAnimationsCSSContent(t *testing.T) {

	if !strings.Contains(disableAnimationsCSS, "animation: none !important") {
		t.Error("missing animation: none rule")
	}
	if !strings.Contains(disableAnimationsCSS, "transition: none !important") {
		t.Error("missing transition: none rule")
	}
	if !strings.Contains(disableAnimationsCSS, "animation-duration: 0s !important") {
		t.Error("missing animation-duration: 0s rule")
	}
	if !strings.Contains(disableAnimationsCSS, "transition-duration: 0s !important") {
		t.Error("missing transition-duration: 0s rule")
	}
	if !strings.Contains(disableAnimationsCSS, "scroll-behavior: auto !important") {
		t.Error("missing scroll-behavior: auto rule")
	}
	if !strings.Contains(disableAnimationsCSS, "data-pinchtab") {
		t.Error("missing data-pinchtab attribute for identification")
	}
}

func TestDisableAnimationsCSSIsIIFE(t *testing.T) {
	trimmed := strings.TrimSpace(disableAnimationsCSS)
	if !strings.HasPrefix(trimmed, "(function()") {
		t.Error("CSS injection should be wrapped in IIFE")
	}
	if !strings.HasSuffix(trimmed, "();") {
		t.Error("CSS injection should end with IIFE invocation")
	}
}

func TestNoAnimationsConfigDefault(t *testing.T) {

	if cfg.NoAnimations {
		t.Error("noAnimations should default to false")
	}
}
