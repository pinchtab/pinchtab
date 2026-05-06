import { afterEach, beforeEach, describe, it } from "node:test";
import assert from "node:assert";
import { executeBrowserAction } from "./tools/browser.ts";
import { executePinchtabAction } from "./tools/pinchtab.ts";
import { setLastTabId } from "./session.ts";

type FetchCall = { url: string; method: string; body?: any };

const originalFetch = globalThis.fetch;

function jsonResponse(body: unknown, init: ResponseInit = {}): Response {
  return new Response(JSON.stringify(body), {
    status: init.status ?? 200,
    headers: { "Content-Type": "application/json", ...(init.headers || {}) },
  });
}

describe("navigation routing", () => {
  let calls: FetchCall[] = [];

  beforeEach(() => {
    calls = [];
    setLastTabId(undefined);
    globalThis.fetch = (async (input: string | URL | Request, init?: RequestInit) => {
      const url = typeof input === "string" ? input : input instanceof URL ? input.toString() : input.url;
      const method = init?.method || "GET";
      const body = typeof init?.body === "string" ? JSON.parse(init.body) : undefined;
      calls.push({ url, method, body });

      const path = new URL(url).pathname;
      if (path === "/health") return jsonResponse({ ok: true, version: "test" });
      if (path === "/instances") return jsonResponse([{ id: "inst_a", status: "running" }]);
      if (path === "/navigate") return jsonResponse({ tabId: "tab_nav" });
      if (path === "/tab") return jsonResponse({ tabId: "tab_new" });
      if (path === "/tabs/tab_existing/navigate") return jsonResponse({ tabId: "tab_existing" });
      throw new Error(`unexpected fetch: ${path}`);
    }) as typeof fetch;
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
  });

  it("browser navigate reuses top-level /navigate when tabId is absent", async () => {
    const result = await executeBrowserAction({ baseUrl: "http://localhost:9867" }, { action: "navigate", url: "https://example.com" });
    assert.match(result.content[0].text, /tab_nav/);
    assert.ok(calls.some((call) => new URL(call.url).pathname === "/navigate"));
    assert.ok(!calls.some((call) => new URL(call.url).pathname === "/tab"));
  });

  it("browser navigate still uses /tab for explicit newTab", async () => {
    const result = await executeBrowserAction({ baseUrl: "http://localhost:9867" }, { action: "navigate", url: "https://example.com", newTab: true });
    assert.match(result.content[0].text, /tab_new/);
    assert.ok(calls.some((call) => new URL(call.url).pathname === "/tab"));
  });

  it("pinchtab navigate reuses top-level /navigate when tabId is absent", async () => {
    const result = await executePinchtabAction({ baseUrl: "http://localhost:9867" }, { action: "navigate", url: "https://example.com" });
    assert.match(result.content[0].text, /tab_nav/);
    assert.ok(calls.some((call) => new URL(call.url).pathname === "/navigate"));
    assert.ok(!calls.some((call) => new URL(call.url).pathname === "/tab"));
  });

  it("pinchtab navigate uses tab-scoped route when tabId is present", async () => {
    const result = await executePinchtabAction({ baseUrl: "http://localhost:9867", persistSessionTabs: false }, { action: "navigate", url: "https://example.com", tabId: "tab_existing" });
    assert.match(result.content[0].text, /tab_existing/);
    assert.ok(calls.some((call) => new URL(call.url).pathname === "/tabs/tab_existing/navigate"));
  });
});
