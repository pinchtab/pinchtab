# Security

PinchTab is designed to be usable by default on a local machine without exposing high-risk browser control features unless you explicitly turn them on.

PinchTab's default and primary deployment model is local-first: one user, one machine, one operator-controlled browser control plane. More complex topologies such as Docker, LAN access, remote bridges, or distributed orchestrator setups are supported, but they are advanced deployments. PinchTab should not be treated as a turnkey internet-facing service, and securing those deployments is the operator's responsibility.

If you run PinchTab on a different machine, do so only if you understand the security model you are operating. Prefer a private or otherwise closed network, avoid exposing the service directly to the public internet, and keep high-risk capabilities disabled unless they are required for that deployment. If they must be enabled, restrict them so only the minimum trusted systems that need them can reach them.

The default security posture is:

- `server.bind = 127.0.0.1`
- `server.token` is generated during default setup and should remain set
- `security.allowEvaluate = false`
- `security.allowMacro = false`
- `security.allowScreencast = false`
- `security.allowDownload = false`
- `security.allowUpload = false`
- `security.attach.enabled = false`
- `security.attach.allowHosts = ["127.0.0.1", "localhost", "::1"]`
- `security.attach.allowSchemes = ["ws", "wss"]`
- `security.idpi.enabled = true`
- `security.idpi.allowedDomains = ["127.0.0.1", "localhost", "::1"]`
- `security.idpi.strictMode = true`
- `security.idpi.scanContent = true`
- `security.idpi.wrapContent = true`

Use `pinchtab security` to review the current posture and restore the recommended defaults.

## Security Philosophy

PinchTab follows a few simple rules:

- default to local-only access
- default dangerous capabilities to off
- separate transport access from feature exposure
- fail closed when content or domain trust cannot be established

This means there are two independent questions:

1. who can reach the server
2. what the server is allowed to do once reached

Both matter.

## Advanced Deployments

If you intentionally run PinchTab beyond the default local setup, the minimum operator checklist is:

- keep `server.token` set to a strong random value
- narrow network reachability with a trusted network boundary, VPN, firewall, or reverse proxy
- add TLS at the proxy or transport layer when traffic leaves the local machine
- keep sensitive endpoint families disabled unless they are explicitly needed, and if they are enabled, restrict them to the minimum trusted callers or network paths that must reach them
- scope `security.attach` and `security.idpi` deliberately for the remote topology you are operating

Those choices are deployment responsibilities, not defaults that PinchTab can infer safely on your behalf.

When the server is not running on the same machine as the user or agent, the bar should be higher: know which hosts can reach it, know which credentials protect it, know which endpoint families are enabled, and know which network boundary is containing it.

Binding to loopback reduces who can reach the API. Tokens reduce who can use it successfully. Sensitive endpoint gates reduce what a successful caller can do. IDPI reduces which websites and extracted content are trusted enough to pass deeper into an agent workflow.

## API Token

`server.token` is the master API token.

For non-browser clients, requests should send:

```http
Authorization: Bearer <token>
```

The browser dashboard uses a different flow:

1. the user enters the token once on the login page
2. the server exchanges it for a same-origin `HttpOnly` session cookie
3. sensitive dashboard actions can require token re-entry for short-lived
   elevation

Why this matters:

- without a token, any process that can reach the server can call the API
- on `127.0.0.1`, that still includes local scripts, browser pages, other users on the same machine, and malware
- on `0.0.0.0` or a LAN bind, a missing token is a much bigger risk

Recommended practice:

- keep `server.bind` on `127.0.0.1`
- set a strong random `server.token`
- only widen the bind when remote access is intentional

`pinchtab config init` generates and stores a token as part of the default setup:

```bash
pinchtab config init
```

You can also generate one from the dashboard Settings page or let `pinchtab security` restore create one if `server.token` is empty.

If you are calling the API manually:

```bash
curl -H "Authorization: Bearer <token>" http://127.0.0.1:9867/health
```

