# Handoff

Mark a tab for human intervention, inspect the handoff state, then resume automation when the manual step is complete.

There is currently no dedicated CLI wrapper. Handoff is API-only.

> Temporary status: the current handoff implementation is advisory only.
> `POST /tabs/{id}/handoff` records `paused_handoff`, but it does not yet enforce a hard execution block across later automation requests.
> Treat it as coordination metadata for now, not as a security boundary. A stricter blocking model tied to session-owned locks is expected later.

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
- `POST /tabs/{id}/resume` clears the handoff state and can carry resume metadata such as `status` or `resolvedData`
- today this is informational state only; it does not by itself block other later automation
- use this for CAPTCHA, 2FA, login approval, or other human-only steps only if your caller also coordinates access separately

## Related Pages

- [Tabs](./tabs.md)
- [CLI Overview](./cli.md)
