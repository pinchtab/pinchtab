/**
 * Lifecycle Tests
 *
 * Covers the hardened start/stop lifecycle helpers:
 *   - isPinchtabHealthy: only an actual PinchTab `/health` ready shape passes.
 *   - stop(): a no-op when no process is running (still resolves, cleans up).
 *
 * The live SIGTERM→SIGKILL escalation path is exercised by integration.test.ts
 * when a binary is present.
 */
import { test, describe } from 'node:test';
import * as assert from 'node:assert';
import Pinchtab, { isPinchtabHealthy } from '../src/index';

describe('isPinchtabHealthy', () => {
  test('accepts a real PinchTab ready body', () => {
    assert.strictEqual(isPinchtabHealthy({ status: 'ok' }), true);
    assert.strictEqual(isPinchtabHealthy({ status: 'ok', tabs: 3 }), true);
  });

  test('rejects non-ready / non-PinchTab bodies', () => {
    assert.strictEqual(isPinchtabHealthy({ status: 'error' }), false);
    assert.strictEqual(isPinchtabHealthy({ status: 'draining' }), false);
    assert.strictEqual(isPinchtabHealthy({}), false);
    assert.strictEqual(isPinchtabHealthy(null), false);
    assert.strictEqual(isPinchtabHealthy(undefined), false);
    assert.strictEqual(isPinchtabHealthy(42), false);
    assert.strictEqual(isPinchtabHealthy('ok'), false);
  });
});

describe('stop()', () => {
  test('resolves and is a no-op when no process is running', async () => {
    const client = new Pinchtab({ port: 9999 });
    await assert.doesNotReject(() => client.stop());
    // Idempotent: a second stop is still safe.
    await assert.doesNotReject(() => client.stop());
  });
});