CLI commands use the configured local server settings by default, and `PINCHTAB_TOKEN` can override the token for a single shell session.

## Sensitive Endpoints

Some endpoint families expose much more power than normal navigation and inspection. PinchTab keeps them disabled by default:

- `security.allowEvaluate`
- `security.allowMacro`
- `security.allowScreencast`
- `security.allowDownload`
- `security.allowUpload`

Why they are considered dangerous:

- `evaluate` can execute JavaScript in page context
- `macro` can trigger higher-level automation flows
- `screencast` can stream live page contents
- `download` can fetch and persist remote content
- `upload` can push local files into browser flows

These are not the same as authentication.

- auth decides who may call the API
- sensitive endpoint gates decide which high-risk capabilities exist at all

For example, a token-protected server with `security.allowEvaluate = true` is still intentionally exposing JavaScript execution to any caller that has the token.

When disabled, these routes are locked and return a `403` explaining that the endpoint family is disabled in config.

## Attach Policy

Attach is an advanced feature for registering an externally managed Chrome instance through a CDP URL. It is disabled by default:

```json
{
  "security": {
    "attach": {
      "enabled": false,
      "allowHosts": ["127.0.0.1", "localhost", "::1"],
      "allowSchemes": ["ws", "wss"]
    }
  }
}
```

If you enable attach:

- keep `allowHosts` narrowly scoped
- prefer local-only hosts unless external Chrome targets or remote bridges are intentional
- only attach to browsers and CDP endpoints you trust

If you use `POST /instances/attach-bridge`, `security.attach.allowSchemes` must also include `http` or `https`.

## IDPI

IDPI stands for Indirect Prompt Injection defense.

It exists to reduce the chance that untrusted website content influences downstream agents through hidden instructions, poisoned text, or unsafe navigation.

PinchTab's IDPI layer currently does four things:

- restricts navigation to an allowlist of approved domains
- blocks or warns when a URL cannot be matched against that allowlist
- scans extracted content for suspicious prompt-injection patterns
- wraps text output so downstream systems can treat it as untrusted content

The default local-only IDPI config is:

```json
{
  "security": {
    "idpi": {
      "enabled": true,
      "allowedDomains": ["127.0.0.1", "localhost", "::1"],
      "strictMode": true,
      "scanContent": true,
      "wrapContent": true,
      "customPatterns": []
    }
  }
}
```

Important notes:

- if `allowedDomains` is empty, the main domain restriction is not doing useful work
- if `allowedDomains` contains `"*"`, the whitelist effectively allows everything
- `strictMode = true` blocks disallowed domains and suspicious content
- `strictMode = false` allows the request but emits warnings instead
- `scanContent` protects `/text` and `/snapshot` style extraction paths
- `wrapContent` adds explicit untrusted-content framing for downstream consumers

Supported domain patterns are:

- exact host: `example.com`
- subdomain wildcard: `*.example.com`
- full wildcard: `*`

`*` is convenient, but it defeats the main allowlist defense and should be avoided unless you are deliberately disabling domain restriction.

## Recommended Config

For a secure local setup:

```json
{
  "server": {
    "bind": "127.0.0.1",
    "token": "replace-with-a-generated-token"
  },
  "security": {
    "allowEvaluate": false,
    "allowMacro": false,
    "allowScreencast": false,
    "allowDownload": false,
    "allowUpload": false,
    "attach": {
      "enabled": false,
      "allowHosts": ["127.0.0.1", "localhost", "::1"],
      "allowSchemes": ["ws", "wss"]
    },
    "idpi": {
      "enabled": true,
      "allowedDomains": ["127.0.0.1", "localhost", "::1"],
      "strictMode": true,
      "scanContent": true,
      "wrapContent": true,
      "customPatterns": []
    }
  }
}
```

If you intentionally expose PinchTab beyond localhost, treat the token as mandatory and keep the sensitive endpoint families disabled unless you have a specific reason to enable them. For anything more exposed than a single-machine local setup, assume you are operating an advanced deployment and review each security control explicitly.
