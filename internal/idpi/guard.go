package idpi

// Guard abstracts content and domain security scanning so the
// implementation can be swapped without touching handler code.
type Guard interface {
	// ScanContent checks text for prompt-injection threats.
	// Returns a zero CheckResult when nothing suspicious is found.
	ScanContent(text string) CheckResult

	// CheckDomain evaluates rawURL against a domain allowlist.
	// Returns a zero CheckResult when the domain is allowed or no
	// allowlist is configured.
	CheckDomain(rawURL string) CheckResult

	// DomainAllowed reports whether rawURL explicitly matches an
	// allowlist entry (returns false when the allowlist is empty or
	// the feature is disabled).
	DomainAllowed(rawURL string) bool

	// WrapContent encloses untrusted web content in trust-boundary
	// markers for downstream LLMs.
	WrapContent(text, pageURL string) string

	// Enabled reports whether the guard is active.
	Enabled() bool
}
