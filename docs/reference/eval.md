# Eval

Run JavaScript in the current tab. This endpoint is disabled unless evaluation is explicitly enabled in config.

Enabling `security.allowEvaluate` is a documented, non-default, security-reducing configuration change. It allows arbitrary JavaScript execution in page context and should only be used on trusted systems with authentication and network exposure reviewed explicitly.

```bash
curl -X POST http://localhost:9867/evaluate \
  -H "Content-Type: application/json" \
  -d '{"expression":"document.title"}'
# CLI Alternative
pinchtab eval "document.title"
# Response
{
  "result": "Example Domain"
}
```

Notes:

- requires `security.allowEvaluate: true`
- the tab-scoped variant is `POST /tabs/{id}/evaluate`

## Related Pages

- [Config](./config.md)
- [Tabs](./tabs.md)
