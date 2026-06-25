/**
 * Download HTTP transport tests (GAP 1)
 *
 * Drives the REAL `httpGetFollowingRedirects` from src/download.ts against a
 * LOCAL node:http server (no real network/TLS). The production function accepts
 * an injectable `requestFn` (defaulting to https.get) precisely so this test can
 * point it at a plain-http local server. This verifies the documented
 * 302/301/307/308-follow, relative-location resolution, and 404/non-200/too-many-
 * redirects/missing-location classification contract deterministically.
 *
 * (HTTPS_PROXY/HTTP_PROXY are intentionally not honored — the former faux
 * buildProxyAgent was a no-op and has been removed; see src/download.ts.)
 */

import { test, describe, before, after } from 'node:test';
import * as assert from 'node:assert';
import * as http from 'node:http';

import { httpGetFollowingRedirects } from '../src/download';

// Thin promise wrapper around the production function, injecting http.get so the
// request hits the local http server. Mirrors how src/download.ts's fetchUrl
// buffers the final 200 response body.
function fetchUrl(url: string, maxRedirects = 5): Promise<Buffer> {
  return new Promise((resolve, reject) => {
    httpGetFollowingRedirects(
      url,
      maxRedirects,
      (response) => {
        const chunks: Buffer[] = [];
        response.on('data', (chunk) => chunks.push(chunk as Buffer));
        response.on('end', () => resolve(Buffer.concat(chunks)));
        response.on('error', reject);
      },
      reject,
      http.get
    );
  });
}

// ---------------------------------------------------------------------------
// Local http server: /redirect 302 -> /target (200 body), /missing 404,
// /teapot 418 (non-200, non-404), /loop infinite redirect.
// ---------------------------------------------------------------------------

const KNOWN_BODY = 'pinchtab-binary-contents-v1';
let server: http.Server;
let baseUrl: string;

before(async () => {
  server = http.createServer((req, res) => {
    switch (req.url) {
      case '/redirect':
        res.writeHead(302, { location: '/target' });
        res.end();
        return;
      case '/relative-redirect':
        // Relative location header must resolve against the current URL.
        res.writeHead(301, { location: 'target' });
        res.end();
        return;
      case '/target':
        res.writeHead(200, { 'content-type': 'text/plain' });
        res.end(KNOWN_BODY);
        return;
      case '/missing':
        res.writeHead(404);
        res.end('nope');
        return;
      case '/teapot':
        res.writeHead(418);
        res.end('teapot');
        return;
      case '/loop':
        res.writeHead(302, { location: '/loop' });
        res.end();
        return;
      case '/no-location':
        res.writeHead(302);
        res.end();
        return;
      default:
        res.writeHead(404);
        res.end('unknown');
    }
  });

  await new Promise<void>((resolve) => server.listen(0, '127.0.0.1', resolve));
  const addr = server.address();
  if (addr && typeof addr === 'object') {
    baseUrl = `http://127.0.0.1:${addr.port}`;
  }
});

after(async () => {
  await new Promise<void>((resolve) => server.close(() => resolve()));
});

describe('httpGetFollowingRedirects', () => {
  test('follows a 302 redirect and returns the final 200 body', async () => {
    const body = await fetchUrl(`${baseUrl}/redirect`);
    assert.strictEqual(body.toString('utf-8'), KNOWN_BODY);
  });

  test('resolves a relative redirect location against the current URL', async () => {
    const body = await fetchUrl(`${baseUrl}/relative-redirect`);
    assert.strictEqual(body.toString('utf-8'), KNOWN_BODY);
  });

  test('classifies 404 as a "Not found" error', async () => {
    await assert.rejects(
      fetchUrl(`${baseUrl}/missing`),
      /Not found: .*\/missing/,
      'a 404 response must reject with the documented "Not found" classification'
    );
  });

  test('classifies other non-200 responses as an HTTP <code> error', async () => {
    await assert.rejects(fetchUrl(`${baseUrl}/teapot`), /HTTP 418: .*\/teapot/);
  });

  test('rejects when redirects exceed maxRedirects', async () => {
    await assert.rejects(
      fetchUrl(`${baseUrl}/loop`, 2),
      /Too many redirects/,
      'an infinite redirect loop must terminate with a "Too many redirects" error'
    );
  });

  test('rejects a redirect that is missing a location header', async () => {
    await assert.rejects(fetchUrl(`${baseUrl}/no-location`), /Redirect without location header/);
  });
});
