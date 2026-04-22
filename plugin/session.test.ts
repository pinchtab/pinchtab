import { describe, it, beforeEach } from "node:test";
import assert from "node:assert";
import { getLastTabId, setLastTabId, resolveProfile } from "./session.ts";

describe("tab session state", () => {
  beforeEach(() => {
    setLastTabId(undefined);
  });

  it("starts with undefined tabId", () => {
    assert.strictEqual(getLastTabId(), undefined);
  });

  it("stores and retrieves tabId", () => {
    setLastTabId("tab123");
    assert.strictEqual(getLastTabId(), "tab123");
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
