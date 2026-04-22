import { describe, it } from "node:test";
import assert from "node:assert";
import { isRefToken, normalizeActionParams, looksLikeStaleRef, textResult, imageResult, resourceResult } from "./client.ts";

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

  it("returns false for other errors", () => {
    assert.strictEqual(looksLikeStaleRef({ error: "network timeout" }), false);
    assert.strictEqual(looksLikeStaleRef({ error: "server error" }), false);
  });

  it("checks body as well as error", () => {
    assert.strictEqual(looksLikeStaleRef({ error: "failed", body: "stale ref" }), true);
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
