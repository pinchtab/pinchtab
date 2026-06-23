import { describe, it, beforeEach, afterEach } from "node:test";
import assert from "node:assert";
import { getLastTabId, setLastTabId, resolveProfile, isLocalHost, formatDiscoveredBaseUrl, ensureReady, invalidateReadiness } from "./session.ts";

describe("tab session state", () => {
  beforeEach(() => {
    setLastTabId(undefined);
    setLastTabId(undefined, { agentId: "main" });
    setLastTabId(undefined, { agentId: "writer" });
  });

  it("starts with undefined tabId", () => {
    assert.strictEqual(getLastTabId(), undefined);
  });

  it("stores and retrieves tabId", () => {
    setLastTabId("tab123");
    assert.strictEqual(getLastTabId(), "tab123");
  });

  it("keeps tab state isolated per agent", () => {
    setLastTabId("main-tab", { agentId: "main", sessionId: "s1" });
    setLastTabId("writer-tab", { agentId: "writer", sessionId: "s2" });
    assert.strictEqual(getLastTabId({ agentId: "main" }), "main-tab");
    assert.strictEqual(getLastTabId({ agentId: "writer" }), "writer-tab");
    assert.strictEqual(getLastTabId(), undefined);
  });

  it("can clear tabId", () => {
    setLastTabId("tab123");
    setLastTabId(undefined);
    assert.strictEqual(getLastTabId(), undefined);
  });
});

describe("resolveProfile", () => {
  it("returns empty object for default openclaw profile", () => {
    const result = resolveProfile({}, undefined);
    assert.deepStrictEqual(result, {});
  });

  it("returns attach:true for user profile", () => {
    const result = resolveProfile({}, "user");
    assert.deepStrictEqual(result, { attach: true });
  });

  it("uses config defaultProfile", () => {
    const cfg = { defaultProfile: "user" };
    const result = resolveProfile(cfg, undefined);
    assert.deepStrictEqual(result, { attach: true });
  });

  it("returns custom profile config", () => {
    const cfg = {
      profiles: {
        staging: { instanceId: "staging-instance" },
        custom: { instanceId: "custom-id", attach: true },
      },
    };
    assert.deepStrictEqual(resolveProfile(cfg, "staging"), { instanceId: "staging-instance" });
    assert.deepStrictEqual(resolveProfile(cfg, "custom"), { instanceId: "custom-id", attach: true });
  });

  it("falls back to builtin for unknown profile", () => {
    const cfg = { profiles: { staging: { instanceId: "staging" } } };
    const result = resolveProfile(cfg, "unknown");
    assert.deepStrictEqual(result, {});
  });

  it("profile param overrides defaultProfile", () => {
    const cfg = { defaultProfile: "user", profiles: { staging: { instanceId: "s1" } } };
    const result = resolveProfile(cfg, "staging");
    assert.deepStrictEqual(result, { instanceId: "s1" });
  });
});

describe("isLocalHost", () => {
  it("returns true for localhost", () => {
    assert.strictEqual(isLocalHost("http://localhost:9867"), true);
    assert.strictEqual(isLocalHost("http://localhost"), true);
    assert.strictEqual(isLocalHost("http://LOCALHOST:9867"), true);
  });

  it("returns true for 127.0.0.1", () => {
    assert.strictEqual(isLocalHost("http://127.0.0.1:9867"), true);
    assert.strictEqual(isLocalHost("http://127.0.0.1"), true);
  });

  it("returns true for IPv6 localhost", () => {
    assert.strictEqual(isLocalHost("http://[::1]:9867"), true);
    assert.strictEqual(isLocalHost("http://[::1]"), true);
  });

  it("returns false for remote hosts", () => {
    assert.strictEqual(isLocalHost("http://example.com"), false);
    assert.strictEqual(isLocalHost("http://192.168.1.1:9867"), false);
    assert.strictEqual(isLocalHost("http://pinchtab.local:9867"), false);
    assert.strictEqual(isLocalHost("https://api.pinchtab.com"), false);
  });

  it("returns false for invalid URLs", () => {
    assert.strictEqual(isLocalHost("not-a-url"), false);
    assert.strictEqual(isLocalHost(""), false);
  });
});


describe("formatDiscoveredBaseUrl", () => {
  it("brackets IPv6 loopback binds", () => {
    assert.strictEqual(formatDiscoveredBaseUrl("::1", 9867), "http://[::1]:9867");
  });

  it("normalizes wildcard binds to local loopback", () => {
    assert.strictEqual(formatDiscoveredBaseUrl("0.0.0.0", 9867), "http://127.0.0.1:9867");
    assert.strictEqual(formatDiscoveredBaseUrl("::", 9867), "http://[::1]:9867");
  });
});

describe("readiness cache", () => {
  const cfg = { baseUrl: "http://localhost:9999", token: "t" };
  let originalFetch: typeof globalThis.fetch;
  let healthCalls = 0;
  let instancesCalls = 0;

  beforeEach(() => {
    invalidateReadiness(cfg);
    healthCalls = 0;
    instancesCalls = 0;
    originalFetch = globalThis.fetch;
    globalThis.fetch = (async (url: any) => {
      const u = String(url);
      if (u.includes("/instances")) {
        instancesCalls++;
        return new Response(JSON.stringify([{ id: "i1", status: "running" }]), {
          status: 200,
          headers: { "content-type": "application/json" },
        });
      }
      if (u.includes("/health")) {
        healthCalls++;
        return new Response(JSON.stringify({ status: "ok" }), {
          status: 200,
          headers: { "content-type": "application/json" },
        });
      }
      return new Response("{}", { status: 200, headers: { "content-type": "application/json" } });
    }) as any;
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
    invalidateReadiness(cfg);
  });

  it("probes on the first call and serves subsequent ones from cache", async () => {
    const r1 = await ensureReady(cfg);
    assert.strictEqual(r1.ok, true);
    assert.ok(healthCalls >= 1, "first call probes /health");
    assert.ok(instancesCalls >= 1, "first call probes /instances");

    const healthAfterFirst = healthCalls;
    const instancesAfterFirst = instancesCalls;

    const r2 = await ensureReady(cfg);
    assert.strictEqual(r2.ok, true);
    assert.strictEqual(healthCalls, healthAfterFirst, "cached: no extra /health probe");
    assert.strictEqual(instancesCalls, instancesAfterFirst, "cached: no extra /instances probe");
  });

  it("re-probes after the latch is invalidated", async () => {
    await ensureReady(cfg);
    const healthAfterFirst = healthCalls;

    invalidateReadiness(cfg);

    await ensureReady(cfg);
    assert.ok(healthCalls > healthAfterFirst, "re-probes after invalidation");
  });
});
