import { afterEach, beforeEach, describe, it } from "node:test";
import assert from "node:assert";
import { isRefToken, normalizeActionParams, looksLikeStaleRef, pinchtabFetch, textResult, imageResult, resourceResult, clearPinchtabSessionCache } from "./client.ts";

describe("isRefToken", () => {
  it("returns true for valid ref tokens", () => {
    assert.strictEqual(isRefToken("e5"), true);
    assert.strictEqual(isRefToken("e123"), true);
    assert.strictEqual(isRefToken("E5"), true);
    assert.strictEqual(isRefToken("e0"), true);
  });

  it("returns false for invalid ref tokens", () => {
    assert.strictEqual(isRefToken("e"), false);
    assert.strictEqual(isRefToken("5"), false);
    assert.strictEqual(isRefToken("ref5"), false);
    assert.strictEqual(isRefToken(""), false);
    assert.strictEqual(isRefToken(null), false);
    assert.strictEqual(isRefToken(undefined), false);
    assert.strictEqual(isRefToken(123), false);
  });

  it("handles whitespace", () => {
    assert.strictEqual(isRefToken(" e5 "), true);
    assert.strictEqual(isRefToken("e5\n"), true);
  });
});

describe("normalizeActionParams", () => {
  it("converts ref-like selector to ref", () => {
    const result = normalizeActionParams({ selector: "e5" });
    assert.strictEqual(result.ref, "e5");
    assert.strictEqual(result.selector, undefined);
  });

  it("preserves non-ref selector", () => {
    const result = normalizeActionParams({ selector: "#button" });
    assert.strictEqual(result.selector, "#button");
    assert.strictEqual(result.ref, undefined);
  });

  it("does not override existing ref", () => {
    const result = normalizeActionParams({ ref: "e10", selector: "e5" });
    assert.strictEqual(result.ref, "e10");
    assert.strictEqual(result.selector, "e5");
  });

  it("trims ref whitespace", () => {
    const result = normalizeActionParams({ ref: " e5 " });
    assert.strictEqual(result.ref, "e5");
  });

  it("preserves other params", () => {
    const result = normalizeActionParams({ action: "click", ref: "e5", text: "hello" });
    assert.strictEqual(result.action, "click");
    assert.strictEqual(result.ref, "e5");
    assert.strictEqual(result.text, "hello");
  });
});

describe("looksLikeStaleRef", () => {
  it("returns false for non-error results", () => {
    assert.strictEqual(looksLikeStaleRef({ success: true }), false);
    assert.strictEqual(looksLikeStaleRef(null), false);
    assert.strictEqual(looksLikeStaleRef(undefined), false);
  });

  it("returns true for stale ref errors", () => {
    assert.strictEqual(looksLikeStaleRef({ error: "stale element reference" }), true);
    assert.strictEqual(looksLikeStaleRef({ error: "ref not found" }), true);
    assert.strictEqual(looksLikeStaleRef({ error: "no node with id" }), true);
    assert.strictEqual(looksLikeStaleRef({ error: "unknown element" }), true);
    assert.strictEqual(looksLikeStaleRef({ error: "context canceled" }), true);
  });

  it("matches Pinchtab backend-node failures", () => {
    assert.strictEqual(
      looksLikeStaleRef({ error: "element not found in DOM (backendNodeId=42)" }),
      true,
    );
    assert.strictEqual(looksLikeStaleRef({ error: "backend-node-not-found" }), true);
    assert.strictEqual(looksLikeStaleRef({ error: "backend node detached" }), true);
    assert.strictEqual(looksLikeStaleRef({ error: "element no longer in DOM" }), true);
    assert.strictEqual(looksLikeStaleRef({ error: "element no longer attached" }), true);
  });

  it("returns false for other errors", () => {
    assert.strictEqual(looksLikeStaleRef({ error: "network timeout" }), false);
    assert.strictEqual(looksLikeStaleRef({ error: "server error" }), false);
  });

  it("does not match unrelated substrings containing 'ref'", () => {
    assert.strictEqual(looksLikeStaleRef({ error: "referrer policy violation" }), false);
    assert.strictEqual(looksLikeStaleRef({ error: "unsupported preferences" }), false);
    assert.strictEqual(looksLikeStaleRef({ error: "page not found" }), false);
  });

  it("checks body as well as error", () => {
    assert.strictEqual(looksLikeStaleRef({ error: "failed", body: "stale ref" }), true);
  });
});

