# Navigate

Open a new tab and navigate it to a URL, or reuse a tab when a tab ID is provided through the API.

```bash
curl -X POST http://localhost:9867/navigate \
  -H "Content-Type: application/json" \
  -d '{"url":"https://pinchtab.com"}'
# CLI Alternative
pinchtab nav https://pinchtab.com
# Response
{
  "tabId": "8f9c7d4e1234567890abcdef12345678",
  "url": "https://pinchtab.com",
  "title": "Example Domain"
}
```

Useful flags:

- CLI: `--new-tab`, `--block-images`, `--block-ads`
- API body: `tabId`, `newTab`, `timeout`, `blockImages`, `blockAds`, `waitFor`, `waitSelector`

## Related Pages

- [Snapshot](./snapshot.md)
- [Tabs](./tabs.md)
