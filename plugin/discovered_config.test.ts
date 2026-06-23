import { describe, it, beforeEach, afterEach } from "node:test";
import assert from "node:assert";
import { mkdtempSync, mkdirSync, rmSync, writeFileSync, utimesSync, statSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { resolveEffectiveConfig } from "./session.ts";

// discoverPinchtabConfig is module-private; it is exercised through the exported
// resolveEffectiveConfig, which only consults the cache when the passed cfg is
// missing baseUrl/token. The config path is hardcoded to
// join(homedir(), ".pinchtab", "config.json"), and os.homedir() honors
// process.env.HOME on POSIX, so we redirect HOME to a temp dir to install a real
// config file the cache will stat/read.
describe("discoverPinchtabConfig mtime cache", () => {
  let tmpHome: string;
  let configPath: string;
  let originalHome: string | undefined;

  // Force discovery: a cfg with both baseUrl AND token short-circuits before the cache.
  const emptyCfg = {} as Record<string, unknown>;

  function writeConfig(port: number, token: string): void {
    writeFileSync(
      configPath,
      JSON.stringify({ server: { bind: "127.0.0.1", port: String(port), token } }),
    );
  }

  beforeEach(() => {
    originalHome = process.env.HOME;
    tmpHome = mkdtempSync(join(tmpdir(), "pinchtab-disc-"));
    mkdirSync(join(tmpHome, ".pinchtab"), { recursive: true });
    configPath = join(tmpHome, ".pinchtab", "config.json");
    process.env.HOME = tmpHome;
  });

  afterEach(() => {
    if (originalHome === undefined) {
      delete process.env.HOME;
    } else {
      process.env.HOME = originalHome;
    }
    rmSync(tmpHome, { recursive: true, force: true });
  });

  it("serves an unchanged file (same mtime) from cache without re-parsing", async () => {
    writeConfig(1111, "tok-a");
    // Pin to an integer-second mtime so the value round-trips exactly through the
    // filesystem; the cache compares info.mtimeMs with ===, and sub-millisecond
    // fractions don't survive a utimesSync round-trip, which would spuriously bust
    // the cache and defeat the cache-hit assertion below.
    const pinned = 1_700_000_000; // seconds
    utimesSync(configPath, pinned, pinned);
    const pinnedMs = statSync(configPath).mtimeMs;

    const first = await resolveEffectiveConfig(emptyCfg);
    assert.strictEqual(first.baseUrl, "http://127.0.0.1:1111");
    assert.strictEqual(first.token, "tok-a");

    // Rewrite the CONTENTS while restoring the SAME (integer-second) mtime.
    writeConfig(2222, "tok-b");
    utimesSync(configPath, pinned, pinned);

    // Guard: confirm we actually pinned mtimeMs back to its original value.
    assert.strictEqual(
      statSync(configPath).mtimeMs,
      pinnedMs,
      "test setup: mtimeMs must be unchanged to exercise the cache-hit path",
    );

    // Cache key (mtimeMs) is unchanged, so the stale parsed value must be returned.
    const second = await resolveEffectiveConfig(emptyCfg);
    assert.strictEqual(second.baseUrl, "http://127.0.0.1:1111", "expected cached (stale) baseUrl");
    assert.strictEqual(second.token, "tok-a", "expected cached (stale) token");
  });

  it("re-parses when the file mtime changes", async () => {
    writeConfig(1111, "tok-a");
    const first = await resolveEffectiveConfig(emptyCfg);
    assert.strictEqual(first.baseUrl, "http://127.0.0.1:1111");

    // New contents AND a strictly newer mtime -> cache miss -> re-parse.
    writeConfig(3333, "tok-c");
    const future = new Date(Date.now() + 10_000);
    utimesSync(configPath, future, future);

    const second = await resolveEffectiveConfig(emptyCfg);
    assert.strictEqual(second.baseUrl, "http://127.0.0.1:3333", "expected re-parsed baseUrl");
    assert.strictEqual(second.token, "tok-c", "expected re-parsed token");
  });

  it("clears the cache when the file is removed (falls back to missing-config defaults)", async () => {
    writeConfig(1111, "tok-a");
    const first = await resolveEffectiveConfig(emptyCfg);
    assert.strictEqual(first.baseUrl, "http://127.0.0.1:1111");
    assert.strictEqual(first.token, "tok-a");

    rmSync(configPath, { force: true });

    // stat() throws -> catch clears the cache and discover returns null, so
    // resolveEffectiveConfig falls back to the built-in default and no token.
    const afterRemoval = await resolveEffectiveConfig(emptyCfg);
    assert.strictEqual(afterRemoval.baseUrl, "http://localhost:9867");
    assert.strictEqual(afterRemoval.token, undefined);

    // Re-create with fresh values: cache was cleared, so this must be re-parsed.
    writeConfig(4444, "tok-d");
    const afterRecreate = await resolveEffectiveConfig(emptyCfg);
    assert.strictEqual(afterRecreate.baseUrl, "http://127.0.0.1:4444");
    assert.strictEqual(afterRecreate.token, "tok-d");
  });
});
