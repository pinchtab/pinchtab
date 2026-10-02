import { afterEach, beforeEach, describe, it } from "node:test";
import assert from "node:assert";
import { executeBrowserAction } from "./tools/browser.ts";
import { setLastTabId } from "./session.ts";

const originalFetch = globalThis.fetch;

describe("browser screenshot format", () => {
  let screenshotQueries: URLSearchParams[] = [];

  beforeEach(() => {
    screenshotQueries = [];
    setLastTabId(undefined);
    globalThis.fetch = (async (input: string | URL | Request) => {
      const url = new URL(typeof input === "string" ? input : input instanceof URL ? input.toString() : input.url);
      if (url.pathname === "/health") return Response.json({ ok: true, version: "test" });
      if (url.pathname === "/instances") return Response.json([{ id: "inst_a", status: "running" }]);
      if (url.pathname === "/tabs") return Response.json({ tabs: [] });
      if (url.pathname === "/screenshot") {
        screenshotQueries.push(url.searchParams);
        return new Response(new Uint8Array([1, 2, 3]));
      }
      throw new Error(`unexpected fetch: ${url.pathname}`);
    }) as typeof fetch;
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
  });

  it("requests png when screenshotFormat is png", async () => {
    const result = await executeBrowserAction({ baseUrl: "http://localhost:9867" }, { action: "screenshot", screenshotFormat: "png" });
    assert.strictEqual(screenshotQueries[0].get("format"), "png");
    assert.strictEqual(result.content[0].mimeType, "image/png");
  });

  it("defaults to jpeg without a screenshotFormat", async () => {
    const result = await executeBrowserAction({ baseUrl: "http://localhost:9867" }, { action: "screenshot" });
    assert.strictEqual(screenshotQueries[0].get("format"), null);
    assert.strictEqual(result.content[0].mimeType, "image/jpeg");
  });

  it("falls back to the configured screenshotFormat", async () => {
    await executeBrowserAction({ baseUrl: "http://localhost:9867", screenshotFormat: "png" }, { action: "screenshot" });
    assert.strictEqual(screenshotQueries[0].get("format"), "png");
  });

  it("an explicit jpeg overrides a png config", async () => {
    const result = await executeBrowserAction({ baseUrl: "http://localhost:9867", screenshotFormat: "png" }, { action: "screenshot", screenshotFormat: "jpeg" });
    assert.strictEqual(screenshotQueries[0].get("format"), null);
    assert.strictEqual(result.content[0].mimeType, "image/jpeg");
  });
});
