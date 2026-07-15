package browsers

import "sort"

type BrowserCapability string

const (
	CapCDP                 BrowserCapability = "cdp"
	CapHeadless            BrowserCapability = "headless"
	CapPDF                 BrowserCapability = "pdf"
	CapExtensions          BrowserCapability = "extensions"
	CapNativeStealth       BrowserCapability = "nativeStealth"
	CapDownloads           BrowserCapability = "downloads"
	CapNetworkInterception BrowserCapability = "networkInterception"
	CapEventScreencast     BrowserCapability = "eventScreencast"
	// CapRuntimeConsoleEvents marks browsers whose CDP Runtime domain delivers
	// consoleAPICalled/exceptionThrown events after navigations. Stealth-patched
	// builds (e.g. Cloak) natively suppress these events because Runtime.enable
	// is a bot-detection vector; the bridge falls back to the deprecated Console
	// domain on a side session for such browsers.
	CapRuntimeConsoleEvents BrowserCapability = "runtimeConsoleEvents"
)

// CapabilitySet is an immutable set of BrowserCapability values.
type CapabilitySet struct {
	m map[BrowserCapability]struct{}
}

// NewCapabilitySet returns a CapabilitySet containing the given capabilities.
// Duplicates are silently deduplicated. A no-arg call returns a valid empty set.
func NewCapabilitySet(caps ...BrowserCapability) CapabilitySet {
	m := make(map[BrowserCapability]struct{}, len(caps))
	for _, c := range caps {
		m[c] = struct{}{}
	}
	return CapabilitySet{m: m}
}

// Has reports whether c is in the set. Safe to call on an empty set.
func (cs CapabilitySet) Has(c BrowserCapability) bool {
	_, ok := cs.m[c]
	return ok
}

// List returns a sorted slice of all capabilities in the set.
// The sort order is the string value of each capability, ensuring
// deterministic output.
func (cs CapabilitySet) List() []BrowserCapability {
	out := make([]BrowserCapability, 0, len(cs.m))
	for c := range cs.m {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		return string(out[i]) < string(out[j])
	})
	return out
}

func (cs CapabilitySet) Len() int {
	return len(cs.m)
}
