# Orchestration

Pinchtab is a Go-based server and bridge system for Chrome. It doesn't just "run" a browser; it manages a fleet of isolated instances, hardened against detection and optimized for automation.

## 1. Allocator Strategy

The foundation of every managed Chrome instance is the chromedp allocator. Pinchtab primarily uses local launch:

- **Managed Launch:** the server spawns a dedicated `pinchtab bridge` child process, which uses `NewExecAllocator` to launch Chrome with a dedicated profile and port.
- **Attach:** separately, the server can register an externally managed Chrome instance through the attach API when attach policy allows it.

## 2. Hardening & Launch Flags

When launching locally, Pinchtab applies a comprehensive set of command-line flags to strip away automation markers and optimize the environment:

- **Anti-Detection:** Flags like `--exclude-switches=enable-automation` and `--disable-infobars` are used to hide the standard "automated software" banner.
- **Isolation:** Each instance is assigned its own `UserDataDir` (Profile), ensuring that cookies, local storage, and sessions are completely isolated between different "agents" or profiles.
- **Stability:** Background throttling and renderer backgrounding are disabled to prevent Chrome from "sleeping" during long-running automation tasks.

## 3. Instance Orchestration

The orchestrator handles the lifecycle of multiple independent Chrome processes:

- **Process Isolation:** Each instance runs as a separate OS process with its own PID.
- **Health Monitoring:** After launching a process, the Orchestrator polls the instance's `/health` endpoint until it is ready to accept commands.
- **Port Management:** It ensures each instance is assigned a unique port and verifies availability before launching.

## 4. Pre-Flight Stealth Injection

The most critical part of the "wrap" happens before any website is even loaded. Pinchtab performs a Pre-Flight Injection:

- **`AddScriptToEvaluateOnNewDocument`:** The `stealth.js` script is registered to execute *before* any other script on a page. This masks `navigator.webdriver` and spoofs hardware identifiers (CPU cores, memory) before a website can fingerprint the browser.
- **Environment Spoofing:** Timezone overrides and locale settings are applied immediately after startup to ensure the browser's "identity" is consistent.

## 5. Resilience & Self-Healing

Pinchtab includes logic to handle common browser startup failures:

- **Lock File Cleanup:** If Chrome previously crashed, it might leave `SingletonLock` or `SingletonSocket` files that prevent it from restarting. Pinchtab automatically detects an "unclean exit" and deletes these locks.
- **Retry Logic:** If Chrome fails to start within the `chromeStartTimeout` (15s), Pinchtab will clear the session data and attempt one retry to ensure service availability.

## 6. Tab Management

Once the browser is running, the `TabManager` (`tab_manager.go`) tracks all open targets (tabs). It provides:

- **Context Lifecycle:** Manages the creation and cancellation of Go `context.Context` objects for each tab.
- **Setup Hooks:** Automatically reapplies stealth and optimization scripts to every new tab opened by the user or by automation scripts.
- **Tab Limits:** Enforces the configured max-tab policy to prevent runaway agents from consuming all memory.
- **Stale Tab Cleanup:** Periodically removes tabs that no longer exist in Chrome.
