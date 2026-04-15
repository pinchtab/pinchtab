# Handoff

Mark a tab for human intervention, inspect the handoff state, then resume automation when the manual step is complete.

CLI wrappers are available:

```bash
pinchtab tab handoff <tabId> --reason captcha --timeout-ms 120000
pinchtab tab handoff-status <tabId>
pinchtab tab resume <tabId> --status completed
```

API equivalents:

When a tab is in `paused_handoff`, action execution routes reject with `409 tab_paused_handoff`
until the tab is resumed or the optional handoff timeout expires.

```bash
curl -X POST http://localhost:9867/tabs/<tabId>/handoff \
  -H "Content-Type: application/json" \
  -d '{"reason":"captcha","timeoutMs":120000}'

curl http://localhost:9867/tabs/<tabId>/handoff

curl -X POST http://localhost:9867/tabs/<tabId>/resume \
  -H "Content-Type: application/json" \
  -d '{"status":"completed","resolvedData":{"operator":"human"}}'
```

Notes:

- `POST /tabs/{id}/handoff` sets the tab state to `paused_handoff`
- `GET /tabs/{id}/handoff` returns the current handoff state, or `active` when no handoff is set
- when a timeout is set, status also includes `expiresAt` and `timeoutMs`
- `POST /tabs/{id}/resume` clears the handoff state and can carry resume metadata such as `status` or `resolvedData`
- paused tabs reject `/action`, `/actions/batch`, and `/macro` requests with `tab_paused_handoff`
- use this for CAPTCHA, 2FA, login approval, or other human-only steps

## Related Pages

- [Tabs](./tabs.md)
- [CLI Overview](./cli.md)
