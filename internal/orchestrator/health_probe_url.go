// This file isolates the security-sensitive URL/allowlist plumbing for health
// probes (SSRF guards, loopback/attach-allowlist policy, base-URL construction)
// so it stays out of the generic startup/attached-bridge monitoring code.
package orchestrator

import (
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/pinchtab/pinchtab/internal/config"
)

type healthProbePolicy int

const (
	healthProbePolicyLoopback healthProbePolicy = iota
	healthProbePolicyAttachAllowlist
)

func (o *Orchestrator) validatedHealthProbeBaseURL(rawURL, port string, policy healthProbePolicy) (*url.URL, error) {
	baseURL, err := o.parseHTTPInstanceURL(rawURL, port)
	if err != nil {
		return nil, err
	}

	host := baseURL.Hostname()
	switch policy {
	case healthProbePolicyAttachAllowlist:
		if o.runtimeCfg == nil {
			return nil, fmt.Errorf("blocked: attach not configured")
		}
		if !isAllowedAttachHost(host, o.runtimeCfg.AttachAllowHosts) {
			slog.Warn("health probe blocked: host not allowed", "url", rawURL, "host", host)
			return nil, fmt.Errorf("blocked: host not allowed")
		}
	default:
		if !isAllowedChildProbeHost(host, configuredChildBind(o.runtimeCfg)) {
			slog.Warn("health probe blocked: non-loopback host", "url", rawURL, "host", host)
			return nil, fmt.Errorf("blocked: non-loopback host")
		}
	}

	// Reconstruct from validated components to break the CodeQL taint chain
	// from the user-controlled rawURL to the outgoing HTTP request.
	return sanitizedBaseURL(baseURL.Scheme, baseURL.Host), nil
}

// sanitizedBaseURL builds a fresh url.URL from individually validated scheme
// and host strings. This intentionally severs any data-flow link to the
// original user-supplied URL so static-analysis tools (CodeQL CWE-918) can
// verify the value is server-controlled.
func sanitizedBaseURL(scheme, host string) *url.URL {
	return &url.URL{Scheme: scheme, Host: host}
}

func healthProbeURL(baseURL *url.URL) string {
	return (&url.URL{
		Scheme: baseURL.Scheme,
		Host:   baseURL.Host,
		Path:   "/health",
	}).String()
}

// isAllowedProbeHost restricts generic health probes to loopback addresses to
// prevent SSRF when inst.URL is attacker-controlled.
func isAllowedProbeHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func isAllowedChildProbeHost(host, bind string) bool {
	if isAllowedProbeHost(host) {
		return true
	}
	return strings.EqualFold(host, configuredChildInstanceHost(bind))
}

func configuredChildBind(cfg *config.RuntimeConfig) string {
	if cfg == nil {
		return ""
	}
	return strings.TrimSpace(cfg.Bind)
}

func configuredChildInstanceHost(bind string) string {
	bind = strings.TrimSpace(bind)
	switch bind {
	case "", "0.0.0.0", "::":
		return "localhost"
	default:
		return bind
	}
}

func httpBaseURL(host, port string) string {
	return (&url.URL{
		Scheme: "http",
		Host:   net.JoinHostPort(host, port),
	}).String()
}

func instanceBaseURLs(bind string, port int) []string {
	portStr := strconv.Itoa(port)
	candidates := make([]string, 0, 4)
	seen := make(map[string]struct{}, 4)
	appendURL := func(host string) {
		baseURL := httpBaseURL(host, portStr)
		if _, ok := seen[baseURL]; ok {
			return
		}
		seen[baseURL] = struct{}{}
		candidates = append(candidates, baseURL)
	}

	bind = strings.TrimSpace(bind)
	if bind != "" && bind != "0.0.0.0" && bind != "::" {
		appendURL(bind)
	}
	appendURL("127.0.0.1")
	appendURL("::1")
	appendURL("localhost")
	return candidates
}
