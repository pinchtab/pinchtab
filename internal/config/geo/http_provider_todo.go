package geo

// TODO(P4b-future): HTTP-backed GeoProvider.
//
// P4b ships only Noop and Static. A future phase will add an
// `HTTPGeoProvider` that resolves the proxy egress IP against an external
// service (ipinfo.io, Maxmind GeoLite2, ip-api, etc.) so users can wire
// "automatic" geo alignment without supplying browser.proxy.geo.* by hand.
//
// Contract (proposed):
//
//   type HTTPGeoProvider struct {
//       Endpoint string         // e.g. "https://ipinfo.io/{ip}/json"
//       Token    string         // optional bearer/api key, redacted in logs
//       Client   *http.Client   // injected for testability; nil → http.DefaultClient with a short timeout
//       Cache    GeoCache       // in-memory TTL cache keyed by IP
//   }
//
//   func (h HTTPGeoProvider) Lookup(ctx context.Context, ip string) (Info, error) {
//       // 1. ip == "" → return Info{}, nil (best-effort, never fail)
//       // 2. cache hit → return cached Info, nil
//       // 3. ctx-aware GET to Endpoint, parse provider-specific JSON, map to
//       //    Info, cache, return.
//       // 4. on any HTTP/parse error → return Info{}, nil and log at Warn.
//       //    Geo alignment is a hint, not a hard requirement; a bad lookup
//       //    must not block launch.
//   }
//
// Open questions deferred until then:
//   - Configuration surface (browser.proxy.geo.http.{endpoint,token,ttl}?).
//   - Whether to support multiple providers with failover.
//   - Cache eviction policy and TTL default.
//   - How to surface lookup status in /stealth/status or the dashboard.
//
// Until those are settled, this file intentionally contains no executable
// code so we don't ship a half-wired network path.
