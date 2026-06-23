import { spawn, ChildProcess } from 'child_process';
import { randomBytes } from 'crypto';
import * as path from 'path';
import * as fs from 'fs';
import * as os from 'os';
import {
  detectPlatform,
  getBinaryName,
  getBinaryPath,
  getCheckoutBinaryPath,
  readPackageVersion,
} from './platform';
import { withAuthedFetch } from './http';
import {
  SnapshotParams,
  SnapshotResponse,
  TabClickParams,
  TabLockParams,
  TabUnlockParams,
  CreateTabParams,
  CreateTabResponse,
  PinchtabOptions,
} from './types';

export * from './types';
export * from './platform';

/**
 * Returns true only for an actual PinchTab `/health` ready body (`{ status: "ok" }`),
 * so an unrelated process listening on the port cannot satisfy startup.
 */
export function isPinchtabHealthy(body: unknown): boolean {
  return (
    typeof body === 'object' && body !== null && (body as { status?: unknown }).status === 'ok'
  );
}

export class Pinchtab {
  private baseUrl: string;
  private timeout: number;
  private port: number;
  private process: ChildProcess | null = null;
  private binaryPath: string | null = null;
  private tempConfigDir: string | null = null;
  private token: string | null;
  private readonly configuredToken: string | null;
  private readonly shutdownTimeout: number;

  constructor(options: PinchtabOptions = {}) {
    this.port = options.port || 9867;
    this.baseUrl = options.baseUrl || `http://localhost:${this.port}`;
    this.timeout = options.timeout || 30000;
    this.configuredToken = options.token?.trim() || null;
    this.token = this.configuredToken;
    this.shutdownTimeout = options.shutdownTimeout ?? 10000;
  }

  /**
   * Start the Pinchtab server process
   */
  async start(binaryPath?: string): Promise<void> {
    if (this.process) {
      throw new Error('Pinchtab process already running');
    }

    if (!binaryPath) {
      binaryPath = await this.getBinaryPathInternal();
    }

    this.binaryPath = binaryPath;
    const tempConfigPath = this.createTempConfig();

    return new Promise((resolve, reject) => {
      let settled = false;
      const fail = (message: string) => {
        if (settled) {
          return;
        }
        settled = true;
        this.cleanupTempConfig();
        reject(new Error(message));
      };

      this.process = spawn(binaryPath, ['server'], {
        stdio: 'inherit',
        env: {
          ...process.env,
          PINCHTAB_CONFIG: tempConfigPath,
        },
      });

      this.process.on('error', (err) => {
        fail(`Failed to start Pinchtab: ${err.message}`);
      });

      this.process.on('exit', (code, signal) => {
        this.cleanupTempConfig();
        if (!settled) {
          const reason = signal ? `signal ${signal}` : `exit code ${code ?? 0}`;
          reject(new Error(`Pinchtab exited before becoming ready (${reason})`));
        }
      });

      void this.waitForServerReady()
        .then(() => {
          if (settled) {
            return;
          }
          settled = true;
          resolve();
        })
        .catch((err: Error) => {
          if (this.process) {
            this.process.kill();
            this.process = null;
          }
          fail(err.message);
        });
    });
  }

  /**
   * Stop the Pinchtab server process
   */
  async stop(): Promise<void> {
    const proc = this.process;
    this.process = null;
    if (!proc) {
      this.cleanupTempConfig();
      return;
    }
    try {
      await this.terminateProcess(proc);
    } finally {
      // Always clean up, even if the child never exited — the previous
      // implementation only cleaned the temp dir on the 'exit' event, leaking
      // it when a wedged child ignored the signal.
      this.cleanupTempConfig();
    }
  }

  /**
   * Terminate a child with SIGTERM, escalating to SIGKILL after shutdownTimeout
   * so a wedged process can never hang stop() indefinitely.
   */
  private terminateProcess(proc: ChildProcess): Promise<void> {
    return new Promise<void>((resolve) => {
      if (proc.exitCode !== null || proc.signalCode !== null) {
        resolve();
        return;
      }

      let resolved = false;
      let graceTimer: ReturnType<typeof setTimeout> | undefined;
      const done = () => {
        if (resolved) {
          return;
        }
        resolved = true;
        clearTimeout(forceTimer);
        if (graceTimer) {
          clearTimeout(graceTimer);
        }
        resolve();
      };

      proc.once('exit', done);

      const forceTimer = setTimeout(() => {
        proc.kill('SIGKILL');
        // Safety net: resolve even if 'exit' never fires after SIGKILL.
        graceTimer = setTimeout(done, 2000);
        graceTimer.unref?.();
      }, this.shutdownTimeout);
      forceTimer.unref?.();

      proc.kill('SIGTERM');
    });
  }

