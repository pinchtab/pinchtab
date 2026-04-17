# Remote Bridge With Orchestrator

Use this guide when:

- the PinchTab orchestrator runs on one machine
- a PinchTab bridge server runs on another machine
- you want agents to keep talking to the orchestrator while the browser work happens remotely

This is an advanced deployment pattern. Use it only when you understand the security model, keep the bridge on a private or otherwise closed network, and avoid exposing the bridge or orchestrator broadly beyond the systems that need to reach them. High-risk endpoint families should remain disabled unless they are explicitly required, and if enabled they should be reachable only by the minimum trusted systems involved in the deployment.

This is now a supported orchestration mode through:

```text
POST /instances/attach-bridge
```

The orchestrator does not launch processes on the remote machine. It attaches to an already running bridge and routes requests to its registered origin.

---

## Mental Model

There are now three different models:

- local managed instance: the orchestrator launches and owns a local bridge process
- attached Chrome: the orchestrator registers an external CDP browser with `POST /instances/attach`
- attached bridge: the orchestrator registers an external PinchTab bridge with `POST /instances/attach-bridge`

For remote bridge attachment, the control flow is:

```text
agent -> orchestrator -> attached remote bridge -> Chrome
```

The main practical rule is:

- clients talk to the orchestrator
- the orchestrator talks to the remote bridge

---

## Good Use Cases

### Shared headed browser host

- machine A runs the orchestrator and dashboard
- machine B runs one or more headed bridge servers
- agents keep a single control plane on machine A
- actual browser rendering happens on machine B

This is useful when you want one orchestration surface but do not want to run headed Chrome on every developer machine.

### Region-local browser worker

- machine A runs the orchestrator
- machine B runs a bridge in the same LAN, VPC, or region as the target sites
- the orchestrator attaches that bridge and routes work there

This is useful when latency, egress location, or network topology matters.

---

## What The Feature Does

`POST /instances/attach-bridge`:

- validates the bridge URL against `security.attach`
- checks the remote bridge health endpoint before registering it
- stores the bridge origin as the canonical instance URL
- optionally stores a per-bridge bearer token used by the orchestrator
- lets normal orchestrator routes proxy to that bridge

This feature also updates routing behavior:

- proxying no longer assumes `localhost` for every instance
- the orchestrator now routes to the registered instance origin
- proxy targets are restricted to origins that belong to registered instances

That last point matters for safety: the proxy is not opened up to arbitrary destinations.

---

## Config

Remote bridge attachment reuses the existing attach policy:

```json
{
  "security": {
    "attach": {
      "enabled": true,
      "allowHosts": ["10.0.12.24", "bridge.internal"],
      "allowSchemes": ["ws", "wss", "http", "https"]
    }
  }
}
```

Important notes:

- `security.attach.enabled` must be `true`
- `allowHosts` must include the remote bridge host
- `allowSchemes` must include `http` or `https` for bridge attachment
- `ws` and `wss` are still used for CDP attachment
- `baseUrl` must be a bare bridge origin; do not include credentials, query strings, fragments, or a path

If you use `allowHosts: ["*"]`, the orchestrator will accept any reachable bridge host with an allowed scheme. That is a documented, non-default, security-reducing override: it removes host allowlisting entirely and should only be used on isolated, operator-controlled networks.

If you leave `allowSchemes` as only `ws,wss`, `attach-bridge` will be rejected.

---

## Step 1: Start The Remote Bridge

On the remote machine, configure and start the bridge:

```bash
# Set bind address for network access
pinchtab config set server.bind 0.0.0.0
pinchtab config set server.port 9868
pinchtab config set server.token bridge-secret-token

# Start the bridge
pinchtab bridge
```

This non-loopback bind is a documented, non-default, security-reducing deployment change. It is appropriate here only because the bridge must be reachable from the orchestrator. Keep the bridge token set and expose the port only on a controlled network boundary.

Example bridge origin:

```text
http://10.0.12.24:9868
```

The bridge should already be healthy and reachable before you attach it.

---

## Step 2: Verify The Bridge Directly

From the orchestrator machine:

```bash
curl -H "Authorization: Bearer bridge-secret-token" \
  http://10.0.12.24:9868/health
```

You should get a `200 OK`.

`attach-bridge` also performs its own health probe, but checking directly first makes network and auth issues easier to debug.

---

