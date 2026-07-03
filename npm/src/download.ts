import * as fs from 'fs';
import * as path from 'path';
import * as http from 'http';
import * as https from 'https';
import * as crypto from 'crypto';
import { detectPlatform, getBinaryName, getBinaryPath, readPackageVersion } from './platform';

const GITHUB_REPO = 'pinchtab/pinchtab';

// Resolve the published package version even when compiled code lives under dist/src.
function getVersion(): string {
  return readPackageVersion(__dirname);
}

// NOTE: HTTPS_PROXY / HTTP_PROXY are NOT honored — downloads go direct. A prior
// buildProxyAgent() built an https.Agent({host,port}) that https.get(url, …)
// silently overrode (the Agent's host/port were never used for tunneling), so
// the proxy support was a no-op. It was removed rather than left as a misleading
// stub; if real proxy tunneling is needed, route through an https-proxy-agent.

// httpGetFollowingRedirects performs a GET that follows 301/302/307/308
// redirects, then hands the final 200 response stream to onResponse. All
// transport/HTTP errors go to onError. Shared by the checksum (buffered) and
// binary (streamed-to-file) download paths so redirect behavior cannot drift
// between them. `requestFn` defaults to https.get; it is injectable so tests can
// drive the real logic against a local http server without TLS/network.
export function httpGetFollowingRedirects(
  url: string,
  maxRedirects: number,
  onResponse: (response: http.IncomingMessage) => void,
  onError: (err: Error) => void,
  requestFn: (
    url: string,
    options: http.RequestOptions,
    callback: (res: http.IncomingMessage) => void
  ) => http.ClientRequest = https.get
): void {
  const attempt = (currentUrl: string, redirectsRemaining: number) => {
    const request = requestFn(currentUrl, {}, (response) => {
      // Handle redirects (301, 302, 307, 308)
      if ([301, 302, 307, 308].includes(response.statusCode || 0)) {
        if (redirectsRemaining <= 0) {
          onError(new Error(`Too many redirects from ${currentUrl}`));
          return;
        }

        let redirectUrl = response.headers.location;
        if (!redirectUrl) {
          onError(new Error(`Redirect without location header from ${currentUrl}`));
          return;
        }

        // Resolve relative URLs
        try {
          redirectUrl = new URL(redirectUrl, currentUrl).toString();
        } catch (_err) {
          onError(new Error(`Invalid redirect URL from ${currentUrl}: ${redirectUrl}`));
          return;
        }

        // Consume the response stream to avoid memory leaks
        response.resume();
        attempt(redirectUrl, redirectsRemaining - 1);
        return;
      }

      if (response.statusCode === 404) {
        onError(new Error(`Not found: ${currentUrl}`));
        return;
      }

      if (response.statusCode !== 200) {
        onError(new Error(`HTTP ${response.statusCode}: ${currentUrl}`));
        return;
      }

      onResponse(response);
    });

    request.on('error', onError);
  };

  attempt(url, maxRedirects);
}

function fetchUrl(url: string, maxRedirects = 5): Promise<Buffer> {
  return new Promise((resolve, reject) => {
    httpGetFollowingRedirects(
      url,
      maxRedirects,
      (response) => {
        const chunks: Buffer[] = [];
        response.on('data', (chunk) => chunks.push(chunk));
        response.on('end', () => resolve(Buffer.concat(chunks)));
        response.on('error', reject);
      },
      reject
    );
  });
}

async function downloadChecksums(version: string): Promise<Map<string, string>> {
  const url = `https://github.com/${GITHUB_REPO}/releases/download/v${version}/checksums.txt`;

  try {
    const data = await fetchUrl(url);
    const checksums = new Map<string, string>();

    data
      .toString('utf-8')
      .trim()
      .split('\n')
      .forEach((line) => {
        const [hash, filename] = line.split(/\s+/);
        if (hash && filename) {
          checksums.set(filename.trim(), hash.trim());
        }
      });

    return checksums;
  } catch (err) {
    throw new Error(
      `Failed to download checksums: ${(err as Error).message}. ` +
        `Ensure v${version} is released on GitHub with checksums.txt`
    );
  }
}

