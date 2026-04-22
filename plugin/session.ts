import type { PluginConfig } from "./types.js";
import { pinchtabFetch } from "./client.js";

let serverStarted = false;
let lastTabId: string | undefined;
let startupPromise: Promise<{ ok: boolean; error?: string; autoStarted?: boolean }> | null = null;

export function isLocalHost(baseUrl: string): boolean {
  try {
    const url = new URL(baseUrl);
    const host = url.hostname.toLowerCase();
    return host === "localhost" || host === "127.0.0.1" || host === "::1" || host === "[::1]";
  } catch {
    return false;
  }
}

export function getLastTabId(): string | undefined {
  return lastTabId;
}

export function setLastTabId(tabId: string | undefined): void {
  lastTabId = tabId;
}

export function resolveProfile(cfg: PluginConfig, profile?: string): { instanceId?: string; attach?: boolean } {
  const name = profile || cfg.defaultProfile || "openclaw";
  if (cfg.profiles?.[name]) {
    return cfg.profiles[name];
  }
  if (name === "user") {
    return { attach: true };
  }
  return {};
}

export async function getEnhancedHealth(cfg: PluginConfig): Promise<any> {
  const base = cfg.baseUrl || "http://localhost:9867";
  const health = await pinchtabFetch(cfg, "/health");
  const serverOk = !health?.error;

  const result: any = {
    server: serverOk ? "ok" : "unreachable",
    baseUrl: base,
    autoStart: cfg.autoStart !== false,
    defaultProfile: cfg.defaultProfile || "openclaw",
    policies: {
      allowEvaluate: cfg.allowEvaluate === true,
      allowDownloads: cfg.allowDownloads === true,
      allowUploads: cfg.allowUploads === true,
      allowedDomains: cfg.allowedDomains?.length ? cfg.allowedDomains : "all",
    },
  };

  if (serverOk) {
    result.serverHealth = health;
    if (health?.version) result.serverVersion = health.version;
  } else {
    result.error = health?.error;
  }

  const warnings: string[] = [];
  if (cfg.allowEvaluate) warnings.push("evaluate enabled - JS execution allowed");
  if (!cfg.allowedDomains?.length) warnings.push("no domain restrictions");
  if (warnings.length) result.warnings = warnings;

  return result;
}

export async function ensureServerRunning(cfg: PluginConfig): Promise<{ ok: boolean; error?: string; autoStarted?: boolean }> {
  const base = cfg.baseUrl || "http://localhost:9867";

  const healthCheck = await pinchtabFetch(cfg, "/health");
  if (!healthCheck?.error) {
    return { ok: true };
  }

  if (!isLocalHost(base) || cfg.autoStart === false || serverStarted) {
    return { ok: false, error: healthCheck.error };
  }

  // Single-flight guard: if startup is in progress, wait for it
  if (startupPromise) {
    return startupPromise;
  }

  startupPromise = doStartServer(cfg);
  try {
    return await startupPromise;
  } finally {
    startupPromise = null;
  }
}

async function resolveBinaryPath(binary: string): Promise<string> {
  if (binary.startsWith("/") || binary.startsWith("./")) {
    return binary;
  }
  try {
    const { execFileSync } = await import("child_process");
    const resolved = execFileSync("which", [binary], { encoding: "utf8" }).trim();
    return resolved || binary;
  } catch {
    return binary;
  }
}

async function doStartServer(cfg: PluginConfig): Promise<{ ok: boolean; error?: string; autoStarted?: boolean }> {
  const binary = await resolveBinaryPath(cfg.binaryPath || "pinchtab");
  const startupTimeout = cfg.startupTimeoutMs ?? 30000;

  try {
    const { spawn } = await import("child_process");
    const proc = spawn(binary, ["server"], {
      detached: true,
      stdio: "ignore",
    });
    proc.unref();
    serverStarted = true;

    const startTime = Date.now();
    while (Date.now() - startTime < startupTimeout) {
      await new Promise((r) => setTimeout(r, 500));
      const check = await pinchtabFetch(cfg, "/health");
      if (!check?.error) {
        return { ok: true, autoStarted: true };
      }
    }
    return { ok: false, error: `Server failed to start within ${startupTimeout}ms` };
  } catch (err: any) {
    return { ok: false, error: `Failed to start pinchtab: ${err?.message}` };
  }
}