## Step 3: Attach The Bridge To The Orchestrator

Against the orchestrator:

```bash
curl -X POST http://127.0.0.1:9867/instances/attach-bridge \
  -H "Authorization: Bearer orchestrator-token" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "bridge-eu-west-1",
    "baseUrl": "http://10.0.12.24:9868",
    "token": "bridge-secret-token"
  }'
```

Response shape:

```json
{
  "id": "inst_0a89a5bb",
  "profileId": "prof_278be873",
  "profileName": "bridge-eu-west-1",
  "port": "",
  "url": "http://10.0.12.24:9868",
  "mode": "headed",
  "headless": false,
  "status": "running",
  "attached": true,
  "attachType": "bridge"
}
```

Fields to notice:

- `attached: true`
- `attachType: "bridge"`
- `url` is the registered bridge origin

---

## Step 4: Confirm It Is Registered

```bash
curl -H "Authorization: Bearer orchestrator-token" \
  http://127.0.0.1:9867/instances
```

The attached bridge appears in the normal instance list. The orchestrator treats it as a running instance for routing and fleet operations.

---

## Step 5: Use Normal Orchestrator Routes

Once attached, clients keep talking to the orchestrator.

Examples:

```bash
curl -H "Authorization: Bearer orchestrator-token" \
  http://127.0.0.1:9867/instances/<instanceId>/tabs
```

```bash
curl -X POST http://127.0.0.1:9867/instances/<instanceId>/tabs/open \
  -H "Authorization: Bearer orchestrator-token" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://pinchtab.com"}'
```

```bash
curl -H "Authorization: Bearer orchestrator-token" \
  http://127.0.0.1:9867/tabs/<tabId>/snapshot
```

If your active strategy uses shorthand routing, those routes can also land on an attached bridge because instance selection now works from canonical instance URLs rather than only local ports.

---

## Authentication Model

There are two separate auth hops:

1. client to orchestrator
2. orchestrator to attached bridge

That means you can use different tokens:

- clients send `Authorization: Bearer orchestrator-token`
- the orchestrator sends `Authorization: Bearer bridge-secret-token` to the remote bridge

Clients do not need to know the bridge token. The orchestrator stores it as instance attachment metadata and injects it on outgoing bridge requests.

---

## Lifecycle Semantics

Attached bridges are externally owned.

That means:

- the orchestrator did not launch the remote bridge process
- `attach-bridge` registers routing metadata, not remote process ownership

Current behavior:

- local `Launch()` remains local-only
- stopping an attached bridge removes it from the orchestrator
- for attached bridges, the orchestrator also makes a best-effort `POST /shutdown` call before unregistering
- starting an attached non-bridge instance through the orchestrator is not supported

If you need true remote process launch, that is a different problem and would need a transport such as SSH, an agent, or a scheduler-backed worker system.

---

## Safety Constraints

Remote bridge support is useful, but it widens the trust boundary.

Recommended practice:

- keep `allowHosts` narrow
- allow only the schemes you actually need
- use a dedicated bridge token
- prefer `https` when the bridge crosses an untrusted network
- keep the bridge itself behind network ACLs or a tunnel when possible

The orchestrator proxy is intentionally restricted:

- it proxies only to registered instance origins
- it does not accept arbitrary caller-controlled targets

That prevents the remote bridge feature from turning into a generic SSRF mechanism.

---

## Limitations

What this feature does not do:

- it does not launch bridge processes on remote machines
- it does not sync profile directories between hosts
- it does not migrate tabs or browser state across machines
- it does not discover workers automatically

The supported model is:

- start bridge remotely
- attach it explicitly
- route traffic through the orchestrator

---

## Hub-Only Mode

If you only want remote bridges and never want local Chrome, use the `no-instance` strategy:

```json
{
  "multiInstance": {
    "strategy": "no-instance"
  }
}
```

This blocks all local launch endpoints and starts the server as a pure hub. Remote bridges attach via `POST /instances/attach-bridge` and shorthand routes proxy to the first connected bridge.

---

## Summary

Use `POST /instances/attach-bridge` when you want:

- orchestrator on machine A
- bridge on machine B
- agents still talking only to machine A
- remote browser work without remote process-management complexity

Use `no-instance` strategy when you want a dedicated hub that never launches local Chrome.

This is the right feature when you want distributed execution with a single control plane.
