# Dialog

Accept or dismiss the JavaScript dialog (`alert`, `confirm`, `prompt`) open on a tab.

```bash
curl -X POST http://localhost:9867/dialog \
  -H "Content-Type: application/json" \
  -d '{"action":"accept"}'
# CLI Alternative
pinchtab dialog accept
# Response (use --json for full JSON)
OK
```

JSON response:

```json
{"type": "alert", "message": "Hello", "handled": true}
```

`type` is `unknown` and `message` empty when PinchTab missed the dialog-open event but a dialog was still answered.

## CLI

| Command | Description |
|---------|-------------|
| `pinchtab dialog accept [text]` | Accept (OK); `text` is the prompt response |
| `pinchtab dialog dismiss` | Dismiss (Cancel) |

Both take `--tab <id>` and `--json`.

## HTTP

`POST /dialog` or `POST /tabs/{id}/dialog`:

| Field | Description |
|-------|-------------|
| `action` | Required: `accept` or `dismiss` |
| `text` | Prompt response, used with `accept` |
| `tabId` | Target tab (default: current tab) |

Errors: `400` for a missing or unknown `action`, or when no dialog is open on the tab (`no dialog open on tab <id>`); `409 tab_paused_handoff` while the tab is paused for handoff.

## Dialog-blocked tabs

While a dialog is open, routes that drive the page — navigate, back/forward/reload, wait, find, evaluate, element reads such as `attr` and `count`, `/action` and `/actions`, storage, and state save — answer `409 dialog_blocked` immediately instead of hanging. The error `details` carry `hint`, `tabId`, `dialogType` and `dialogMessage`, plus `defaultPrompt` for a prompt. Only an alert carries `remedy` (`pinchtab dialog accept`), because it has one answer; a confirm, prompt or beforeunload has two, so there is no remedy and the `hint` names both. A click or double-click that itself opens a dialog is not blocked: it succeeds and names the dialog in `result.dialog` with the same fields. A batch stops at the step after the one that opened the dialog, which fails with code `dialog_blocked`, even with `stopOnError:false`. Answer the dialog with `pinchtab dialog accept|dismiss`, or pass `--dialog-action accept|dismiss` (API `dialogAction`) on the click that opens it — see [Click](./click.md).

Tested by `tests/e2e/scenarios/api/dialog-guard-basic.sh`, `tests/e2e/scenarios/cli/dialog-guard-basic.sh` and `tests/e2e/scenarios/api/actions-extended.sh`.

MCP: `pinchtab_dialog` with `action` (required), `text`, `tabId`.