// verifySHA256 streams the file through the hash so large binaries are never
// buffered whole in memory (readFileSync loaded the entire binary at once).
function verifySHA256(filePath: string, expectedHash: string): Promise<boolean> {
  return new Promise((resolve, reject) => {
    const hash = crypto.createHash('sha256');
    const stream = fs.createReadStream(filePath);
    stream.on('error', reject);
    stream.on('data', (chunk) => hash.update(chunk));
    stream.on('end', () => {
      resolve(hash.digest('hex').toLowerCase() === expectedHash.toLowerCase());
    });
  });
}

async function downloadBinary(
  platform: ReturnType<typeof detectPlatform>,
  version: string
): Promise<void> {
  const binaryName = getBinaryName(platform);
  const binaryPath = getBinaryPath(binaryName, version);

  // Fetch the checksum map once and reuse it for both the existing-binary
  // verification and the post-download check (it was previously downloaded
  // twice per install).
  const checksums = await downloadChecksums(version);
  if (!checksums.has(binaryName)) {
    throw new Error(
      `Binary not found in checksums: ${binaryName}. ` +
        `Available: ${Array.from(checksums.keys()).join(', ')}`
    );
  }
  const expectedHash = checksums.get(binaryName)!;

  // Always verify existing binaries, even if they exist
  // (guards against corrupted installs from previous failures)
  if (fs.existsSync(binaryPath)) {
    if (await verifySHA256(binaryPath, expectedHash)) {
      console.log(`✓ Pinchtab binary verified: ${binaryPath}`);
      return;
    }
    console.warn(`⚠ Existing binary failed checksum, re-downloading...`);
    try {
      fs.unlinkSync(binaryPath);
    } catch {
      // ignore
    }
  }

  console.log(`Downloading Pinchtab ${version} for ${platform.os}-${platform.arch}...`);
  const downloadUrl = `https://github.com/${GITHUB_REPO}/releases/download/v${version}/${binaryName}`;

  // Ensure the managed install directory exists
  const binDir = path.dirname(binaryPath);
  if (!fs.existsSync(binDir)) {
    fs.mkdirSync(binDir, { recursive: true });
  }

  // Download to temp file first, then atomically rename to final path
  // This prevents partial/corrupted files from being left behind
  const tempPath = `${binaryPath}.tmp`;

  return new Promise((resolve, reject) => {
    console.log(`Downloading from ${downloadUrl}...`);

    const file = fs.createWriteStream(tempPath);

    // Any failure must drop the partial temp file before rejecting.
    const failWithCleanup = (err: Error) => {
      fs.unlink(tempPath, () => {});
      reject(err);
    };

    httpGetFollowingRedirects(
      downloadUrl,
      5,
      (response) => {
        response.pipe(file);

        file.on('finish', () => {
          file.close();

          // Verify checksum before moving to final location
          verifySHA256(tempPath, expectedHash)
            .then((ok) => {
              if (!ok) {
                failWithCleanup(
                  new Error(
                    `Checksum verification failed for ${binaryName}. ` +
                      `Download may be corrupted. Please try again.`
                  )
                );
                return;
              }

              // Atomically move temp file to final location
              try {
                fs.renameSync(tempPath, binaryPath);
              } catch (err) {
                failWithCleanup(new Error(`Failed to finalize binary: ${(err as Error).message}`));
                return;
              }

              // Make executable
              try {
                fs.chmodSync(binaryPath, 0o755);
              } catch (err) {
                // On Windows, chmod may fail but binary may still be usable
                console.warn(
                  `⚠ Warning: could not set executable permissions: ${(err as Error).message}`
                );
              }

              console.log(`✓ Verified and installed: ${binaryPath}`);
              resolve();
            })
            .catch(failWithCleanup);
        });

        file.on('error', failWithCleanup);
      },
      failWithCleanup
    );
  });
}

export async function ensureBinary(): Promise<string> {
  const platform = detectPlatform();
  const version = getVersion();

  await downloadBinary(platform, version);

  const binaryName = getBinaryName(platform);
  return getBinaryPath(binaryName, version);
}
