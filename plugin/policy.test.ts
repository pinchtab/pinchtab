import { describe, it } from "node:test";
import assert from "node:assert";
import { matchesDomain, checkNavigationPolicy, checkEvaluatePolicy, checkDownloadPolicy, checkUploadPolicy } from "./policy.ts";

describe("matchesDomain", () => {
  it("returns true for empty patterns", () => {
    assert.strictEqual(matchesDomain("https://example.com", []), true);
  });

  it("matches exact domain", () => {
    assert.strictEqual(matchesDomain("https://example.com/path", ["example.com"]), true);
    assert.strictEqual(matchesDomain("https://other.com/path", ["example.com"]), false);
  });

  it("matches wildcard subdomain", () => {
    assert.strictEqual(matchesDomain("https://sub.example.com", ["*.example.com"]), true);
    assert.strictEqual(matchesDomain("https://deep.sub.example.com", ["*.example.com"]), true);
    assert.strictEqual(matchesDomain("https://example.com", ["*.example.com"]), true);
    assert.strictEqual(matchesDomain("https://other.com", ["*.example.com"]), false);
  });

  it("handles invalid URLs", () => {
    assert.strictEqual(matchesDomain("not-a-url", ["example.com"]), false);
  });

  it("matches multiple patterns", () => {
    const patterns = ["example.com", "*.test.com"];
    assert.strictEqual(matchesDomain("https://example.com", patterns), true);
    assert.strictEqual(matchesDomain("https://sub.test.com", patterns), true);
    assert.strictEqual(matchesDomain("https://other.com", patterns), false);
  });
});

describe("checkNavigationPolicy", () => {
  it("allows navigation when no domains configured", () => {
    const result = checkNavigationPolicy({}, "https://any.com");
    assert.strictEqual(result.allowed, true);
  });

  it("allows navigation when URL matches allowedDomains", () => {
    const cfg = { allowedDomains: ["example.com"] };
    const result = checkNavigationPolicy(cfg, "https://example.com/page");
    assert.strictEqual(result.allowed, true);
  });

  it("blocks navigation when URL not in allowedDomains", () => {
    const cfg = { allowedDomains: ["example.com"] };
    const result = checkNavigationPolicy(cfg, "https://blocked.com");
    assert.strictEqual(result.allowed, false);
    assert.ok(result.error);
    const blockedUrl = new URL("https://blocked.com");
    assert.ok(result.error.includes(blockedUrl.hostname));
  });

  it("allows when no URL provided", () => {
    const cfg = { allowedDomains: ["example.com"] };
    const result = checkNavigationPolicy(cfg, undefined);
    assert.strictEqual(result.allowed, true);
  });
});

describe("checkEvaluatePolicy", () => {
  it("blocks evaluate by default", () => {
    const result = checkEvaluatePolicy({});
    assert.strictEqual(result.allowed, false);
  });

  it("blocks evaluate when explicitly false", () => {
    const result = checkEvaluatePolicy({ allowEvaluate: false });
    assert.strictEqual(result.allowed, false);
  });

  it("allows evaluate when explicitly true", () => {
    const result = checkEvaluatePolicy({ allowEvaluate: true });
    assert.strictEqual(result.allowed, true);
  });
});

describe("checkDownloadPolicy", () => {
  it("blocks downloads by default", () => {
    const result = checkDownloadPolicy({});
    assert.strictEqual(result.allowed, false);
  });

  it("allows downloads when explicitly true", () => {
    const result = checkDownloadPolicy({ allowDownloads: true });
    assert.strictEqual(result.allowed, true);
  });
});

describe("checkUploadPolicy", () => {
  it("blocks uploads by default", () => {
    const result = checkUploadPolicy({});
    assert.strictEqual(result.allowed, false);
  });

  it("allows uploads when explicitly true", () => {
    const result = checkUploadPolicy({ allowUploads: true });
    assert.strictEqual(result.allowed, true);
  });
});
