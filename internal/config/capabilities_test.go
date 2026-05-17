package config

import (
	"reflect"
	"testing"
)

func TestProviderCapabilities_Chrome(t *testing.T) {
	got := ProviderCapabilities(BrowserProviderChrome)
	want := []BrowserCapability{
		CapCDP,
		CapHeadless,
		CapPDF,
		CapExtensions,
		CapDownloads,
		CapNetworkInterception,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("chrome capabilities = %v, want %v", got, want)
	}
}

func TestProviderCapabilities_Cloak(t *testing.T) {
	got := ProviderCapabilities(BrowserProviderCloak)
	want := []BrowserCapability{
		CapCDP,
		CapHeadless,
		CapPDF,
		CapExtensions,
		CapDownloads,
		CapNetworkInterception,
		CapNativeStealth,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("cloak capabilities = %v, want %v", got, want)
	}

	// Guards against accidental divergence: cloak must be chrome + nativeStealth.
	chrome := ProviderCapabilities(BrowserProviderChrome)
	for _, c := range chrome {
		if !HasCapability(BrowserProviderCloak, c) {
			t.Fatalf("cloak missing chrome capability %q", c)
		}
	}
	if !HasCapability(BrowserProviderCloak, CapNativeStealth) {
		t.Fatalf("cloak missing nativeStealth capability")
	}
	if HasCapability(BrowserProviderChrome, CapNativeStealth) {
		t.Fatalf("chrome unexpectedly declares nativeStealth")
	}
}

func TestProviderCapabilities_UnknownProvider_Empty(t *testing.T) {
	for _, name := range []string{"lightpanda", "firefox", "", "  ", "unknown"} {
		got := ProviderCapabilities(name)
		if len(got) != 0 {
			t.Fatalf("provider %q: expected empty slice, got %v", name, got)
		}
	}
}

func TestHasCapability_KnownCap_True(t *testing.T) {
	if !HasCapability(BrowserProviderChrome, CapCDP) {
		t.Fatalf("chrome should declare cdp")
	}
	if !HasCapability(BrowserProviderCloak, CapNativeStealth) {
		t.Fatalf("cloak should declare nativeStealth")
	}
	if !HasCapability("  Chrome ", CapHeadless) {
		t.Fatalf("mixed-case chrome should declare headless")
	}
}

func TestHasCapability_KnownProvider_UnknownCap_False(t *testing.T) {
	if HasCapability(BrowserProviderChrome, CapNativeStealth) {
		t.Fatalf("chrome should not declare nativeStealth")
	}
	if HasCapability(BrowserProviderChrome, BrowserCapability("nonexistent")) {
		t.Fatalf("chrome should not declare a fabricated capability")
	}
	if HasCapability("lightpanda", CapCDP) {
		t.Fatalf("unknown provider should report no capabilities")
	}
}

func TestProviderCapabilities_ReturnsCopy(t *testing.T) {
	first := ProviderCapabilities(BrowserProviderChrome)
	if len(first) == 0 {
		t.Fatalf("expected non-empty chrome capabilities")
	}
	first[0] = BrowserCapability("mutated")
	second := ProviderCapabilities(BrowserProviderChrome)
	if second[0] != CapCDP {
		t.Fatalf("mutation leaked into table: second[0] = %q", second[0])
	}
	if reflect.DeepEqual(first, second) {
		t.Fatalf("mutated slice should differ from a fresh lookup")
	}
}
