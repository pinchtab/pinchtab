# browserleaks — BrowserLeaks Multi-Probe

**URLs** (the playbook navigates four sub-pages and merges metrics):

- https://browserleaks.com/canvas
- https://browserleaks.com/webgl
- https://browserleaks.com/fonts
- https://browserleaks.com/tls

BrowserLeaks doesn't issue an overall score — it produces extremely detailed
per-category breakdowns. We capture the most-cited identifiers per category.

## Steps

For each URL in the list above:

1. `nav <url> --new-tab --print-tab-id --dismiss-banners`. Save the tab ID.
2. `wait --text "Fingerprint" --timeout 20000` (most BrowserLeaks pages render that label).
3. `text` and capture the relevant metrics for that page.
4. Close the tab when done — `tab close <id>` — to keep things tidy.

After all four navs, aggregate into ONE record with `site: "browserleaks"` and
all metrics merged into the same `metrics` map.

## Metrics

From /canvas:
| key                  | what to look for                          |
|----------------------|-------------------------------------------|
| canvas_signature     | "Signature" hash near the top             |
| canvas_uniqueness    | uniqueness % or ratio shown               |

From /webgl:
| key                  | what to look for                          |
|----------------------|-------------------------------------------|
| webgl_vendor         | UNMASKED_VENDOR or "Vendor" row           |
| webgl_renderer       | UNMASKED_RENDERER or "Renderer" row       |
| webgl_image_hash     | "Image Hash" / "WebGL Hash"               |

From /fonts:
| key                  | what to look for                          |
|----------------------|-------------------------------------------|
| fonts_count          | number of fonts detected                  |
| fonts_signature      | fonts fingerprint hash if shown           |

From /tls:
| key                  | what to look for                          |
|----------------------|-------------------------------------------|
| tls_ja3              | JA3 hash if shown                         |
| tls_ja4              | JA4 hash if shown                         |
| tls_user_agent       | TLS-derived UA / client guess             |
| http2_fingerprint    | HTTP/2 / Akamai fingerprint if shown      |

## Gotchas

- BrowserLeaks pages embed an ad banner; `--dismiss-banners` usually clears it but if a banner blocks reads, `click` the visible Accept / Close.
- /tls is sometimes very slow on first hit (the server runs a live TLS probe). `wait 6000` after nav before reading if the JA3 cell is empty.
