# Geo Provider

## Overview

The geo provider (`internal/config/geo`) resolves geographic information for a
proxy egress IP so users can wire "automatic" geo alignment without supplying
`browser.proxy.geo.*` by hand.

P4b ships only the `Noop` and `Static` providers. This note captures the
proposed contract for a future HTTP-backed provider so the design lives in
documentation rather than as ballast in the compiled package.

## Future: HTTP-backed GeoProvider

A future phase will add an `HTTPGeoProvider` that resolves the proxy egress IP
against an external service (ipinfo.io, Maxmind GeoLite2, ip-api, etc.).

### Proposed contract

```go
type HTTPGeoProvider struct {
    Endpoint string       // e.g. "https://ipinfo.io/{ip}/json"
    Token    string       // optional bearer/api key, redacted in logs
    Client   *http.Client // injected for testability; nil → http.DefaultClient with a short timeout
    Cache    GeoCache     // in-memory TTL cache keyed by IP
}

func (h HTTPGeoProvider) Lookup(ctx context.Context, ip string) (Info, error) {
    // 1. ip == "" → return Info{}, nil (best-effort, never fail)
    // 2. cache hit → return cached Info, nil
    // 3. ctx-aware GET to Endpoint, parse provider-specific JSON, map to
    //    Info, cache, return.
    // 4. on any HTTP/parse error → return Info{}, nil and log at Warn.
    //    Geo alignment is a hint, not a hard requirement; a bad lookup
    //    must not block launch.
}
```

### Open questions

- Configuration surface (`browser.proxy.geo.http.{endpoint,token,ttl}`?).
- Whether to support multiple providers with failover.
- Cache eviction policy and TTL default.
- How to surface lookup status in `/stealth/status` or the dashboard.

Until those are settled, no executable code ships so we don't introduce a
half-wired network path.
