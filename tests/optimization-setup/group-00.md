# Group 0: Auto-Start & First-Use Validation

A fresh user is told to "install PinchTab and run `pinchtab nav <url>`" — the CLI auto-creates a config and auto-starts the server. These steps prove that documented flow works end to end on a clean machine.

**Do not** run `./pinchtab config init` or `./pinchtab server` manually. The whole point of group 0 is to verify the auto-flow handles them. Likewise do not create a session — the CLI uses the token from the auto-created config.

The setup section in `subagent-context.md` already exported `PINCHTAB_CONFIG=~/.pinchtab/setup-config-<timestamp>.json` to a non-existent path. Step 0.1 is the very first command you run against the binary.

### 0.1 Cold nav auto-creates config and starts server
Run a single navigation against the fixture server with no preceding `pinchtab` calls:

```bash
./pinchtab nav http://localhost:$FIXTURE_PORT/ --print-tab-id
```

**Verify**:
- The command exits 0 and prints a tab ID. Capture it (`TAB_ID=...`) and reuse for every step in group 1.
- `$PINCHTAB_CONFIG` now exists on disk — auto-created by the CLI.
- A `pinchtab server` process is running on default port `9867` (`lsof -i :9867` or check `ps`).
- A managed Chrome instance has come up (`./pinchtab instances` shows at least one row, `status=running`).

### 0.2 Auto-created config matches OOTB defaults
Without ever running `config init` manually, inspect what the auto-flow wrote:

```bash
./pinchtab config validate
./pinchtab config get server.port
./pinchtab config get instanceDefaults.mode
./pinchtab config get security.allowEvaluate
./pinchtab config get security.allowedDomains
```

**Verify**:
- `config validate` reports the file at `$PINCHTAB_CONFIG` is valid.
- `server.port` → `9867`
- `instanceDefaults.mode` → `headless`
- `security.allowEvaluate` → `false`
- `security.allowedDomains` → contains `localhost` (or `127.0.0.1`)

If any of these are different, the auto-flow didn't produce true OOTB defaults — flag it.

### 0.3 Server is reachable on default port
The auto-started server should respond to a plain health check.

```bash
./pinchtab health
```

**Verify**: `status: ok`. The configured token from the auto-created config was used implicitly — no env override, no session.

### 0.4 Auth is enforced
Override the token to a deliberately wrong value:

```bash
PINCHTAB_TOKEN=wrong-token ./pinchtab health
```

**Verify**: Exits non-zero with HTTP 401 / `unauthorized`.

### 0.5 IDPI rejects non-local URLs OOTB
The README explicitly warns that the OOTB security posture restricts browsing to local domains. Test it.

```bash
./pinchtab nav https://example.com
```

**Verify**: The command fails with `idpi_domain_blocked` (or similar) referring to `example.com` not being in the allowed-domains list. This is correct OOTB behavior — a fresh agent should know it has to add domains explicitly before navigating to the public web. **Do not** run `./pinchtab config set` to add `example.com` here; the rejection itself is the test.

### 0.6 Tab list shows the navigated tab
The tab from 0.1 should still be listed.

```bash
./pinchtab tab
```

**Verify**: `$TAB_ID` from 0.1 is in the list, with title `PinchTab Benchmark - Home` and URL `http://localhost:$FIXTURE_PORT/`. Reuse this tab ID for every step in group 1.

---