  /**
   * Take a snapshot of the current tab
   */
  async snapshot(params?: SnapshotParams): Promise<SnapshotResponse> {
    return this.request<SnapshotResponse>('/snapshot', params);
  }

  /**
   * Click on a UI element
   */
  async click(params: TabClickParams): Promise<void> {
    await this.request('/tab/click', params);
  }

  /**
   * Lock a tab
   */
  async lock(params: TabLockParams): Promise<void> {
    await this.request('/lock', params);
  }

  /**
   * Unlock a tab
   */
  async unlock(params: TabUnlockParams): Promise<void> {
    await this.request('/unlock', params);
  }

  /**
   * Create a new tab
   */
  async createTab(params: CreateTabParams): Promise<CreateTabResponse> {
    return this.request<CreateTabResponse>('/tab/create', params);
  }

  /**
   * Make a request to the Pinchtab API
   */
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  private async request<T = any>(path: string, body?: any): Promise<T> {
    const url = `${this.baseUrl}${path}`;

    return withAuthedFetch(this.token, this.timeout, async (doFetch) => {
      const response = await doFetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: body ? JSON.stringify(body) : undefined,
      });

      if (!response.ok) {
        const error = await response.text();
        throw new Error(`${response.status}: ${error}`);
      }

      return response.json() as Promise<T>;
    });
  }

  /**
   * Get the path to the Pinchtab binary
   */
  private async getBinaryPathInternal(): Promise<string> {
    const checkoutBinaryPath = getCheckoutBinaryPath(__dirname);
    if (checkoutBinaryPath) {
      if (!fs.existsSync(checkoutBinaryPath)) {
        throw new Error(
          `Pinchtab source-checkout binary not found at ${checkoutBinaryPath}.\n` +
            `Build it first with: bash scripts/npm-dev-binary.sh`
        );
      }
      return checkoutBinaryPath;
    }

    const platform = detectPlatform();
    const binaryName = getBinaryName(platform);

    // Try version-specific path first
    let version: string | undefined;
    try {
      version = readPackageVersion(__dirname);
    } catch (err) {
      console.warn(
        `Could not read version from package.json, falling back to unversioned binary. (${(err as Error).message})`
      );
    }

    const binaryPath = getBinaryPath(binaryName, version);
    if (!fs.existsSync(binaryPath)) {
      throw new Error(
        `Pinchtab binary not found at ${binaryPath}.\n` +
          `Please run: npm rebuild pinchtab\n` +
          `Or pass an explicit path to pinch.start('/path/to/pinchtab')`
      );
    }

    return binaryPath;
  }

  private createTempConfig(): string {
    this.cleanupTempConfig();

    const configDir = fs.mkdtempSync(path.join(os.tmpdir(), 'pinchtab-npm-'));
    const configPath = path.join(configDir, 'config.json');
    const stateDir = path.join(configDir, 'state');
    const token = this.configuredToken || `npm-${randomBytes(16).toString('hex')}`;
    this.token = token;

    fs.writeFileSync(
      configPath,
      JSON.stringify(
        {
          server: {
            bind: '127.0.0.1',
            port: String(this.port),
            stateDir,
            token,
          },
        },
        null,
        2
      )
    );

    this.tempConfigDir = configDir;
    return configPath;
  }

  private cleanupTempConfig(): void {
    if (!this.tempConfigDir) {
      return;
    }
    fs.rmSync(this.tempConfigDir, { recursive: true, force: true });
    this.tempConfigDir = null;
  }

  private async waitForServerReady(): Promise<void> {
    const deadline = Date.now() + 10000;

    while (Date.now() < deadline) {
      if (await this.probeHealthy()) {
        return;
      }
      await new Promise((resolve) => setTimeout(resolve, 250));
    }

    throw new Error(`Pinchtab did not become ready at ${this.baseUrl} within 10s`);
  }

  /**
   * Probe GET /health for an actual PinchTab-ready shape, so a foreign process
   * on the port can't satisfy startup. Bounded per-probe so one hung fetch can't
   * consume the whole readiness budget.
   */
  private async probeHealthy(): Promise<boolean> {
    try {
      return await withAuthedFetch(this.token, 2000, async (doFetch) => {
        const response = await doFetch(`${this.baseUrl}/health`);
        if (!response.ok) {
          return false;
        }
        const body = await response.json().catch(() => null);
        return isPinchtabHealthy(body);
      });
    } catch {
      // Server not ready yet (not listening, hung, or not PinchTab).
      return false;
    }
  }
}

export default Pinchtab;
