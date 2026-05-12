package bridge

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/chromedp/cdproto/target"
	"github.com/pinchtab/pinchtab/internal/config"
)

func TestPendingClick_ClaimsPopupForOpener(t *testing.T) {
	tm := &TabManager{}
	opener := target.ID("OPENER123")
	newTarget := target.ID("POPUP456")

	slot := tm.markPendingClick(opener)
	if slot == nil {
		t.Fatal("markPendingClick returned nil")
	}

	if !tm.claimPendingPopup(opener, newTarget) {
		t.Fatal("expected popup to be claimed when click is pending")
	}

	select {
	case got := <-slot.captured:
		if got != newTarget {
			t.Fatalf("captured target = %q, want %q", got, newTarget)
		}
	default:
		t.Fatal("captured channel had no value")
	}
}

func TestPendingClick_DoesNotClaimWithoutPending(t *testing.T) {
	tm := &TabManager{}
	if tm.claimPendingPopup(target.ID("OPENER"), target.ID("POPUP")) {
		t.Fatal("claimPendingPopup should return false with no pending click")
	}
}

func TestPendingClick_DoesNotClaimForOtherOpener(t *testing.T) {
	tm := &TabManager{}
	tm.markPendingClick(target.ID("OPENER_A"))
	if tm.claimPendingPopup(target.ID("OPENER_B"), target.ID("POPUP")) {
		t.Fatal("claimPendingPopup should ignore mismatched opener")
	}
}

func TestPendingClick_SecondPopupNotClaimed(t *testing.T) {
	tm := &TabManager{}
	opener := target.ID("OPENER")
	tm.markPendingClick(opener)

	if !tm.claimPendingPopup(opener, target.ID("POPUP1")) {
		t.Fatal("first popup should be claimed")
	}
	if tm.claimPendingPopup(opener, target.ID("POPUP2")) {
		t.Fatal("second popup should not be claimed — guard must close it")
	}
}

func TestPendingClick_ClearReleasesSlot(t *testing.T) {
	tm := &TabManager{}
	opener := target.ID("OPENER")
	slot := tm.markPendingClick(opener)
	if targetID, ok := tm.clearPendingClick(opener, slot); ok || targetID != "" {
		t.Fatalf("clearPendingClick returned target %q, %v; want none", targetID, ok)
	}
	if tm.claimPendingPopup(opener, target.ID("POPUP")) {
		t.Fatal("cleared slot should not claim popups")
	}
}

func TestPendingClick_ClearReturnsClaimedPopup(t *testing.T) {
	tm := &TabManager{}
	opener := target.ID("OPENER")
	popup := target.ID("POPUP")
	slot := tm.markPendingClick(opener)

	if !tm.claimPendingPopup(opener, popup) {
		t.Fatal("expected pending click to claim popup")
	}

	targetID, ok := tm.clearPendingClick(opener, slot)
	if !ok || targetID != popup {
		t.Fatalf("clearPendingClick returned %q, %v; want %q, true", targetID, ok, popup)
	}
	if tm.claimPendingPopup(opener, target.ID("LATE_POPUP")) {
		t.Fatal("cleared slot should not claim late popups")
	}
}

func TestAutoSwitchFinish_NoPopupReturnsWithinGrace(t *testing.T) {
	tm := &TabManager{}
	opener := target.ID("OPENER")
	slot := tm.markPendingClick(opener)
	session := &autoSwitchSession{tm: tm, openerCDP: opener, slot: slot}

	start := time.Now()
	result := session.finish(context.Background(), map[string]any{"clicked": true})
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("finish without popup took %s, want under 500ms", elapsed)
	}
	if _, ok := result["switchedToTab"]; ok {
		t.Fatal("finish without popup should not set switchedToTab")
	}
}

func TestAdoptExistingTarget_EnforcesRejectTabLimit(t *testing.T) {
	tm := NewTabManager(context.Background(), &config.RuntimeConfig{
		MaxTabs:           1,
		TabEvictionPolicy: "reject",
	}, nil, nil, nil)
	tm.tabs["existing"] = &TabEntry{Ctx: context.Background(), CDPID: "existing"}

	_, err := tm.adoptExistingTarget(target.ID("new-target"), true)
	var limitErr *TabLimitError
	if !errors.As(err, &limitErr) {
		t.Fatalf("adoptExistingTarget error = %v, want TabLimitError", err)
	}
}