describe("pinchtabFetch", () => {
  const originalFetch = globalThis.fetch;

  beforeEach(() => {
    globalThis.fetch = (async (_input: string | URL | Request, init?: RequestInit) => {
      return new Response(JSON.stringify({ headers: init?.headers ?? {} }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }) as typeof fetch;
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
  });

  it("forwards OpenClaw agent and session headers", async () => {
    const result = await pinchtabFetch(
      { baseUrl: "http://localhost:9867", token: "secret" },
      "/health",
      {},
      { agentId: "writer", sessionId: "session-123", sessionKey: "chat:writer" },
    );
    assert.deepStrictEqual(result.headers, {
      Authorization: "Bearer secret",
      "X-OpenClaw-Agent-Id": "writer",
      "X-OpenClaw-Session-Id": "session-123",
      "X-OpenClaw-Session-Key": "chat:writer",
    });
  });
});

describe("pinchtabFetch per-agent session token", () => {
  const originalFetch = globalThis.fetch;
  let calls: Array<{ url: string; method: string; headers: Record<string, string>; body?: string }>;

  beforeEach(() => {
    clearPinchtabSessionCache();
    calls = [];
    globalThis.fetch = (async (input: string | URL | Request, init?: RequestInit) => {
      const url = typeof input === "string" ? input : input.toString();
      const method = (init?.method || "GET").toUpperCase();
      const headers = (init?.headers ?? {}) as Record<string, string>;
      const body = typeof init?.body === "string" ? init.body : undefined;
      calls.push({ url, method, headers, body });
      if (url.endsWith("/sessions") && method === "POST") {
        return new Response(JSON.stringify({ id: "ses_abc", sessionToken: "ses_token_xyz" }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }
      return new Response(JSON.stringify({ ok: true, headers }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }) as typeof fetch;
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
    clearPinchtabSessionCache();
  });

  it("creates a Pinchtab session lazily and uses its token for browser actions", async () => {
    await pinchtabFetch(
      { baseUrl: "http://localhost:9867", token: "server-token" },
      "/snapshot",
      {},
      { agentId: "alpha" },
    );
    assert.strictEqual(calls.length, 2);
    assert.strictEqual(calls[0].url, "http://localhost:9867/sessions");
    assert.strictEqual(calls[0].method, "POST");
    assert.strictEqual(calls[0].headers.Authorization, "Bearer server-token");
    assert.deepStrictEqual(JSON.parse(calls[0].body!), { agentId: "alpha", label: "openclaw" });
    assert.strictEqual(calls[1].url, "http://localhost:9867/snapshot");
    assert.strictEqual(calls[1].headers.Authorization, "Session ses_token_xyz");
  });

  it("reuses the cached session token across calls for the same agent", async () => {
    const cfg = { baseUrl: "http://localhost:9867", token: "server-token" };
    await pinchtabFetch(cfg, "/snapshot", {}, { agentId: "alpha" });
    await pinchtabFetch(cfg, "/tabs", {}, { agentId: "alpha" });
    const sessionPosts = calls.filter((c) => c.url.endsWith("/sessions") && c.method === "POST");
    assert.strictEqual(sessionPosts.length, 1);
    const tabsCall = calls.find((c) => c.url.endsWith("/tabs"));
    assert.strictEqual(tabsCall?.headers.Authorization, "Session ses_token_xyz");
  });

  it("creates separate sessions for distinct agents", async () => {
    const cfg = { baseUrl: "http://localhost:9867", token: "server-token" };
    await pinchtabFetch(cfg, "/snapshot", {}, { agentId: "alpha" });
    await pinchtabFetch(cfg, "/snapshot", {}, { agentId: "beta" });
    const sessionPosts = calls.filter((c) => c.url.endsWith("/sessions") && c.method === "POST");
    assert.strictEqual(sessionPosts.length, 2);
    const agentIds = sessionPosts.map((c) => JSON.parse(c.body!).agentId).sort();
    assert.deepStrictEqual(agentIds, ["alpha", "beta"]);
  });

  it("uses the server token for /health, /sessions, /instances, /profiles", async () => {
    const cfg = { baseUrl: "http://localhost:9867", token: "server-token" };
    await pinchtabFetch(cfg, "/health", {}, { agentId: "alpha" });
    await pinchtabFetch(cfg, "/instances", {}, { agentId: "alpha" });
    await pinchtabFetch(cfg, "/profiles", {}, { agentId: "alpha" });
    for (const call of calls) {
      assert.strictEqual(call.headers.Authorization, "Bearer server-token");
    }
  });

  it("falls back to server token when context has no agentId", async () => {
    await pinchtabFetch(
      { baseUrl: "http://localhost:9867", token: "server-token" },
      "/snapshot",
      {},
    );
    assert.strictEqual(calls.length, 1);
    assert.strictEqual(calls[0].headers.Authorization, "Bearer server-token");
  });
});

describe("pinchtabFetch session failure modes", () => {
  const originalFetch = globalThis.fetch;

  afterEach(() => {
    globalThis.fetch = originalFetch;
    clearPinchtabSessionCache();
  });

  it("fails closed (no bearer fallback) when /sessions creation fails for an agent", async () => {
    clearPinchtabSessionCache();
    const calls: Array<{ url: string; method: string }> = [];
    globalThis.fetch = (async (input: string | URL | Request, init?: RequestInit) => {
      const url = typeof input === "string" ? input : input.toString();
      const method = (init?.method || "GET").toUpperCase();
      calls.push({ url, method });
      if (url.endsWith("/sessions") && method === "POST") {
        return new Response("upstream timeout", { status: 503 });
      }
      throw new Error("must not fall back to bearer for the snapshot path");
    }) as typeof fetch;

    const result = await pinchtabFetch(
      { baseUrl: "http://localhost:9867", token: "server-token" },
      "/snapshot",
      {},
      { agentId: "alpha" },
    );
    assert.strictEqual(calls.length, 1);
    assert.strictEqual(calls[0].url, "http://localhost:9867/sessions");
    assert.match(result.error, /Pinchtab session creation failed for agent alpha/);
    assert.match(result.error, /503/);
  });

  it("evicts cached token and retries once on 401 from a Session-scheme call", async () => {
    clearPinchtabSessionCache();
    const calls: Array<{ url: string; method: string; headers: Record<string, string> }> = [];
    let sessionTokenCounter = 0;
    let firstSnapshot = true;
    globalThis.fetch = (async (input: string | URL | Request, init?: RequestInit) => {
      const url = typeof input === "string" ? input : input.toString();
      const method = (init?.method || "GET").toUpperCase();
      const headers = (init?.headers ?? {}) as Record<string, string>;
      calls.push({ url, method, headers });
      if (url.endsWith("/sessions") && method === "POST") {
        sessionTokenCounter += 1;
        return new Response(
          JSON.stringify({ id: `ses_${sessionTokenCounter}`, sessionToken: `ses_token_${sessionTokenCounter}` }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        );
      }
      if (url.endsWith("/snapshot")) {
        if (firstSnapshot) {
          firstSnapshot = false;
          return new Response(JSON.stringify({ error: "bad_session" }), {
            status: 401,
            headers: { "Content-Type": "application/json" },
          });
        }
        return new Response(JSON.stringify({ ok: true }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }
      return new Response("", { status: 200 });
    }) as typeof fetch;

    const result = await pinchtabFetch(
      { baseUrl: "http://localhost:9867", token: "server-token" },
      "/snapshot",
      {},
      { agentId: "alpha" },
    );
    assert.deepStrictEqual(result, { ok: true });
    const sessionPosts = calls.filter((c) => c.url.endsWith("/sessions") && c.method === "POST");
    assert.strictEqual(sessionPosts.length, 2, "should re-create session after 401");
    const snapshotCalls = calls.filter((c) => c.url.endsWith("/snapshot"));
    assert.strictEqual(snapshotCalls.length, 2);
    assert.strictEqual(snapshotCalls[0].headers.Authorization, "Session ses_token_1");
    assert.strictEqual(snapshotCalls[1].headers.Authorization, "Session ses_token_2");
  });

  it("does not loop indefinitely if 401s keep coming after retry", async () => {
    clearPinchtabSessionCache();
    let sessionPosts = 0;
    let snapshotCalls = 0;
    globalThis.fetch = (async (input: string | URL | Request, init?: RequestInit) => {
      const url = typeof input === "string" ? input : input.toString();
      const method = (init?.method || "GET").toUpperCase();
      if (url.endsWith("/sessions") && method === "POST") {
        sessionPosts += 1;
        return new Response(JSON.stringify({ sessionToken: `ses_${sessionPosts}` }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }
      snapshotCalls += 1;
      return new Response("bad_session", { status: 401 });
    }) as typeof fetch;

    const result = await pinchtabFetch(
      { baseUrl: "http://localhost:9867", token: "server-token" },
      "/snapshot",
      {},
      { agentId: "alpha" },
    );
    assert.strictEqual(snapshotCalls, 2, "exactly one retry, no infinite loop");
    assert.strictEqual(result.error, "401 ");
  });
});

describe("textResult", () => {
  it("wraps string as text content", () => {
    const result = textResult("hello");
    assert.deepStrictEqual(result, { content: [{ type: "text", text: "hello" }] });
  });

  it("wraps object as JSON text", () => {
    const result = textResult({ key: "value" });
    assert.strictEqual(result.content[0].type, "text");
    assert.ok(result.content[0].text.includes('"key"'));
  });

  it("uses text property if present", () => {
    const result = textResult({ text: "extracted" });
    assert.deepStrictEqual(result, { content: [{ type: "text", text: "extracted" }] });
  });
});

describe("imageResult", () => {
  it("creates image content", () => {
    const result = imageResult("base64data", "image/png");
    assert.deepStrictEqual(result, {
      content: [{ type: "image", data: "base64data", mimeType: "image/png" }],
    });
  });
});

describe("resourceResult", () => {
  it("creates resource content", () => {
    const result = resourceResult("pdf://export", "application/pdf", "base64blob");
    assert.deepStrictEqual(result, {
      content: [{ type: "resource", resource: { uri: "pdf://export", mimeType: "application/pdf", blob: "base64blob" } }],
    });
  });
});
